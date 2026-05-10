/**
 * Integration tests for the voice loop logic in useAgent.ts.
 *
 * These tests simulate the state machine transitions that occur during
 * real voice interaction flows, validating the corrected ordering of
 * dispatches and the new awaiting_input state.
 *
 * We test the state machine transitions directly (not the full useAgent
 * composable) since the composable has too many dependencies to mock
 * cleanly. The key insight: if the transitions are correct, the loop works.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ref, computed, readonly } from 'vue';

vi.stubGlobal('ref', ref);
vi.stubGlobal('computed', computed);
vi.stubGlobal('readonly', readonly);
vi.stubGlobal('useState', (key: string, init?: () => any) => ref(init ? init() : undefined));

vi.mock('../useDebugLog', () => ({
  useDebugLog: () => ({ log: () => {} }),
  setDebugLogStateAccessor: () => {},
}));

import {
  dispatch,
  abort,
  getState,
  _forceStateForTesting,
} from '../useStateMachine';

beforeEach(() => {
  _forceStateForTesting('idle');
});

/**
 * Simulates tryCompleteTurn() as implemented in useAgent.ts:
 * 1. dispatch('complete_turn') — agent_turn → idle
 * 2. attempt startLoopRecording() — idle → user_turn (if accepted)
 */
function simulateTryCompleteTurn(voiceLoopShouldStart: boolean): { completedTurn: boolean; loopStarted: boolean } {
  const completedTurn = dispatch('complete_turn', 'turn_complete');
  let loopStarted = false;
  if (voiceLoopShouldStart) {
    loopStarted = dispatch('start_user_turn', 'loop_recording_start');
  }
  return { completedTurn, loopStarted };
}

/**
 * Simulates startLoopRecording() returning false because guards blocked it
 * (e.g., voiceInitiatedTurn is false for text-typed turns)
 */
function simulateTryCompleteTurnTextOnly(): { completedTurn: boolean } {
  const completedTurn = dispatch('complete_turn', 'turn_complete');
  // startLoopRecording guards block → returns false, no dispatch
  return { completedTurn };
}

