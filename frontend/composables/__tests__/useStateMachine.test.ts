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
  onTransition,
  _forceStateForTesting,
  ACTION_TABLE,
  INTERACTION_STATES,
  type ActionName,
  type InteractionState,
  type TransitionInfo,
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

  it('error from awaiting_input', () => {
    _forceStateForTesting('awaiting_input');

    expect(dispatch('error', 'ws_disconnected')).toBe(true);
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

  it('abort during awaiting_input', () => {
    _forceStateForTesting('awaiting_input');
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

    _forceStateForTesting('awaiting_input');
    expect(dispatch('recover', 'test')).toBe(false);
  });

  // ── awaiting_input scenarios ──────────────────────────────────────

  it('question flow: agent asks, user answers by voice', () => {
    // Agent is processing
    _forceStateForTesting('agent_turn');

    // Agent sends question_request → await_input
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // TTS finishes, mic starts → start_user_turn from awaiting_input
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(getState()).toBe('user_turn');

    // User speaks answer → start_agent_turn
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('permission flow: agent asks, user taps button', () => {
    _forceStateForTesting('agent_turn');

    // Agent sends permission_request → await_input
    expect(dispatch('await_input', 'permission_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User taps approve → answer_input
    expect(dispatch('answer_input', 'permission_response')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('multi-question batch: answer Q1, then Q2', () => {
    _forceStateForTesting('agent_turn');

    // First question arrives
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User answers Q1 by voice
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Next question in batch
    expect(dispatch('await_input', 'next_question')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User answers Q2 by voice
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes after all questions answered
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('question answered by UI button (no voice)', () => {
    _forceStateForTesting('agent_turn');

    // Question arrives
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User clicks an option button → answer_input (same as permission)
    expect(dispatch('answer_input', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('cannot await_input from idle', () => {
    expect(dispatch('await_input', 'test')).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('cannot answer_input from agent_turn', () => {
    _forceStateForTesting('agent_turn');
    expect(dispatch('answer_input', 'test')).toBe(false);
    expect(getState()).toBe('agent_turn');
  });

  it('cannot await_input from awaiting_input (idempotent guard needed)', () => {
    _forceStateForTesting('awaiting_input');
    expect(dispatch('await_input', 'test')).toBe(false);
    expect(getState()).toBe('awaiting_input');
  });
});

// ── onTransition listener ───────────────────────────────────────────

describe('onTransition', () => {
  it('fires on successful dispatch with correct info', () => {
    const received: TransitionInfo[] = [];
    onTransition((info) => received.push(info));

    dispatch('start_agent_turn', 'test_trigger');

    expect(received).toHaveLength(1);
    expect(received[0]).toEqual({
      from: 'idle',
      to: 'agent_turn',
      action: 'start_agent_turn',
      trigger: 'test_trigger',
    });
  });

  it('does NOT fire on rejected dispatch', () => {
    const received: TransitionInfo[] = [];
    onTransition((info) => received.push(info));

    // idle → complete_turn is invalid
    dispatch('complete_turn', 'test');

    expect(received).toHaveLength(0);
  });

  it('fires on abort with action="abort"', () => {
    _forceStateForTesting('agent_turn');
    const received: TransitionInfo[] = [];
    onTransition((info) => received.push(info));

    abort('user_abort');

    expect(received).toHaveLength(1);
    expect(received[0]).toEqual({
      from: 'agent_turn',
      to: 'idle',
      action: 'abort',
      trigger: 'user_abort',
    });
  });

  it('unsubscribe stops notifications', () => {
    const received: TransitionInfo[] = [];
    const unsub = onTransition((info) => received.push(info));

    dispatch('start_agent_turn', 'first');
    expect(received).toHaveLength(1);

    unsub();

    _forceStateForTesting('idle');
    dispatch('start_agent_turn', 'second');
    expect(received).toHaveLength(1); // still 1
  });

  it('multiple listeners all fire', () => {
    let countA = 0;
    let countB = 0;
    onTransition(() => countA++);
    onTransition(() => countB++);

    dispatch('start_agent_turn', 'test');

    expect(countA).toBe(1);
    expect(countB).toBe(1);
  });

  it('listener error does not break state machine', () => {
    onTransition(() => { throw new Error('boom'); });
    const received: TransitionInfo[] = [];
    onTransition((info) => received.push(info));

    dispatch('start_agent_turn', 'test');

    expect(getState()).toBe('agent_turn');
    expect(received).toHaveLength(1);
  });

  it('listeners are cleared by _forceStateForTesting', () => {
    const received: TransitionInfo[] = [];
    onTransition((info) => received.push(info));

    _forceStateForTesting('idle');
    dispatch('start_agent_turn', 'test');

    expect(received).toHaveLength(0);
  });
});
