/**
 * Shared reactive state for cross-composable communication.
 *
 * Each ref has a single owner (the composable that writes to it).
 * Other composables read via the exported readonly refs.
 *
 * Owner legend:
 *   useAgent          → isStreaming, suppressInitialTools
 *   useAudioOrchestrator → voiceEnabled, voiceInitiatedTurn, loopRecordingActive
 */

/**
 * Whether the agent is currently streaming a response.
 * Owner: useAgent
 */
export function useIsStreaming() {
  const _ref = useState<boolean>('session-is-streaming', () => false);
  return {
    isStreaming: readonly(_ref),
    _setIsStreaming: (v: boolean) => { _ref.value = v; },
  };
}

/**
 * Whether the current turn was initiated by voice (affects loop restart + TTS).
 * Owner: useAudioOrchestrator
 */
export function useVoiceInitiatedTurn() {
  const _ref = useState<boolean>('session-voice-initiated-turn', () => false);
  return {
    voiceInitiatedTurn: readonly(_ref),
    _setVoiceInitiatedTurn: (v: boolean) => { _ref.value = v; },
  };
}

/**
 * Whether voice mode is enabled.
 * Owner: useAudioOrchestrator
 */
export function useVoiceEnabled() {
  const _ref = useState<boolean>('session-voice-enabled', () => false);
  return {
    voiceEnabled: readonly(_ref),
    _setVoiceEnabled: (v: boolean) => { _ref.value = v; },
  };
}

/**
 * Whether the voice loop is actively recording.
 * Owner: useAudioOrchestrator
 */
export function useLoopRecordingActive() {
  const _ref = useState<boolean>('session-loop-recording-active', () => false);
  return {
    loopRecordingActive: readonly(_ref),
    _setLoopRecordingActive: (v: boolean) => { _ref.value = v; },
  };
}

/**
 * Whether initial tool events should be suppressed (e.g. session load).
 * Owner: useAgent
 */
export function useSuppressInitialTools() {
  const _ref = useState<boolean>('session-suppress-initial-tools', () => true);
  return {
    suppressInitialTools: readonly(_ref),
    _setSuppressInitialTools: (v: boolean) => { _ref.value = v; },
  };
}
