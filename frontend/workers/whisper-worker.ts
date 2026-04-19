/**
 * Web Worker for Whisper WASM inference via Transformers.js.
 *
 * Runs the Whisper ASR pipeline in a dedicated thread so the UI stays
 * responsive during model loading (~41-77 MB download) and inference
 * (~3-5s per utterance on mobile).
 *
 * Communication protocol:
 *   Main -> Worker:  { type: 'load', model: string, dtype: string }
 *   Worker -> Main:  { type: 'progress', progress: number }  // 0-100
 *   Worker -> Main:  { type: 'ready' }
 *   Main -> Worker:  { type: 'transcribe', audio: Float32Array }  // transferable
 *   Worker -> Main:  { type: 'result', text: string }
 *   Worker -> Main:  { type: 'error', message: string }
 *
 * Models are automatically cached by Transformers.js via the Cache API.
 */

import { pipeline, type AutomaticSpeechRecognitionPipeline } from '@huggingface/transformers';

let transcriber: AutomaticSpeechRecognitionPipeline | null = null;
let currentModel: string | null = null;
let currentDtype: string | null = null;

self.onmessage = async (e: MessageEvent) => {
  const msg = e.data;

  if (msg.type === 'load') {
    const { model, dtype } = msg as { model: string; dtype: string };

    // Skip if the requested model is already loaded
    if (transcriber && currentModel === model && currentDtype === dtype) {
      self.postMessage({ type: 'ready' });
      return;
    }

    try {
      // Dispose previous pipeline if switching models
      if (transcriber) {
        await transcriber.dispose();
        transcriber = null;
      }

      transcriber = await pipeline(
        'automatic-speech-recognition',
        model,
        {
          dtype,
          device: 'wasm',
          progress_callback: (progress: { progress?: number; status?: string }) => {
            if (typeof progress.progress === 'number') {
              self.postMessage({ type: 'progress', progress: progress.progress });
            }
          },
        },
      );

      currentModel = model;
      currentDtype = dtype;
      self.postMessage({ type: 'ready' });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load Whisper model';
      console.error('[whisper-worker] Load error:', message);
      self.postMessage({ type: 'error', message });
    }
  } else if (msg.type === 'transcribe') {
    if (!transcriber) {
      self.postMessage({ type: 'error', message: 'Model not loaded' });
      return;
    }

    try {
      const audio = msg.audio as Float32Array;
      console.log('[whisper-worker] Transcribing audio:', audio.length, 'samples,', (audio.length / 16000).toFixed(1), 'seconds');

      // Log audio stats to verify it's not silence
      let min = Infinity, max = -Infinity, sumSq = 0;
      for (let i = 0; i < audio.length; i++) {
        if (audio[i] < min) min = audio[i];
        if (audio[i] > max) max = audio[i];
        sumSq += audio[i] * audio[i];
      }
      const rms = Math.sqrt(sumSq / audio.length);
      console.log('[whisper-worker] Audio stats: min=', min.toFixed(4), 'max=', max.toFixed(4), 'rms=', rms.toFixed(4));

      const result = await transcriber(audio, {
        language: 'en',
        task: 'transcribe',
      });

      console.log('[whisper-worker] Raw result:', JSON.stringify(result));

      // Result can be a single output or array — normalize to string.
      const text = Array.isArray(result)
        ? result.map((r) => r.text).join(' ')
        : result.text;

      console.log('[whisper-worker] Final text:', JSON.stringify(text.trim()));
      self.postMessage({ type: 'result', text: text.trim() });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Transcription failed';
      console.error('[whisper-worker] Transcribe error:', message);
      self.postMessage({ type: 'error', message });
    }
  }
};
