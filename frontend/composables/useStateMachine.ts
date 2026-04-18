/**
 * Action-gated state machine for the voilot interaction lifecycle.
 *
 * Callers submit named actions via `dispatch()`. The state machine checks
 * whether the action is valid in the current state and transitions if so.
 * Side effects remain in the calling composables — the state machine is
 * a pure gate with no dependencies on TTS, audio, voice, or agent logic.
 *
 * `abort()` is a special case that force-resets to `idle` from any state.
 */

import { setDebugLogStateAccessor, useDebugLog } from './useDebugLog';

// ── States ──────────────────────────────────────────────────────────

export const INTERACTION_STATES = [
  'idle',
  'mic:acquiring',
  'mic:monitoring',
  'mic:recording',
  'stt:transcribing',
  'agent:submitting',
  'agent:streaming',
  'agent:awaiting-question',
  'agent:awaiting-permission',
  'tts:speaking',
  'turn:completing',
  'error',
] as const;

export type InteractionState = typeof INTERACTION_STATES[number];

// ── Actions ─────────────────────────────────────────────────────────

export const ACTION_NAMES = [
  'acquire_mic',
  'start_monitoring',
  'start_recording',
  'stop_recording',
  'submit_message',
  'start_streaming',
  'finish_streaming',
  'start_tts',
  'drain_tts',
  'complete_turn',
  'enter_question',
  'resolve_question',
  'enter_permission',
  'resolve_permission',
  'error',
  'recover',
] as const;

export type ActionName = typeof ACTION_NAMES[number];

interface ActionDef {
  /** States from which this action may be dispatched. */
  from: readonly InteractionState[];
  /** Target state after a successful dispatch. */
  to: InteractionState;
}

const ACTION_TABLE: Record<ActionName, ActionDef> = {
  acquire_mic:       { from: ['idle', 'turn:completing', 'error'],   to: 'mic:acquiring' },
  start_monitoring:  { from: ['mic:acquiring'],                      to: 'mic:monitoring' },
  start_recording:   { from: ['mic:acquiring', 'mic:monitoring'],    to: 'mic:recording' },
  stop_recording:    { from: ['mic:recording'],                      to: 'stt:transcribing' },
  submit_message:    { from: ['idle', 'stt:transcribing'],           to: 'agent:submitting' },
  start_streaming:   { from: ['agent:submitting'],                   to: 'agent:streaming' },
  finish_streaming:  { from: ['agent:streaming', 'tts:speaking'],    to: 'turn:completing' },
  start_tts:         { from: ['agent:streaming', 'turn:completing'], to: 'tts:speaking' },
  drain_tts:         { from: ['tts:speaking'],                       to: 'turn:completing' },
  complete_turn:     { from: ['turn:completing'],                    to: 'idle' },
  enter_question:    { from: ['agent:streaming'],                    to: 'agent:awaiting-question' },
  resolve_question:  { from: ['agent:awaiting-question'],            to: 'agent:streaming' },
  enter_permission:  { from: ['agent:streaming'],                    to: 'agent:awaiting-permission' },
  resolve_permission:{ from: ['agent:awaiting-permission'],          to: 'agent:streaming' },
  error:             { from: INTERACTION_STATES.filter(s => s !== 'idle'), to: 'error' },
  recover:           { from: ['error'],                              to: 'idle' },
};

// ── Module-level singleton state ────────────────────────────────────

let _state: Ref<InteractionState> | null = null;
let _stateAccessorRegistered = false;

function _ensureState(): Ref<InteractionState> {
  if (!_state) {
    _state = ref('idle') as Ref<InteractionState>;
  }
  return _state;
}

function _log(
  level: 'info' | 'warn' | 'debug',
  event: string,
  data?: Record<string, unknown>,
) {
  try {
    const { log } = useDebugLog();
    log(level, 'state', event, data);
  } catch {
    // Outside setup — best effort
  }
}

// ── Public API ──────────────────────────────────────────────────────

/**
 * Dispatch a named action. Returns true if the action was accepted
 * (current state is in the action's `from` list), false otherwise.
 *
 * On success, the state transitions to the action's `to` state.
 * On failure, the state is unchanged and a warning is logged.
 */
export function dispatch(action: ActionName, trigger?: string): boolean {
  const state = _ensureState();
  const from = state.value;
  const def = ACTION_TABLE[action];

  if (!def.from.includes(from)) {
    _log('warn', 'action_rejected', { action, from, to: def.to, trigger });
    return false;
  }

  state.value = def.to;
  _log('info', 'action_dispatched', { action, from, to: def.to, trigger });
  return true;
}

/**
 * Force-reset to idle from any state. Bypasses the action table.
 * Used for abort/cleanup scenarios.
 */
export function abort(trigger: string): void {
  const state = _ensureState();
  const from = state.value;
  state.value = 'idle';
  _log('info', 'abort', { from, trigger });
}

/**
 * Get the current interaction state value.
 */
export function getState(): InteractionState {
  return _ensureState().value;
}

/**
 * Composable that provides reactive state access and computed convenience flags.
 */
export function useStateMachine() {
  if (!_state) {
    _state = useState<InteractionState>('interaction-state', () => 'idle');
  }

  // Register state accessor for debug log entries (once)
  if (!_stateAccessorRegistered) {
    try {
      setDebugLogStateAccessor(() => _state!.value);
      _stateAccessorRegistered = true;
    } catch {
      // Outside setup — best effort
    }
  }

  // Computed convenience flags
  const isRecording = computed(() => _state!.value === 'mic:recording');
  const isMonitoring = computed(() => _state!.value === 'mic:monitoring');
  const isTranscribing = computed(() => _state!.value === 'stt:transcribing');
  const isStreaming = computed(() => _state!.value === 'agent:streaming');
  const isSpeaking = computed(() => _state!.value === 'tts:speaking');
  const isIdle = computed(() => _state!.value === 'idle');
  const isError = computed(() => _state!.value === 'error');

  return {
    state: readonly(_state) as Readonly<Ref<InteractionState>>,
    dispatch,
    abort,
    getState,
    // Convenience flags
    isRecording,
    isMonitoring,
    isTranscribing,
    isStreaming,
    isSpeaking,
    isIdle,
    isError,
  };
}

// ── Testing helpers ─────────────────────────────────────────────────

/**
 * Force-set state to a specific value. ONLY for testing.
 */
export function _forceStateForTesting(state: InteractionState): void {
  _ensureState().value = state;
}

// Re-export for migration compatibility
export { ACTION_TABLE };
