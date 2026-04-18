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
    expect(dispatch('acquire_mic', 'voice_button')).toBe(true);
    expect(getState()).toBe('mic:acquiring');

    expect(dispatch('start_recording', 'mic_ready')).toBe(true);
    expect(getState()).toBe('mic:recording');

    expect(dispatch('stop_recording', 'silence_detected')).toBe(true);
    expect(getState()).toBe('stt:transcribing');

    expect(dispatch('submit_message', 'stt_result')).toBe(true);
    expect(getState()).toBe('agent:submitting');

    expect(dispatch('start_streaming', 'first_event')).toBe(true);
    expect(getState()).toBe('agent:streaming');

    expect(dispatch('start_tts', 'playback_started')).toBe(true);
    expect(getState()).toBe('tts:speaking');

    expect(dispatch('finish_streaming', 'status_idle')).toBe(true);
    expect(getState()).toBe('turn:completing');

    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('happy path: text-only round-trip (no mic)', () => {
    expect(dispatch('submit_message', 'typed')).toBe(true);
    expect(dispatch('start_streaming', 'first_event')).toBe(true);
    expect(dispatch('finish_streaming', 'done')).toBe(true);
    expect(dispatch('complete_turn', 'no_voice')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('duplicate finish_streaming is structurally rejected', () => {
    // Set up: agent streaming, TTS playing
    _forceStateForTesting('tts:speaking');

    // First finish_streaming: tts:speaking → turn:completing (valid)
    expect(dispatch('finish_streaming', 'status_idle')).toBe(true);
    expect(getState()).toBe('turn:completing');

    // Second finish_streaming: turn:completing → rejected
    expect(dispatch('finish_streaming', 'done_event')).toBe(false);
    expect(getState()).toBe('turn:completing');
  });

  it('finish_streaming from agent:streaming (TTS not yet started)', () => {
    _forceStateForTesting('agent:streaming');

    expect(dispatch('finish_streaming', 'status_idle')).toBe(true);
    expect(getState()).toBe('turn:completing');

    // Second call rejected
    expect(dispatch('finish_streaming', 'done_event')).toBe(false);
    expect(getState()).toBe('turn:completing');
  });

  it('TTS drain drives turn completion', () => {
    _forceStateForTesting('tts:speaking');

    expect(dispatch('drain_tts', 'queue_drained')).toBe(true);
    expect(getState()).toBe('turn:completing');

    expect(dispatch('complete_turn', 'no_loop')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('TTS starts during turn:completing (flushed chunks)', () => {
    _forceStateForTesting('turn:completing');

    // Flushed chunker enqueued more text, TTS starts playing
    expect(dispatch('start_tts', 'playback_started')).toBe(true);
    expect(getState()).toBe('tts:speaking');

    // TTS finishes
    expect(dispatch('drain_tts', 'queue_drained')).toBe(true);
    expect(getState()).toBe('turn:completing');
  });

  it('voice loop restart: turn:completing → mic:acquiring', () => {
    _forceStateForTesting('turn:completing');

    expect(dispatch('acquire_mic', 'loop_restart')).toBe(true);
    expect(getState()).toBe('mic:acquiring');
  });

  it('question interrupt and resume', () => {
    _forceStateForTesting('agent:streaming');

    expect(dispatch('enter_question', 'question_request')).toBe(true);
    expect(getState()).toBe('agent:awaiting-question');

    expect(dispatch('resolve_question', 'question_replied')).toBe(true);
    expect(getState()).toBe('agent:streaming');
  });

  it('permission interrupt and resume', () => {
    _forceStateForTesting('agent:streaming');

    expect(dispatch('enter_permission', 'permission_request')).toBe(true);
    expect(getState()).toBe('agent:awaiting-permission');

    expect(dispatch('resolve_permission', 'permission_replied')).toBe(true);
    expect(getState()).toBe('agent:streaming');
  });

  it('error from streaming, then recovery', () => {
    _forceStateForTesting('agent:streaming');

    expect(dispatch('error', 'ws_disconnected')).toBe(true);
    expect(getState()).toBe('error');

    expect(dispatch('recover', 'user_retry')).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('error from idle is rejected', () => {
    expect(dispatch('error', 'spurious')).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('error then retry via acquire_mic', () => {
    _forceStateForTesting('error');

    expect(dispatch('acquire_mic', 'retry')).toBe(true);
    expect(getState()).toBe('mic:acquiring');
  });

  it('abort during streaming stops everything', () => {
    _forceStateForTesting('agent:streaming');
    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort during tts:speaking', () => {
    _forceStateForTesting('tts:speaking');
    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort during mic:recording', () => {
    _forceStateForTesting('mic:recording');
    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort from idle is a no-op (stays idle)', () => {
    abort('test');
    expect(getState()).toBe('idle');
  });

  it('monitoring → recording transition', () => {
    _forceStateForTesting('mic:monitoring');

    expect(dispatch('start_recording', 'speech_detected')).toBe(true);
    expect(getState()).toBe('mic:recording');
  });

  it('cannot start recording from idle', () => {
    expect(dispatch('start_recording', 'test')).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('cannot submit message from agent:streaming', () => {
    _forceStateForTesting('agent:streaming');
    expect(dispatch('submit_message', 'test')).toBe(false);
    expect(getState()).toBe('agent:streaming');
  });

  it('cannot start TTS from idle', () => {
    expect(dispatch('start_tts', 'test')).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('full voice loop cycle: two consecutive turns', () => {
    // Turn 1
    expect(dispatch('acquire_mic', 'button')).toBe(true);
    expect(dispatch('start_recording', 'ready')).toBe(true);
    expect(dispatch('stop_recording', 'silence')).toBe(true);
    expect(dispatch('submit_message', 'stt')).toBe(true);
    expect(dispatch('start_streaming', 'event')).toBe(true);
    expect(dispatch('start_tts', 'play')).toBe(true);
    expect(dispatch('finish_streaming', 'done')).toBe(true);
    // TTS still has items, starts playing flushed chunks
    expect(dispatch('start_tts', 'flushed_play')).toBe(true);
    expect(dispatch('drain_tts', 'empty')).toBe(true);
    // Voice loop restarts
    expect(dispatch('acquire_mic', 'loop')).toBe(true);

    // Turn 2
    expect(dispatch('start_recording', 'ready')).toBe(true);
    expect(dispatch('stop_recording', 'silence')).toBe(true);
    expect(dispatch('submit_message', 'stt')).toBe(true);
    expect(dispatch('start_streaming', 'event')).toBe(true);
    expect(dispatch('finish_streaming', 'done')).toBe(true);
    expect(dispatch('complete_turn', 'no_tts')).toBe(true);
    expect(getState()).toBe('idle');
  });
});
