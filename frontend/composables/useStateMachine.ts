/**
 * 4-state turn lifecycle state machine for the voilot interaction loop.
 *
 * States: idle, user_turn, agent_turn, error
 *
 * Audio/media concerns (mic, TTS, STT) are tracked by reactive booleans
 * in their respective composables — NOT as state machine states.
 *
 * `dispatch(action, trigger)` checks whether the action is valid in the
 * current state and transitions if so. Returns true on success.
 * `abort(trigger)` force-resets to idle from any state.
 */

import { setDebugLogStateAccessor, useDebugLog } from './useDebugLog';

// ── States ──────────────────────────────────────────────────────────

export const INTERACTION_STATES = [
  'idle',
  'user_turn',
  'agent_turn',
  'awaiting_input',
  'error',
] as const;

export type InteractionState = typeof INTERACTION_STATES[number];

// ── Actions ─────────────────────────────────────────────────────────

export const ACTION_NAMES = [
  'start_user_turn',
  'start_agent_turn',
  'complete_turn',
  'await_input',
  'answer_input',
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
  start_user_turn:  { from: ['idle', 'awaiting_input'], to: 'user_turn' },
  start_agent_turn: { from: ['user_turn', 'idle'],      to: 'agent_turn' },
  complete_turn:    { from: ['agent_turn'],              to: 'idle' },
  await_input:      { from: ['agent_turn'],              to: 'awaiting_input' },
  answer_input:     { from: ['awaiting_input'],          to: 'agent_turn' },
  error:            { from: INTERACTION_STATES.filter(s => s !== 'idle'), to: 'error' },
  recover:          { from: ['error'],                   to: 'idle' },
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
 * Composable that provides reactive state access.
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

  return {
    state: readonly(_state) as Readonly<Ref<InteractionState>>,
    dispatch,
    abort,
    getState,
  };
}

// ── Testing helpers ─────────────────────────────────────────────────

export function _forceStateForTesting(state: InteractionState): void {
  _ensureState().value = state;
}

export { ACTION_TABLE };