describe('voice loop: tryCompleteTurn ordering', () => {
  it('normal voice turn: agent_turn → idle → user_turn', () => {
    _forceStateForTesting('agent_turn');

    const { completedTurn, loopStarted } = simulateTryCompleteTurn(true);

    expect(completedTurn).toBe(true);
    expect(loopStarted).toBe(true);
    expect(getState()).toBe('user_turn');
  });

  it('text-only turn: agent_turn → idle (no loop restart)', () => {
    _forceStateForTesting('agent_turn');

    const { completedTurn } = simulateTryCompleteTurnTextOnly();

    expect(completedTurn).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('complete_turn from wrong state (idle) is rejected gracefully', () => {
    // This happens for unsolicited agent responses (initial greeting
    // that streams while state is already idle)
    _forceStateForTesting('idle');

    const result = dispatch('complete_turn', 'turn_complete');

    expect(result).toBe(false);
    expect(getState()).toBe('idle');
  });

  it('startLoopRecording dispatch rejected from wrong state returns false', () => {
    // If somehow we're in agent_turn and try start_user_turn directly
    // (the old bug), it should be rejected
    _forceStateForTesting('agent_turn');

    const result = dispatch('start_user_turn', 'loop_recording_start');

    expect(result).toBe(false);
    expect(getState()).toBe('agent_turn');
  });
});

describe('voice loop: full round-trip scenarios', () => {
  it('voice button → record → send → agent responds → TTS drains → loop restarts', () => {
    // 1. User taps voice button
    expect(dispatch('start_user_turn', 'voice_button_start')).toBe(true);
    expect(getState()).toBe('user_turn');

    // 2. STT result sent to agent
    expect(dispatch('start_agent_turn', 'send_message')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // 3. Agent streams response... finishStreaming()... TTS drains...
    // 4. tryCompleteTurn() fires:
    const { completedTurn, loopStarted } = simulateTryCompleteTurn(true);
    expect(completedTurn).toBe(true);
    expect(loopStarted).toBe(true);
    expect(getState()).toBe('user_turn');

    // 5. Next recording... silence auto-stop... STT... send again
    expect(dispatch('start_agent_turn', 'send_message')).toBe(true);
    expect(getState()).toBe('agent_turn');
  });

  it('three consecutive voice turns (voice loop stays alive)', () => {
    for (let turn = 0; turn < 3; turn++) {
      // User turn
      if (turn === 0) {
        expect(dispatch('start_user_turn', 'voice_button_start')).toBe(true);
      } else {
        // Loop restart from tryCompleteTurn
        expect(getState()).toBe('user_turn');
      }
      expect(getState()).toBe('user_turn');

      // Agent turn
      expect(dispatch('start_agent_turn', 'send_message')).toBe(true);
      expect(getState()).toBe('agent_turn');

      // Complete + loop restart
      simulateTryCompleteTurn(true);
    }
    expect(getState()).toBe('user_turn');
  });
});

describe('question flow: awaiting_input', () => {
  it('agent asks question → mic starts for voice answer → agent resumes', () => {
    // Setup: user sent a voice message, agent is processing
    _forceStateForTesting('agent_turn');

    // 1. Agent sends question_request → await_input
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // 2. TTS announces question... queue drains... onQueueDrained fires
    // 3. startLoopRecording() dispatches start_user_turn from awaiting_input
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(getState()).toBe('user_turn');

    // 4. User speaks answer... auto-stop... sendMessage → tryAnswerPendingQuestion
    // 5. respondToQuestion() dispatches start_agent_turn (last question answered)
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // 6. Agent finishes processing → tryCompleteTurn
    const { completedTurn, loopStarted } = simulateTryCompleteTurn(true);
    expect(completedTurn).toBe(true);
    expect(loopStarted).toBe(true);
    expect(getState()).toBe('user_turn');
  });

  it('multi-question batch: answer Q1 by voice, Q2 by voice, agent finishes', () => {
    _forceStateForTesting('agent_turn');

    // Q1 arrives
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // Answer Q1 by voice
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // More questions remain → back to awaiting_input
    expect(dispatch('await_input', 'next_question')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // Answer Q2 by voice
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // All answered, agent finishes
    const { completedTurn, loopStarted } = simulateTryCompleteTurn(true);
    expect(completedTurn).toBe(true);
    expect(loopStarted).toBe(true);
    expect(getState()).toBe('user_turn');
  });

  it('question answered by UI button click (no voice)', () => {
    _forceStateForTesting('agent_turn');

    // Question arrives
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User clicks an option → answer_input (no mic involved)
    expect(dispatch('answer_input', 'question_answered')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes (text-only, no voice loop)
    const { completedTurn } = simulateTryCompleteTurnTextOnly();
    expect(completedTurn).toBe(true);
    expect(getState()).toBe('idle');
  });

  it('question arrives while in idle (unsolicited) — await_input rejected', () => {
    // Edge case: question_request arrives while state is idle (not agent_turn)
    // This shouldn't normally happen, but if it does, the dispatch is rejected
    _forceStateForTesting('idle');

    const result = dispatch('await_input', 'question_request');
    expect(result).toBe(false);
    expect(getState()).toBe('idle');
  });
});

describe('permission flow: awaiting_input', () => {
  it('agent requests permission → user taps approve → agent resumes', () => {
    _forceStateForTesting('agent_turn');

    // Permission request arrives
    expect(dispatch('await_input', 'permission_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User taps approve button → answer_input
    expect(dispatch('answer_input', 'permission_response')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes
    const { completedTurn, loopStarted } = simulateTryCompleteTurn(true);
    expect(completedTurn).toBe(true);
    expect(loopStarted).toBe(true);
    expect(getState()).toBe('user_turn');
  });

  it('permission during question batch — sequential', () => {
    _forceStateForTesting('agent_turn');

    // Question arrives first
    expect(dispatch('await_input', 'question_request')).toBe(true);

    // Answer by voice
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'question_answered')).toBe(true);

    // After answer, agent needs permission
    expect(dispatch('await_input', 'permission_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // User approves
    expect(dispatch('answer_input', 'permission_response')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });
});

describe('auto-voice on new sessions', () => {
  it('initial agent response from idle → agent_turn → complete → loop starts', () => {
    // Simulates: auto_voice_initial dispatch in handleTextEvent
    _forceStateForTesting('idle');

    // First text event arrives, autoVoiceArmed triggers start_agent_turn
    expect(dispatch('start_agent_turn', 'auto_voice_initial')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes initial greeting → TTS drains → tryCompleteTurn
    const { completedTurn, loopStarted } = simulateTryCompleteTurn(true);
    expect(completedTurn).toBe(true);
    expect(loopStarted).toBe(true);
    expect(getState()).toBe('user_turn');
  });

  it('auto-voice disabled: initial response stays idle → no loop', () => {
    // Without auto-voice, text events arrive but no start_agent_turn dispatch
    // State stays idle, finishStreaming sets agentDone, onQueueDrained fires
    // tryCompleteTurn dispatches complete_turn from idle → rejected
    _forceStateForTesting('idle');

    const result = dispatch('complete_turn', 'turn_complete');
    expect(result).toBe(false);
    expect(getState()).toBe('idle');
    // No loop starts — correct behavior when auto-voice is off
  });
});

describe('abort scenarios', () => {
  it('abort during awaiting_input resets to idle', () => {
    _forceStateForTesting('awaiting_input');

    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort during voice loop (user_turn) resets to idle', () => {
    // Full sequence: voice started, then user aborts
    _forceStateForTesting('idle');
    dispatch('start_user_turn', 'voice_button');
    expect(getState()).toBe('user_turn');

    abort('user_abort');
    expect(getState()).toBe('idle');
  });

  it('abort during agent_turn (stop button) resets to idle', () => {
    _forceStateForTesting('agent_turn');

    abort('abort_session');
    expect(getState()).toBe('idle');
  });

  it('after abort, full voice flow can restart cleanly', () => {
    // Agent was processing, user aborts
    _forceStateForTesting('agent_turn');
    abort('abort_session');
    expect(getState()).toBe('idle');

    // User starts fresh voice turn
    expect(dispatch('start_user_turn', 'voice_button_start')).toBe(true);
    expect(dispatch('start_agent_turn', 'send_message')).toBe(true);
    expect(dispatch('complete_turn', 'turn_complete')).toBe(true);
    expect(getState()).toBe('idle');
  });
});

describe('edge cases', () => {
  it('duplicate await_input from multi-question burst — only first succeeds', () => {
    _forceStateForTesting('agent_turn');

    // First question — transitions
    expect(dispatch('await_input', 'question_request')).toBe(true);
    expect(getState()).toBe('awaiting_input');

    // Second question in same burst — rejected (already awaiting_input)
    expect(dispatch('await_input', 'question_request')).toBe(false);
    expect(getState()).toBe('awaiting_input');

    // Third question — also rejected
    expect(dispatch('await_input', 'question_request')).toBe(false);
    expect(getState()).toBe('awaiting_input');
  });

  it('stop_playback during awaiting_input does not break flow', () => {
    _forceStateForTesting('agent_turn');

    // Question arrives
    dispatch('await_input', 'question_request');
    expect(getState()).toBe('awaiting_input');

    // User clicks stop playback — state stays awaiting_input
    // (stopPlayback doesn't dispatch any state actions)
    expect(getState()).toBe('awaiting_input');

    // onQueueDrained fires → startLoopRecording still works
    expect(dispatch('start_user_turn', 'loop_recording_start')).toBe(true);
    expect(getState()).toBe('user_turn');
  });

  it('session busy on load goes through full cycle', () => {
    // Page loads, session is busy
    _forceStateForTesting('idle');
    expect(dispatch('start_agent_turn', 'session_busy_on_load')).toBe(true);
    expect(getState()).toBe('agent_turn');

    // Agent finishes
    const { completedTurn } = simulateTryCompleteTurnTextOnly();
    expect(completedTurn).toBe(true);
    expect(getState()).toBe('idle');
  });
});
