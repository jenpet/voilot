/**
 * Web Speech API STT provider — uses the browser's built-in speech
 * recognition engine for real-time streaming transcription.
 *
 * On iOS Safari this leverages the on-device Siri recognition engine
 * via the prefixed `webkitSpeechRecognition` API (available since
 * iOS 14.5). Non-continuous mode is used because `continuous = true`
 * is unreliable on Safari — voilot's press-to-talk UX only needs
 * single-utterance recognition anyway.
 *
 * Dual-path: useVoice.ts always runs MediaRecorder in parallel so
 * that if streaming recognition fails or returns empty, the recorded
 * audio blob can be sent to the server as a fallback.
 */

import type { STTProviderInstance } from '../useSTTProvider';

/**
 * SpeechRecognition is not yet in lib.dom.d.ts for all browsers.
 * Declare the minimal subset we use to avoid TypeScript errors.
 */
interface SpeechRecognitionEvent extends Event {
  readonly results: SpeechRecognitionResultList;
  readonly resultIndex: number;
}

interface SpeechRecognitionErrorEvent extends Event {
  readonly error: string;
  readonly message?: string;
}

interface SpeechRecognitionInstance extends EventTarget {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  maxAlternatives: number;
  onresult: ((event: SpeechRecognitionEvent) => void) | null;
  onerror: ((event: SpeechRecognitionErrorEvent) => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
  abort(): void;
}

type SpeechRecognitionConstructor = new () => SpeechRecognitionInstance;

/** Get the SpeechRecognition constructor, preferring the prefixed version for Safari. */
function getSpeechRecognitionCtor(): SpeechRecognitionConstructor | null {
  if (typeof window === 'undefined') return null;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const w = window as any;
  return w.SpeechRecognition || w.webkitSpeechRecognition || null;
}

export function useWebSpeechSTT() {
  function createWebSpeechSTT(): STTProviderInstance {
    const Ctor = getSpeechRecognitionCtor();
    const isReady = ref(Ctor !== null);
    const isLoading = ref(false);
    const loadProgress = ref(Ctor ? 100 : 0);
    const error = ref<string | null>(null);

    let _recognition: SpeechRecognitionInstance | null = null;
    /** Accumulates all final transcript segments during a streaming session. */
    let _accumulatedTranscript = '';

    /**
     * Batch transcription is not the primary path for Web Speech API,
     * but we implement it as a no-op that returns null so the provider
     * interface is satisfied. The actual transcription comes from
     * streaming callbacks. If streaming fails, useVoice.ts will fall
     * back to the server provider's batch transcription.
     */
    async function transcribe(_audioBlob: Blob, _mimeType: string): Promise<string | null> {
      // Web Speech API doesn't support file-based transcription.
      // Return null to signal that the caller should fall back.
      return null;
    }

    function startStreaming(
      onInterim: (text: string) => void,
      onFinal: (text: string) => void,
    ): void {
      if (!Ctor) {
        error.value = 'Web Speech API is not available in this browser';
        return;
      }

      // Clean up any lingering instance
      if (_recognition) {
        try { _recognition.abort(); } catch { /* ignore */ }
        _recognition = null;
      }

      error.value = null;
      _accumulatedTranscript = '';

      const recognition = new Ctor();
      recognition.lang = 'en-US';
      recognition.continuous = false;     // Single utterance — reliable on iOS Safari
      recognition.interimResults = true;
      recognition.maxAlternatives = 1;

      recognition.onresult = (event: SpeechRecognitionEvent) => {
        let interimTranscript = '';
        let finalTranscript = '';

        for (let i = event.resultIndex; i < event.results.length; i++) {
          const result = event.results[i];
          const transcript = result[0]?.transcript || '';

          if (result.isFinal) {
            finalTranscript += transcript;
          } else {
            interimTranscript += transcript;
          }
        }

        if (finalTranscript) {
          _accumulatedTranscript += finalTranscript;
          onFinal(_accumulatedTranscript);
        } else if (interimTranscript) {
          onInterim(_accumulatedTranscript + interimTranscript);
        }
      };

      recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
        // "no-speech" and "aborted" are expected when the user stops
        // quickly or there's just silence — not real errors.
        if (event.error === 'no-speech' || event.error === 'aborted') {
          return;
        }
        const msg = `Web Speech API error: ${event.error}`;
        console.warn('[voilot]', msg);
        error.value = msg;
      };

      recognition.onend = () => {
        // Recognition ended naturally. Don't null _recognition here —
        // stopStreaming() manages that and resolves the pending promise.
      };

      try {
        recognition.start();
        _recognition = recognition;
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Failed to start Web Speech API';
        console.error('[voilot]', msg);
        error.value = msg;
      }
    }

    /**
     * Stop streaming recognition and wait for the engine to deliver
     * any pending final results before returning.
     *
     * recognition.stop() tells the browser to finalize pending results
     * and fire onresult (isFinal) + onend. We wait for onend so we
     * don't miss the final transcript.
     */
    async function stopStreaming(): Promise<string | null> {
      if (!_recognition) {
        const result = _accumulatedTranscript || null;
        _accumulatedTranscript = '';
        return result;
      }

      const recognition = _recognition;
      _recognition = null;

      return new Promise<string | null>((resolve) => {
        // Safety timeout — if the browser never fires onend, don't hang forever
        const timeout = setTimeout(() => {
          const result = _accumulatedTranscript || null;
          _accumulatedTranscript = '';
          resolve(result);
        }, 2000);

        recognition.onend = () => {
          clearTimeout(timeout);
          const result = _accumulatedTranscript || null;
          _accumulatedTranscript = '';
          resolve(result);
        };

        try {
          // stop() lets the engine finalize pending results before firing onend.
          recognition.stop();
        } catch {
          // Already stopped — resolve with what we have
          clearTimeout(timeout);
          const result = _accumulatedTranscript || null;
          _accumulatedTranscript = '';
          resolve(result);
        }
      });
    }

    return {
      name: 'Web Speech API',
      type: 'streaming',
      transcribe,
      startStreaming,
      stopStreaming,
      isReady,
      isLoading,
      loadProgress,
      error,
    };
  }

  return { createWebSpeechSTT };
}
