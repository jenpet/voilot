/**
 * Minimal mocks for Nuxt auto-imports used in composables under test.
 * Only covers what the audio feedback composables actually use.
 */
export { ref, computed, watch, reactive, toRaw } from 'vue';

const _states: Record<string, { value: unknown; _ref?: unknown }> = {};

/**
 * Simplified useState — returns a reactive ref keyed by name.
 * Resets are handled per-test via resetNuxtState().
 */
export function useState<T>(key: string, init?: () => T) {
  if (!_states[key]) {
    _states[key] = { value: init ? init() : undefined };
  }
  // Return a ref-like object. For tests we use the vue ref directly.
  const { ref: vueRef } = require('vue');
  if (!_states[key]._ref) {
    _states[key]._ref = vueRef(_states[key].value);
  }
  return _states[key]._ref;
}

/**
 * Reset all useState entries between tests.
 */
export function resetNuxtState() {
  for (const key of Object.keys(_states)) {
    delete _states[key];
  }
}
