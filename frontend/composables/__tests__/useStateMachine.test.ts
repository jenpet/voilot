import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ref, computed, readonly } from 'vue';

// Provide Nuxt auto-imports as globals
vi.stubGlobal('ref', ref);
vi.stubGlobal('computed', computed);
vi.stubGlobal('readonly', readonly);
vi.stubGlobal('useState', (key: string, init?: () => any) => ref(init ? init() : undefined));

// Mock useDebugLog
vi.mock('../useDebugLog', () => ({
  useDebugLog: () => ({ log: () => {} }),
  setDebugLogStateAccessor: () => {},
}));

import {
  dispatch,
  abort,
  getState,
  _forceStateForTesting,
  ACTION_TABLE,
  INTERACTION_STATES,
  type ActionName,
  type InteractionState,
} from '../useStateMachine';

beforeEach(() => {
  _forceStateForTesting('idle');
});

// ── Exhaustive action/state matrix ──────────────────────────────────

describe('action/state matrix', () => {
  const allActions = Object.keys(ACTION_TABLE) as ActionName[];
  const allStates = [...INTERACTION_STATES];

  for (const action of allActions) {
    const def = ACTION_TABLE[action];

    describe(`action: ${action}`, () => {
      for (const state of allStates) {
        const shouldSucceed = def.from.includes(state);

        it(`from ${state} → ${shouldSucceed ? 'accepted' : 'rejected'}`, () => {
          _forceStateForTesting(state);

          const result = dispatch(action, 'test');

          if (shouldSucceed) {
            expect(result).toBe(true);
            expect(getState()).toBe(def.to);
          } else {
            expect(result).toBe(false);
            expect(getState()).toBe(state);
          }
        });
      }
    });
  }
});

// ── Abort from every state ──────────────────────────────────────────

describe('abort', () => {
  const allStates = [...INTERACTION_STATES];

  for (const state of allStates) {
    it(`from ${state} → idle`, () => {
      _forceStateForTesting(state);
      abort('test');
      expect(getState()).toBe('idle');
    });
  }
});

// ── Scenario tests ──────────────────────────────────────────────────

describe('scenarios', () => {
  it('happy path: full voice round-trip', () => {
    expect(dispatch('start_user_turn', 'voice_button')).toBe(true);
    expect(getState()).toBe('user_turn');

    expect(dispatch('start_agent_turn', 'stt_result')).toBe(true);
    expect(getState()).toBe('agent_turn');

    expect(dispatch('complete_turn', 'tts_drained')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('happy path: text-only round-trip (no mic)', () => {
    // Text submit goes directly from idle to agent_turn
    expect(dispatch('start_agent_turn', 'typed')).toBe(true);
    expect(getState()).toBe('agent_turn');

    expect(dispatch('complete_turn', 'done')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('error from agent_turn, then recovery', () => {
    _forceStateForTesting('agent_turn');

    expect(dispatch('error', 'ws_disconnected')).toBe(true);
    expect(getState()).toBe('error');

    expect(dispatch('recover', 'user_retry')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('error from idle is rejected', () => {
    expect(dispatch('error', 'spurious')).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('error from user_turn', () => {
    _forceStateForTesting('user_turn');

    expect(dispatch('error', 'mic_failed')).toBe(true);
    expect(getState()).toBe('error');
  });

  it('abort during agent_turn', () => {
    _forceStateForTesting('agent_turn');
    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort during user_turn', () => {
    _forceStateForTesting('user_turn');
    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort from idle is a no-op (stays idle)', () => {
    abort('test');
    expect(getState()).toBe('idle');
  });

  it('cannot start_user_turn from agent_turn', () => {
    _forceStateForTesting('agent_turn');
    expect(dispatch('start_user_turn', 'test')).toBe(false);
    expect(getState()).toBe('agent_turn');
  });

  it('cannot complete_turn from user_turn', () => {
    _forceStateForTesting('user_turn');
    expect(dispatch('complete_turn', 'test')).toBe(false);
    expect(getState()).toBe('user_turn');
  });

  it('cannot complete_turn from idle', () => {
    expect(dispatch('complete_turn', 'test')).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('start_agent_turn from idle (busy on load)', () => {
    expect(dispatch('start_agent_turn', 'session_busy_on_load')).toBe(true);
    expect(getState()).toBe('agent_turn');
  });

  it('full voice loop: two consecutive turns', () => {
    // Turn 1: voice
    expect(dispatch('start_user_turn', 'voice_button')).toBe(true);
    expect(dispatch('start_agent_turn', 'send_message')).toBe(true);
    expect(dispatch('complete_turn', 'tts_drained')).toBe(true);
    expect(getState()).toBe('idle');

    // Turn 2: voice loop auto-restart
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'send_message')).toBe(true);
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('recover only works from error', () => {
    expect(dispatch('recover', 'test')).toBe(false);
    expect(getState()).toBe('idle');

    _forceStateForTesting('user_turn');
    expect(dispatch('recover', 'test')).toBe(false);

    _forceStateForTesting('agent_turn');
    expect(dispatch('recover', 'test')).toBe(false);
  });
});
