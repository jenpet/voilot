/**
 * Debug log composable — ring buffer logger for the voilot interaction lifecycle.
 *
 * Captures timestamped events from all components (mic, stt, agent, tts, etc.)
 * with the current interaction state attached to every entry.
 *
 * Singleton via module-level state. Enable/disable via toggle; when disabled,
 * log() calls are no-ops. The buffer is capped at MAX_ENTRIES to prevent
 * memory issues; oldest entries are evicted when full.
 */

const MAX_ENTRIES = 5000;

export type DebugLogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface DebugLogEntry {
  timestamp: number;
  elapsed: number;
  state: string;
  level: DebugLogLevel;
  component: string;
  event: string;
  data?: Record<string, unknown>;
}

export interface DebugLogExport {
  exportedAt: string;
  recordingSince: string | null;
  userAgent: string;
  entryCount: number;
  entries: DebugLogEntry[];
}

// Module-level state (singleton)
let _entries: DebugLogEntry[] = [];
let _enabled = false;
let _recordingSince: number | null = null;

// State accessor — set by useInteractionState to avoid circular imports
let _getState: () => string = () => 'unknown';

export function setDebugLogStateAccessor(fn: () => string) {
  _getState = fn;
}

export function useDebugLog() {
  /**
   * Log a debug event. No-op when logging is disabled.
   */
  function log(
    level: DebugLogLevel,
    component: string,
    event: string,
    data?: Record<string, unknown>,
  ) {
    if (!_enabled) return;

    const now = Date.now();
    const entry: DebugLogEntry = {
      timestamp: now,
      elapsed: _recordingSince ? now - _recordingSince : 0,
      state: _getState(),
      level,
      component,
      event,
      data,
    };

    _entries.push(entry);

    // Evict oldest entries when buffer is full
    if (_entries.length > MAX_ENTRIES) {
      _entries = _entries.slice(_entries.length - MAX_ENTRIES);
    }
  }

  /** Enable debug logging and start tracking time. */
  function enable() {
    _enabled = true;
    _recordingSince = Date.now();
    _entries = [];
  }

  /** Disable debug logging and clear the buffer. */
  function disable() {
    _enabled = false;
    _recordingSince = null;
    _entries = [];
  }

  /** Check if logging is enabled. */
  function isEnabled(): boolean {
    return _enabled;
  }

  /** Get the timestamp when logging started. */
  function getRecordingSince(): number | null {
    return _recordingSince;
  }

  /** Export the current buffer as a serializable object. */
  function exportLog(): DebugLogExport {
    return {
      exportedAt: new Date().toISOString(),
      recordingSince: _recordingSince ? new Date(_recordingSince).toISOString() : null,
      userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : 'unknown',
      entryCount: _entries.length,
      entries: [..._entries],
    };
  }

  /** Get the current entry count (for UI display). */
  function getEntryCount(): number {
    return _entries.length;
  }

  return {
    log,
    enable,
    disable,
    isEnabled,
    getRecordingSince,
    getEntryCount,
    exportLog,
  };
}
