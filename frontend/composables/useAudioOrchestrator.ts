/**
 * Audio orchestrator — owns all audio/voice/TTS coordination.
 *
 * Listens to state machine transitions (via onTransition) and reactive
 * refs to fire the correct audio cues, manage the working hum, control
 * TTS playback, and drive the voice recording loop.
 *
 * Sections:
 *   1. Transition handler (SM hook)
 *   2. Reactive watchers (TTS, connection, voiceEnabled)
 *   3. Voice loop control
 *   4. TTS pipeline (feed/flush/notify)
 *   5. Public API
 */

import type { Ref, ComputedRef } from 'vue';

import {
  onTransition,
  dispatch,
  getState,
  type TransitionInfo,
} from './useStateMachine';

import {
  playHandoff,
  startWorkingHum,
  stopWorkingHum,
  playSuccessChime,
  playQuestionChime,
  playPermissionChime,
  playCancelTone,
  playLoopListeningTick,
  startWatchdog,
  cancelWatchdog,
  isHumActive,
  setTTSEnqueue,
  notifyToolActivity,
  stopAll,
  playWarningTone,
  playReconnectChime,
} from './useAudioFeedback';

import { useTTS } from './useTTS';

// ── Types ───────────────────────────────────────────────────────────

interface TTSHandle {
  enqueue: (text: string) => void;
  stop: () => void;
  isPlaying: Readonly<Ref<boolean>>;
}

interface AudioOrchestratorOptions {
  sessionId: string;
  sendMessage: (text: string) => void;
  hasPendingQuestion: ComputedRef<boolean>;
  hasPendingPermission: ComputedRef<boolean>;
  connectionState: Ref<string>;
  /** External voiceEnabled ref — gates all audio output. */
  voiceEnabled?: Ref<boolean>;
  /** Optional external TTS handle. If omitted, orchestrator creates its own. */
  tts?: TTSHandle;
}

// ── Triggers that indicate a fresh agent turn (not returning from input) ──

const FRESH_TURN_TRIGGERS = new Set([
  'send_message',
  'session_busy_on_load',
  'auto_voice_start',
  'auto_voice_resume',
]);

const RETURNING_FROM_INPUT_TRIGGERS = new Set([
  'permission_response',
  'question_answered',
]);

// ── Composable ──────────────────────────────────────────────────────

