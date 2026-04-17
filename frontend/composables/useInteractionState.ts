/**
 * Interaction state machine for the voilot voice interaction lifecycle.
 *
 * Tracks the full round-trip: mic acquisition → recording → STT →
 * agent submission → streaming → TTS playback → turn completion.
 *
 * Single reactive state ref with validated transitions. Invalid transitions
 * are rejected and logged as warnings. All transitions are logged to the
 * debug log.
 */

import { setDebugLogStateAccessor, useDebugLog } from './useDebugLog';

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

/** Map of each state to its allowed target states. */
const ALLOWED_TRANSITIONS: Record<InteractionState, readonly InteractionState[]> = {
  'idle':                      ['mic:acquiring', 'agent:submitting'],
  'mic:acquiring':             ['mic:monitoring', 'mic:recording', 'error', 'idle'],
  'mic:monitoring':            ['mic:recording', 'idle', 'error'],
  'mic:recording':             ['stt:transcribing', 'idle', 'error'],
  'stt:transcribing':          ['agent:submitting', 'idle', 'error'],
  'agent:submitting':          ['agent:streaming', 'error'],
  'agent:streaming':           ['agent:awaiting-question', 'agent:awaiting-permission', 'tts:speaking', 'turn:completing', 'error', 'idle'],
  'agent:awaiting-question':   ['agent:streaming', 'idle', 'error'],
  'agent:awaiting-permission': ['agent:streaming', 'idle', 'error'],
  'tts:speaking':              ['turn:completing', 'mic:recording', 'idle', 'error'],
  'turn:completing':           ['mic:acquiring', 'idle', 'tts:speaking'],
  'error':                     ['idle', 'mic:acquiring'],
};

export interface InteractionStateMetadata {
  errorSource?: string;
  errorMessage?: string;
  trigger?: string;
}

// Module-level singleton state
let _state: Ref<InteractionState> | null = null;
let _metadata: InteractionStateMetadata = {};
let _stateAccessorRegistered = false;

// ── Module-level transition helpers ─────────────────────────────────
//
// These allow composables that run outside Vue setup() (useVoice,
// useTTS, useAudioFeedback) to drive state transitions without
// needing inject/provide or passing the composable around.

function _ensureState(): Ref<InteractionState> {
  if (!_state) {
    // This branch is only hit if transitionState / resetState is called
    // before useInteractionState() — defensive fallback.
    _state = ref('idle') as Ref<InteractionState>;
  }
  return _state;
}

function _logTransition(
  level: 'info' | 'warn',
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

/**
 * Drive a state transition from anywhere (inside or outside setup).
 * Returns true if the transition was accepted.
 */
export function transitionState(
  to: InteractionState,
  trigger: string,
  metadata?: Partial<InteractionStateMetadata>,
): boolean {
  const state = _ensureState();
  const from = state.value;

  if (from === to) return true;

  const allowed = ALLOWED_TRANSITIONS[from];
  if (!allowed.includes(to)) {
    _logTransition('warn', 'invalid_transition', { from, to, trigger });
    return false;
  }

  _metadata = { trigger, ...metadata };
  state.value = to;

  _logTransition('info', 'transition', {
    from,
    to,
    trigger,
    ...(metadata || {}),
  });

  return true;
}

/**
 * Force-reset to idle from anywhere (abort / cleanup scenarios).
 */
export function resetState(trigger: string): void {
  const state = _ensureState();
  const from = state.value;
  _metadata = { trigger };
  state.value = 'idle';

  _logTransition('info', 'reset', { from, trigger });
}

/**
 * Get the current interaction state value from anywhere.
 */
export function getInteractionState(): InteractionState {
  return _ensureState().value;
}

export function useInteractionState() {
  if (!_state) {
    _state = useState<InteractionState>('interaction-state', () => 'idle');
  }

  const { log } = useDebugLog();

  // Register state accessor for debug log entries (once)
  if (!_stateAccessorRegistered) {
    setDebugLogStateAccessor(() => _state!.value);
    _stateAccessorRegistered = true;
  }

  /**
   * Transition to a new state. Returns true if the transition was accepted.
   * Invalid transitions are rejected and logged as warnings.
   */
  function transition(
    to: InteractionState,
    trigger: string,
    metadata?: Partial<InteractionStateMetadata>,
  ): boolean {
    return transitionState(to, trigger, metadata);
  }

  /** Get the current state metadata. */
  function getMetadata(): InteractionStateMetadata {
    return { ..._metadata };
  }

  /** Force-reset to idle (for abort/cleanup scenarios). */
  function reset(trigger: string) {
    resetState(trigger);
  }

  // Computed backward-compatibility flags
  const isRecording = computed(() => _state!.value === 'mic:recording');
  const isMonitoring = computed(() => _state!.value === 'mic:monitoring');
  const isTranscribing = computed(() => _state!.value === 'stt:transcribing');
  const isStreaming = computed(() => _state!.value === 'agent:streaming');
  const isSpeaking = computed(() => _state!.value === 'tts:speaking');
  const isIdle = computed(() => _state!.value === 'idle');
  const isError = computed(() => _state!.value === 'error');
  const hasPendingQuestion = computed(() => _state!.value === 'agent:awaiting-question');
  const hasPendingPermission = computed(() => _state!.value === 'agent:awaiting-permission');

  return {
    state: readonly(_state),
    transition,
    reset,
    getMetadata,
    // Backward-compatible computed flags
    isRecording,
    isMonitoring,
    isTranscribing,
    isStreaming,
    isSpeaking,
    isIdle,
    isError,
    hasPendingQuestion,
    hasPendingPermission,
  };
}
