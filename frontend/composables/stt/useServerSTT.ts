/**
 * Server-side STT provider — sends audio to the backend's faster-whisper service.
 *
 * This is the original transcription path extracted from useVoice.ts.
 * The backend proxies the request to the faster-whisper Docker container.
 */

import type { STTProviderInstance } from '../useSTTProvider';

export function useServerSTT() {
  function createServerSTT(): STTProviderInstance {
    const backendUrl = resolveBackendUrl();
    const isReady = ref(true);
    const isLoading = ref(false);
    const loadProgress = ref(100);
    const error = ref<string | null>(null);

    async function transcribe(audioBlob: Blob, mimeType: string): Promise<string | null> {
      try {
        error.value = null;
        const result = await $fetch<{ text: string }>(
          `${backendUrl}/api/stt/transcribe`,
          {
            method: 'POST',
            headers: {
              'Content-Type': mimeType,
            },
            body: audioBlob,
          },
        );
        return result.text || null;
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'STT server request failed';
        console.error('[voilot] Server STT failed:', msg);
        error.value = msg;
        return null;
      }
    }

    function startStreaming(): void {
      // Server provider is batch-only — streaming is not supported.
      console.warn('[voilot] Server STT does not support streaming');
    }

    async function stopStreaming(): Promise<string | null> {
      // No-op for batch provider.
      return null;
    }

    return {
      name: 'Server (faster-whisper)',
      type: 'batch',
      transcribe,
      startStreaming,
      stopStreaming,
      isReady,
      isLoading,
      loadProgress,
      error,
    };
  }

  return { createServerSTT };
}
