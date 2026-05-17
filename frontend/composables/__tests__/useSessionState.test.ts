/**
 * Tests for useSessionState shared refs.
 *
 * Validates that useState-backed session state behaves correctly across
 * re-initializations (simulating SPA navigation between sessions).
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

import { useIsStreaming } from '../useSessionState';

describe('useSessionState', () => {
  beforeEach(() => {
    _useStateCache.clear();
  });

  describe('useIsStreaming', () => {
    it('defaults to false on first access', () => {
      const { isStreaming } = useIsStreaming();
      expect(isStreaming.value).toBe(false);
    });

    it('setter updates the value', () => {
      const { isStreaming, _setIsStreaming } = useIsStreaming();
      _setIsStreaming(true);
      expect(isStreaming.value).toBe(true);
    });
  });
});
