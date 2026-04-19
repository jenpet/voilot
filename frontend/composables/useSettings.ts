/**
 * Reactive settings composable backed by localStorage.
 *
 * All settings are persisted to localStorage as JSON and restored on
 * page load. Reactive via Vue's ref + watch, so any component reading
 * `settings.value.stt.provider` will automatically update when the user
 * changes it in the Settings panel.
 *
 * Singleton via useState — all callers share the same reactive state.
 */

const STORAGE_KEY = 'voilot-settings';

export type STTProviderType = 'server' | 'web-speech-api' | 'whisper-wasm';

export type WhisperModel =
  | 'onnx-community/whisper-tiny.en'
  | 'onnx-community/whisper-base.en';

export type WhisperDtype = 'q8' | 'fp16' | 'q4';

export interface STTSettings {
  provider: STTProviderType;
  whisperModel: WhisperModel;
  whisperDtype: WhisperDtype;
}

export interface VoilotSettings {
  stt: STTSettings;
  // Future: tts, theme, etc.
}

const defaultSettings: VoilotSettings = {
  stt: {
    provider: 'server',
    whisperModel: 'onnx-community/whisper-tiny.en',
    whisperDtype: 'q8',
  },
};

function loadFromStorage(): VoilotSettings {
  if (typeof window === 'undefined') return structuredClone(defaultSettings);
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return structuredClone(defaultSettings);
    const parsed = JSON.parse(raw) as Partial<VoilotSettings>;
    // Merge with defaults to handle missing keys from older versions
    return {
      stt: {
        ...defaultSettings.stt,
        ...(parsed.stt || {}),
      },
    };
  } catch {
    return structuredClone(defaultSettings);
  }
}

function saveToStorage(settings: VoilotSettings): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    console.error('[voilot] Failed to save settings to localStorage');
  }
}

/** Whether the settings panel is open. Shared state so any page can toggle it. */
const _settingsPanelOpen = ref(false);

export function useSettings() {
  const settings = useState<VoilotSettings>('voilot-settings', () => loadFromStorage());

  // Persist on every change
  watch(settings, (val) => {
    saveToStorage(val);
  }, { deep: true });

  function updateSTT(partial: Partial<STTSettings>): void {
    settings.value = {
      ...settings.value,
      stt: {
        ...settings.value.stt,
        ...partial,
      },
    };
  }

  function resetToDefaults(): void {
    settings.value = structuredClone(defaultSettings);
  }

  return {
    settings: readonly(settings),
    settingsPanelOpen: _settingsPanelOpen,
    updateSTT,
    resetToDefaults,
  };
}
