/**
 * Persistent user settings backed by localStorage.
 *
 * All settings are reactive (via useState) and automatically persisted.
 * On first load, values are read from localStorage or fall back to defaults.
 */

const STORAGE_KEY = 'voilot-settings';

interface Settings {
  /** Silence duration in ms before auto-stop (500–5000). */
  silenceDurationMs: number;
  /** Whether to show the round-trip timing bar after voice interactions. */
  showRoundTripTimings: boolean;
}

const DEFAULTS: Settings = {
  silenceDurationMs: 2500,
  showRoundTripTimings: false,
};

let _initialized = false;

function _load(): Settings {
  if (typeof localStorage === 'undefined') return { ...DEFAULTS };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULTS };
    const parsed = JSON.parse(raw);
    return {
      silenceDurationMs: typeof parsed.silenceDurationMs === 'number'
        ? Math.max(500, Math.min(5000, parsed.silenceDurationMs))
        : DEFAULTS.silenceDurationMs,
      showRoundTripTimings: typeof parsed.showRoundTripTimings === 'boolean'
        ? parsed.showRoundTripTimings
        : DEFAULTS.showRoundTripTimings,
    };
  } catch {
    return { ...DEFAULTS };
  }
}

function _save(settings: Settings): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // Storage full or unavailable — best effort
  }
}

export function useSettings() {
  const silenceDurationMs = useState('settings-silence-duration', () => DEFAULTS.silenceDurationMs);
  const showRoundTripTimings = useState('settings-show-round-trip-timings', () => DEFAULTS.showRoundTripTimings);

  // Load from localStorage once (client-side only)
  if (!_initialized && import.meta.client) {
    const loaded = _load();
    silenceDurationMs.value = loaded.silenceDurationMs;
    showRoundTripTimings.value = loaded.showRoundTripTimings;
    _initialized = true;
  }

  function setSilenceDuration(ms: number): void {
    const clamped = Math.max(500, Math.min(5000, Math.round(ms)));
    silenceDurationMs.value = clamped;
    _save({ silenceDurationMs: clamped, showRoundTripTimings: showRoundTripTimings.value });
  }

  function setShowRoundTripTimings(val: boolean): void {
    showRoundTripTimings.value = val;
    _save({ silenceDurationMs: silenceDurationMs.value, showRoundTripTimings: val });
  }

  return {
    silenceDurationMs: readonly(silenceDurationMs) as Readonly<Ref<number>>,
    showRoundTripTimings: readonly(showRoundTripTimings) as Readonly<Ref<boolean>>,
    setSilenceDuration,
    setShowRoundTripTimings,
  };
}
