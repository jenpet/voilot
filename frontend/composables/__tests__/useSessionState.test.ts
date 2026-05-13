/**
 * Tests for useSessionState shared refs.
 *
 * Validates that useState-backed session state behaves correctly across
 * re-initializations (simulating SPA navigation between sessions).
 *
 * Key invariant: suppressInitialTools must be explicitly reset to true
 * on each session init — Nuxt's useState only runs the init function
 * on first access per key, so subsequent calls return the stale value.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ref, readonly } from 'vue';

// ── Nuxt auto-import stubs ──────────────────────────────────────────

vi.stubGlobal('ref', ref);
vi.stubGlobal('readonly', readonly);

// Mimic Nuxt's useState: init function only runs on first access per key.
// This is the exact behavior that causes stale state across SPA navigations.
const _useStateCache = new Map<string, any>();
vi.stubGlobal('useState', (key: string, init?: () => any) => {
  if (!_useStateCache.has(key)) {
    _useStateCache.set(key, ref(init ? init() : undefined));
  }
  return _useStateCache.get(key)!;
});

import { useSuppressInitialTools } from '../useSessionState';

describe('useSessionState', () => {
  beforeEach(() => {
    _useStateCache.clear();
  });

  describe('useSuppressInitialTools', () => {
    it('defaults to true on first access', () => {
      const { suppressInitialTools } = useSuppressInitialTools();
      expect(suppressInitialTools.value).toBe(true);
    });

    it('setter updates the value', () => {
      const { suppressInitialTools, _setSuppressInitialTools } = useSuppressInitialTools();
      _setSuppressInitialTools(false);
      expect(suppressInitialTools.value).toBe(false);
    });

    it('useState caches value — second call returns stale false, not default true', () => {
      // First "session": set to false (simulates text arriving)
      const first = useSuppressInitialTools();
      first._setSuppressInitialTools(false);

      // Second "session": re-call the composable (simulates SPA navigation)
      // Without an explicit reset, this inherits the stale false value.
      const second = useSuppressInitialTools();
      expect(second.suppressInitialTools.value).toBe(false); // NOT true!
    });

    it('explicit reset to true restores the guard for a new session', () => {
      // First "session": cleared the flag
      const first = useSuppressInitialTools();
      first._setSuppressInitialTools(false);

      // Simulate what useAgent init should do: explicit reset
      const second = useSuppressInitialTools();
      second._setSuppressInitialTools(true);

      expect(second.suppressInitialTools.value).toBe(true);
    });

    it('both calls share the same underlying ref', () => {
      const a = useSuppressInitialTools();
      const b = useSuppressInitialTools();

      a._setSuppressInitialTools(false);
      expect(b.suppressInitialTools.value).toBe(false);

      b._setSuppressInitialTools(true);
      expect(a.suppressInitialTools.value).toBe(true);
    });
  });
});
