/**
 * Whisper WASM STT provider — runs Whisper inference locally in the
 * browser via a Web Worker + Transformers.js (ONNX Runtime WASM backend).
 *
 * Pipeline:
 * 1. User records audio → MediaRecorder captures a Blob
 * 2. Blob is decoded to 16 kHz mono Float32Array via AudioContext
 * 3. PCM data is transferred to the Web Worker (zero-copy via transferable)
 * 4. Worker runs the Whisper pipeline and returns transcribed text
 *
 * Model is downloaded on first use and cached via the Cache API.
 * The SettingsPanel can trigger preloading via `loadModel()`.
 *
 * Safari AudioContext caveat: Safari may not honor the `sampleRate`
 * constructor option. If the decoded buffer has a different sample rate,
 * we manually resample to 16 kHz.
 */

import type { STTProviderInstance } from '../useSTTProvider';

const TARGET_SAMPLE_RATE = 16000;

/** Module-level singleton state so only one worker is ever created. */
let _worker: Worker | null = null;
let _workerReady = false;
let _loadedModel: string | null = null;
let _loadedDtype: string | null = null;

/**
 * Pending transcription resolve/reject — only one transcription can
 * be in flight at a time (press-to-talk UX).
 */
let _pendingResolve: ((text: string | null) => void) | null = null;

/** Shared reactive refs (singleton across all useWhisperSTT() calls). */
const _isReady = ref(false);
const _isLoading = ref(false);
const _loadProgress = ref(0);
const _error = ref<string | null>(null);
/** Whether the model files are cached (downloaded before, even if not loaded in memory). */
const _isCached = ref(false);

/** Load-completion promise so concurrent loadModel() calls coalesce. */
let _loadPromise: Promise<void> | null = null;

function getOrCreateWorker(): Worker {
  if (_worker) return _worker;

  // Vite's worker detection requires a relative path (not a ~ alias).
  // The URL is resolved at build time relative to this file's location.
  _worker = new Worker(
    new URL('../../workers/whisper-worker.ts', import.meta.url),
    { type: 'module' },
  );

  _worker.onmessage = (e: MessageEvent) => {
    const msg = e.data;
    console.debug('[voilot] Whisper worker message:', msg.type, msg);

    switch (msg.type) {
      case 'progress':
        _loadProgress.value = msg.progress;
        break;

      case 'ready':
        _workerReady = true;
        _isReady.value = true;
        _isLoading.value = false;
        _loadProgress.value = 100;
        _error.value = null;
        break;

      case 'result':
        console.debug('[voilot] Whisper transcription result:', msg.text);
        if (_pendingResolve) {
          _pendingResolve(msg.text || null);
          _pendingResolve = null;
        }
        break;

      case 'error':
        console.error('[voilot] Whisper worker error:', msg.message);
        _error.value = msg.message;
        _isLoading.value = false;
        // Reject pending transcription if any
        if (_pendingResolve) {
          _pendingResolve(null);
          _pendingResolve = null;
        }
        break;
    }
  };

  _worker.onerror = (e: ErrorEvent) => {
    const msg = e.message || 'Worker crashed';
    console.error('[voilot] Whisper worker error:', msg);
    _error.value = msg;
    _isLoading.value = false;
    _isReady.value = false;
    _workerReady = false;
    if (_pendingResolve) {
      _pendingResolve(null);
      _pendingResolve = null;
    }
  };

  return _worker;
}

/**
 * Decode an audio Blob to 16 kHz mono Float32Array.
 *
 * Safari may ignore the sampleRate constructor option, so we check
 * the actual decoded sample rate and resample if necessary.
 */
async function decodeToFloat32(blob: Blob): Promise<Float32Array> {
  const arrayBuffer = await blob.arrayBuffer();
  console.debug('[voilot] Whisper decodeToFloat32: blob size=', blob.size, 'type=', blob.type, 'arrayBuffer size=', arrayBuffer.byteLength);

  // Try to create an AudioContext at 16 kHz — Safari may ignore this.
  let audioCtx: AudioContext;
  try {
    audioCtx = new AudioContext({ sampleRate: TARGET_SAMPLE_RATE });
  } catch {
    audioCtx = new AudioContext();
  }

  try {
    const audioBuffer = await audioCtx.decodeAudioData(arrayBuffer);
    console.debug('[voilot] Whisper decodeToFloat32: decoded sampleRate=', audioBuffer.sampleRate, 'channels=', audioBuffer.numberOfChannels, 'duration=', audioBuffer.duration.toFixed(2), 's, frames=', audioBuffer.length);
    const channelData = audioBuffer.getChannelData(0); // mono

    // If the AudioContext honored our sampleRate, we're done
    if (audioBuffer.sampleRate === TARGET_SAMPLE_RATE) {
      return channelData;
    }

    // Manual linear resampling (Safari fallback)
    console.debug('[voilot] Whisper decodeToFloat32: resampling from', audioBuffer.sampleRate, 'to', TARGET_SAMPLE_RATE);
    return resample(channelData, audioBuffer.sampleRate, TARGET_SAMPLE_RATE);
  } finally {
    audioCtx.close().catch(() => {});
  }
}

