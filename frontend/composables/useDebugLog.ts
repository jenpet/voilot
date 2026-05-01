/**
 * Debug log composable — ring buffer logger for the voilot interaction lifecycle.
 *
 * Captures timestamped events from all components (mic, stt, agent, tts, etc.)
 * with the current interaction state attached to every entry.
 *
 * Enabled by default. The preference is persisted in localStorage (inside the
 * voilot-settings object). When disabled, log() calls are no-ops.
 *
 * Entries older than RETENTION_MINUTES are evicted on each log() call.
 * MAX_ENTRIES serves as a hard safety cap to prevent runaway memory usage.
 */

const MAX_ENTRIES = 5000;
const RETENTION_MINUTES = 15;
const RETENTION_MS = RETENTION_MINUTES * 60 * 1000;
const SETTINGS_KEY = 'voilot-settings';

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
  retentionMinutes: number;
  userAgent: string;
  entryCount: number;
  entries: DebugLogEntry[];
}

// Module-level state (singleton)
let _entries: DebugLogEntry[] = [];
let _enabled = false;
let _recordingSince: number | null = null;
let _initialized = false;

// State accessor — set by useInteractionState to avoid circular imports
let _getState: () => string = () => 'unknown';

export function setDebugLogStateAccessor(fn: () => string) {
  _getState = fn;
}

/** Read debugLogEnabled from localStorage (voilot-settings object). */
function _readEnabled(): boolean {
  if (typeof localStorage === 'undefined') return true;
  try {
    const raw = localStorage.getItem(SETTINGS_KEY);
    if (!raw) return true;
    const parsed = JSON.parse(raw);
    return typeof parsed.debugLogEnabled === 'boolean' ? parsed.debugLogEnabled : true;
  } catch {
    return true;
  }
}

/** Persist debugLogEnabled into the voilot-settings localStorage object. */
function _persistEnabled(value: boolean): void {
  if (typeof localStorage === 'undefined') return;
  try {
    const raw = localStorage.getItem(SETTINGS_KEY);
    const settings = raw ? JSON.parse(raw) : {};
    settings.debugLogEnabled = value;
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    // Storage full or unavailable — best effort
  }
}

/** Lazy initialization — runs once on first use. */
function _ensureInit(): void {
  if (_initialized) return;
  _initialized = true;
  _enabled = _readEnabled();
  if (_enabled) {
    _recordingSince = Date.now();
  }
}

/** Evict entries older than the retention window. */
function _evictStale(now: number): void {
  const cutoff = now - RETENTION_MS;
  let i = 0;
  while (i < _entries.length && _entries[i].timestamp < cutoff) {
    i++;
  }
  if (i > 0) {
    _entries = _entries.slice(i);
  }
}

export function useDebugLog() {
  _ensureInit();

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

    // Time-based eviction
    _evictStale(now);

    // Hard safety cap
    if (_entries.length > MAX_ENTRIES) {
      _entries = _entries.slice(_entries.length - MAX_ENTRIES);
    }
  }

  /** Enable debug logging and start tracking time. */
  function enable() {
    _enabled = true;
    _recordingSince = Date.now();
    _entries = [];
    _persistEnabled(true);
  }

  /** Disable debug logging and clear the buffer. */
  function disable() {
    _enabled = false;
    _recordingSince = null;
    _entries = [];
    _persistEnabled(false);
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
      retentionMinutes: RETENTION_MINUTES,
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
