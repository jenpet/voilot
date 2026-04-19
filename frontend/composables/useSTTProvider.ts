/**
 * STT provider abstraction — routes transcription to the active provider
 * based on user settings.
 *
 * Providers implement either batch (record → transcribe) or streaming
 * (real-time recognition) transcription. The dual-path logic in useVoice.ts
 * uses the provider type to decide how to handle recording.
 */

import type { Ref } from 'vue';
import { useServerSTT } from '~/composables/stt/useServerSTT';
import { useWebSpeechSTT } from '~/composables/stt/useWebSpeechSTT';
import { useWhisperSTT } from '~/composables/stt/useWhisperSTT';

export interface STTProviderInstance {
  /** Human-readable name for display / debugging. */
  name: string;

  /** 'batch' = record-then-transcribe, 'streaming' = real-time recognition. */
  type: 'batch' | 'streaming';

  /**
   * Transcribe an audio blob (batch providers only).
   * @param audioBlob  Recorded audio data.
   * @param mimeType   MIME type of the blob (e.g., 'audio/webm;codecs=opus').
   * @returns Transcribed text, or null if nothing was recognized.
   */
  transcribe(audioBlob: Blob, mimeType: string): Promise<string | null>;

  /**
   * Start streaming recognition (streaming providers only).
   * @param onInterim  Called with partial/interim transcription results.
   * @param onFinal    Called with the final committed transcription.
   */
  startStreaming(
    onInterim: (text: string) => void,
    onFinal: (text: string) => void,
  ): void;

  /**
   * Stop streaming recognition and wait for the final result.
   * Returns the final transcribed text, or null if nothing was recognized.
   */
  stopStreaming(): Promise<string | null>;

  /** Whether the provider is ready to transcribe (model loaded, service reachable, etc.). */
  isReady: Ref<boolean>;

  /** Whether the provider is loading (downloading model, initializing, etc.). */
  isLoading: Ref<boolean>;

  /** Download/initialization progress (0–100). Only meaningful when isLoading is true. */
  loadProgress: Ref<number>;

  /** Last error message, or null if no error. */
  error: Ref<string | null>;
}

// Module-level provider cache — true singletons shared across all
// useSTTProvider() callers (useVoice, SettingsPanel, etc.).
let _serverProvider: STTProviderInstance | null = null;
let _webSpeechProvider: STTProviderInstance | null = null;
let _whisperProvider: STTProviderInstance | null = null;

/**
 * Returns the currently active STT provider based on user settings.
 *
 * The returned object is reactive — if the user changes the STT provider
 * in settings, the next call to transcribe() will use the new provider.
 */
export function useSTTProvider(): {
  provider: ComputedRef<STTProviderInstance>;
} {
  const { settings } = useSettings();

  function getServerProvider(): STTProviderInstance {
    if (!_serverProvider) {
      const { createServerSTT } = useServerSTT();
      _serverProvider = createServerSTT();
    }
    return _serverProvider;
  }

  function getWebSpeechProvider(): STTProviderInstance {
    if (!_webSpeechProvider) {
      const { createWebSpeechSTT } = useWebSpeechSTT();
      _webSpeechProvider = createWebSpeechSTT();
    }
    return _webSpeechProvider;
  }

  function getWhisperProvider(): STTProviderInstance {
    if (!_whisperProvider) {
      const { createWhisperSTT } = useWhisperSTT();
      _whisperProvider = createWhisperSTT();
    }
    return _whisperProvider;
  }

  const provider = computed<STTProviderInstance>(() => {
    switch (settings.value.stt.provider) {
      case 'web-speech-api':
        return getWebSpeechProvider();
      case 'whisper-wasm':
        return getWhisperProvider();
      case 'server':
      default:
        return getServerProvider();
    }
  });

  return { provider };
}