/**
 * Simple linear interpolation resampling.
 * Good enough for speech — Whisper is robust to minor artifacts.
 */
function resample(input: Float32Array, fromRate: number, toRate: number): Float32Array {
  const ratio = fromRate / toRate;
  const outputLength = Math.round(input.length / ratio);
  const output = new Float32Array(outputLength);

  for (let i = 0; i < outputLength; i++) {
    const srcIndex = i * ratio;
    const low = Math.floor(srcIndex);
    const high = Math.min(low + 1, input.length - 1);
    const frac = srcIndex - low;
    output[i] = input[low] * (1 - frac) + input[high] * frac;
  }

  return output;
}

export function useWhisperSTT() {
  /**
   * Check if a Whisper model's files are already in the browser's Cache API.
   * Transformers.js uses a cache named 'transformers-cache' and stores files
   * under URLs like https://huggingface.co/{model}/resolve/main/{file}.
   */
  async function checkModelCached(model: string): Promise<boolean> {
    if (typeof caches === 'undefined') return false;
    try {
      const cache = await caches.open('transformers-cache');
      const keys = await cache.keys();
      // Check if any cached URL contains the model name
      return keys.some(req => req.url.includes(model.replace('/', '%2F')) || req.url.includes(model));
    } catch {
      return false;
    }
  }

  function createWhisperSTT(): STTProviderInstance {
    const { settings } = useSettings();

    /**
     * Load (or preload) the Whisper model. Called from the settings
     * panel "Download Model" button or lazily on first transcription.
     */
    function loadModel(): Promise<void> {
      const model = settings.value.stt.whisperModel;
      const dtype = settings.value.stt.whisperDtype;

      // Already loaded with this config
      if (_workerReady && _loadedModel === model && _loadedDtype === dtype) {
        return Promise.resolve();
      }

      // Coalesce concurrent load requests
      if (_loadPromise && _isLoading.value) {
        return _loadPromise;
      }

      _isLoading.value = true;
      _loadProgress.value = 0;
      _error.value = null;

      const worker = getOrCreateWorker();
      _loadPromise = new Promise<void>((resolve) => {
        // Listen for ready/error to resolve this promise
        const originalOnMessage = worker.onmessage;
        worker.onmessage = (e: MessageEvent) => {
          // Delegate to the main handler first
          if (originalOnMessage) {
            (originalOnMessage as (e: MessageEvent) => void)(e);
          }

          if (e.data.type === 'ready') {
            _loadedModel = model;
            _loadedDtype = dtype;
            _isCached.value = true;
            worker.onmessage = originalOnMessage;
            _loadPromise = null;
            resolve();
          } else if (e.data.type === 'error') {
            worker.onmessage = originalOnMessage;
            _loadPromise = null;
            resolve(); // Resolve (not reject) — error is in the reactive ref
          }
        };

        worker.postMessage({ type: 'load', model, dtype });
      });

      return _loadPromise;
    }

    async function transcribe(audioBlob: Blob, _mimeType: string): Promise<string | null> {
      // Ensure model is loaded
      if (!_workerReady) {
        await loadModel();
      }

      if (!_workerReady || !_worker) {
        _error.value = 'Whisper model is not loaded';
        return null;
      }

      try {
        _error.value = null;
        const pcm = await decodeToFloat32(audioBlob);
        console.debug('[voilot] Whisper: decoded audio to', pcm.length, 'samples at 16kHz,', (pcm.length / TARGET_SAMPLE_RATE).toFixed(1), 'seconds');

        return new Promise<string | null>((resolve) => {
          // Safety timeout — if worker never responds, don't hang forever
          const timeout = setTimeout(() => {
            console.warn('[voilot] Whisper: transcription timed out after 30s');
            if (_pendingResolve === resolve) {
              _pendingResolve = null;
              _error.value = 'Transcription timed out';
              resolve(null);
            }
          }, 30000);

          _pendingResolve = (text: string | null) => {
            clearTimeout(timeout);
            resolve(text);
          };
          // Transfer the Float32Array buffer (zero-copy)
          _worker!.postMessage(
            { type: 'transcribe', audio: pcm },
            [pcm.buffer],
          );
        });
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Audio decoding failed';
        console.error('[voilot] Whisper decode error:', msg);
        _error.value = msg;
        return null;
      }
    }

    function startStreaming(): void {
      // Whisper WASM is batch-only — streaming not supported.
      console.warn('[voilot] Whisper WASM does not support streaming');
    }

    async function stopStreaming(): Promise<string | null> {
      // No-op for batch provider.
      return null;
    }

    // Check if model is cached on creation (async, updates reactively)
    checkModelCached(settings.value.stt.whisperModel).then(cached => {
      _isCached.value = cached;
    });

    return {
      name: 'On-Device Whisper (WASM)',
      type: 'batch',
      transcribe,
      startStreaming,
      stopStreaming,
      isReady: _isReady,
      isLoading: _isLoading,
      loadProgress: _loadProgress,
      error: _error,
      // Extra properties for the settings panel
      isCached: _isCached,
      loadModel,
    } as STTProviderInstance & { loadModel: () => Promise<void>; isCached: Ref<boolean> };
  }

  return { createWhisperSTT };
}