export function useAudioOrchestrator(options: AudioOrchestratorOptions) {
  const { connectionState } = options;

  // ── Internal state ──────────────────────────────────────────────

  const tts: TTSHandle = options.tts ?? useTTS();
  let _autoVoiceArmed = false;
  let _agentDone = false;
  const { suppressInitialTools } = useSuppressInitialTools();
  const _voiceInitiatedTurn = ref(false);
  const _voiceEnabled = options.voiceEnabled ?? ref(false);

  // Wire TTS enqueue to audio feedback (for check-in announcements)
  setTTSEnqueue(tts.enqueue);

  // ── Section 1: Transition handler ─────────────────────────────

  function handleTransition(info: TransitionInfo): void {
    const { from, to, action, trigger } = info;

    // --- Abort (special case) ---
    if (action === 'abort') {
      handleAbortTransition(from);
      return;
    }

    // All non-abort audio is gated on voiceEnabled
    if (!_voiceEnabled.value) return;

    // --- Entering agent_turn ---
    if (to === 'agent_turn') {
      handleEnterAgentTurn(from, trigger);
      return;
    }

    // --- Leaving agent_turn → idle (complete_turn) ---
    if (from === 'agent_turn' && to === 'idle') {
      handleAgentTurnComplete();
      return;
    }

    // --- Leaving agent_turn → awaiting_input ---
    if (from === 'agent_turn' && to === 'awaiting_input') {
      handleAwaitInput(trigger);
      return;
    }

    // --- Entering user_turn ---
    if (to === 'user_turn') {
      handleEnterUserTurn(trigger);
      return;
    }
  }

  function handleEnterAgentTurn(from: string, trigger?: string): void {
    // Consume autoVoiceArmed on any → agent_turn transition
    if (_autoVoiceArmed) {
      _autoVoiceArmed = false;
      _voiceInitiatedTurn.value = true;
    }

    // Returning from input — no audio cues
    if (trigger && RETURNING_FROM_INPUT_TRIGGERS.has(trigger)) {
      return;
    }

    if (trigger === 'send_message') {
      playHandoff();
      startWorkingHum();
      startWatchdog();
    } else if (trigger === 'session_busy_on_load') {
      // Hum only — no handoff (we didn't send the message),
      // no watchdog (we don't know when the turn started)
      startWorkingHum();
    } else if (FRESH_TURN_TRIGGERS.has(trigger ?? '')) {
      startWorkingHum();
    }
  }

  function handleAgentTurnComplete(): void {
    cancelWatchdog();

    if (isHumActive()) {
      stopWorkingHum();
    }

    if (!suppressInitialTools.value) {
      playSuccessChime();
    }
  }

  function handleAwaitInput(trigger?: string): void {
    stopWorkingHum();
    cancelWatchdog();

    if (trigger === 'permission_request') {
      playPermissionChime();
    } else if (trigger === 'question_request') {
      playQuestionChime();
    }
  }

  function handleEnterUserTurn(trigger?: string): void {
    if (trigger === 'loop_recording_start') {
      playLoopListeningTick();
    }
  }

  function handleAbortTransition(from: string): void {
    cancelWatchdog();

    if (!_voiceEnabled.value) return;

    playCancelTone();
    tts.stop();
    tts.enqueue('Stopped.');

    if (isHumActive()) {
      stopWorkingHum();
    }
  }

  // Register the listener
  const unsubTransition = onTransition(handleTransition);

  // ── Section 2: Reactive watchers ──────────────────────────────

  // TTS playing → stop hum
  const unwatchTTS = watch(tts.isPlaying, (playing: boolean) => {
    if (playing && isHumActive()) {
      stopWorkingHum();
    }

    // TTS stopped + agent done → complete the turn
    if (!playing && _agentDone) {
      if (getState() === 'agent_turn') {
        dispatch('complete_turn', 'tts_drain_complete');
      }
    }
  });

  // Connection state changes
  const unwatchConnection = watch(connectionState, (newState: string, oldState: string) => {
    if (newState === 'disconnected' && oldState !== 'disconnected') {
      playWarningTone();
      tts.enqueue('Connection lost.');
    } else if (newState === 'connected' && oldState !== 'connected') {
      playReconnectChime();
      tts.enqueue('Reconnected.');
    }
  });

  // Voice toggled off → stop TTS
  const unwatchVoice = watch(_voiceEnabled, (enabled: boolean) => {
    if (!enabled) {
      tts.stop();
    }
  });

  // ── Section 3: TTS pipeline ───────────────────────────────────

  function notifyStreamingDone(context: { abortedTurn: boolean }): void {
    _agentDone = true;

    // If TTS is not playing, complete immediately
    if (!tts.isPlaying.value && getState() === 'agent_turn' && !context.abortedTurn) {
      dispatch('complete_turn', 'streaming_done_no_tts');
    }
  }

  // ── Section 5: Lifecycle ──────────────────────────────────────

  function cleanup(): void {
    unsubTransition();
    unwatchTTS();
    unwatchConnection();
    unwatchVoice();
  }

  // ── Public API ────────────────────────────────────────────────

  return {
    // Refs
    voiceInitiatedTurn: readonly(_voiceInitiatedTurn),
    isTTSPlaying: tts.isPlaying,

    // TTS pipeline
    notifyStreamingDone,

    // Lifecycle
    cleanup,

    // Testing helpers
    _setAutoVoiceArmedForTesting(v: boolean) { _autoVoiceArmed = v; },
    _getAutoVoiceArmedForTesting() { return _autoVoiceArmed; },
    _setVoiceEnabledForTesting(v: boolean) { _voiceEnabled.value = v; },
  };
}
