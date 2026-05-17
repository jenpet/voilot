/**
 * Transition → Audio mapping tests for useAudioOrchestrator.
 *
 * Tests the core contract: when a state machine transition fires,
 * the orchestrator calls the correct audio feedback functions.
 *
 * These are pure dispatch-and-assert tests — no Vue reactivity needed.
 * All audio/voice/TTS dependencies are mocked at the module level.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ref, computed, readonly } from 'vue';

// ── Nuxt auto-import stubs ──────────────────────────────────────────

vi.stubGlobal('ref', ref);
vi.stubGlobal('computed', computed);
vi.stubGlobal('readonly', readonly);
const _useStateCache = new Map<string, any>();
vi.stubGlobal('useState', (key: string, init?: () => any) => {
  if (!_useStateCache.has(key)) {
    _useStateCache.set(key, ref(init ? init() : undefined));
  }
  return _useStateCache.get(key)!;
});
vi.stubGlobal('watch', vi.fn().mockReturnValue(vi.fn()));

// ── Mock useDebugLog ────────────────────────────────────────────────

vi.mock('../useDebugLog', () => ({
  useDebugLog: () => ({ log: vi.fn() }),
  setDebugLogStateAccessor: vi.fn(),
}));

// ── Mock useAudioFeedback ───────────────────────────────────────────

const mockPlayHandoff = vi.fn();
const mockStartWorkingHum = vi.fn();
const mockStopWorkingHum = vi.fn().mockResolvedValue(undefined);
const mockPlaySuccessChime = vi.fn();
const mockPlayQuestionChime = vi.fn();
const mockPlayPermissionChime = vi.fn();
const mockPlayCancelTone = vi.fn();
const mockPlayLoopListeningTick = vi.fn();
const mockStartWatchdog = vi.fn();
const mockCancelWatchdog = vi.fn();
const mockSetTTSEnqueue = vi.fn();
const mockNotifyToolActivity = vi.fn();
const mockInitAudioFeedback = vi.fn();
const mockStopAll = vi.fn();
const mockIsHumActive = vi.fn().mockReturnValue(false);

vi.mock('../useAudioFeedback', () => ({
  playHandoff: (...args: any[]) => mockPlayHandoff(...args),
  startWorkingHum: (...args: any[]) => mockStartWorkingHum(...args),
  stopWorkingHum: (...args: any[]) => mockStopWorkingHum(...args),
  playSuccessChime: (...args: any[]) => mockPlaySuccessChime(...args),
  playQuestionChime: (...args: any[]) => mockPlayQuestionChime(...args),
  playPermissionChime: (...args: any[]) => mockPlayPermissionChime(...args),
  playCancelTone: (...args: any[]) => mockPlayCancelTone(...args),
  playLoopListeningTick: (...args: any[]) => mockPlayLoopListeningTick(...args),
  startWatchdog: (...args: any[]) => mockStartWatchdog(...args),
  cancelWatchdog: (...args: any[]) => mockCancelWatchdog(...args),
  setTTSEnqueue: (...args: any[]) => mockSetTTSEnqueue(...args),
  notifyToolActivity: (...args: any[]) => mockNotifyToolActivity(...args),
  initAudioFeedback: (...args: any[]) => mockInitAudioFeedback(...args),
  stopAll: (...args: any[]) => mockStopAll(...args),
  isHumActive: (...args: any[]) => mockIsHumActive(...args),
}));

// ── Mock useTTS ─────────────────────────────────────────────────────

const mockTTSEnqueue = vi.fn();
const mockTTSStop = vi.fn();
const mockTTSIsPlaying = ref(false);

vi.mock('../useTTS', () => ({
  useTTS: () => ({
    enqueue: mockTTSEnqueue,
    stop: mockTTSStop,
    isPlaying: readonly(mockTTSIsPlaying),
    queue: readonly(ref([])),
    onQueueDrained: vi.fn(),
  }),
  unlockAudio: vi.fn(),
}));

// ── Mock useVoice ───────────────────────────────────────────────────

vi.mock('../useVoice', () => ({
  useVoice: () => ({
    startRecording: vi.fn().mockResolvedValue(undefined),
    stopRecording: vi.fn().mockResolvedValue('test transcript'),
    isRecording: readonly(ref(false)),
    isMonitoring: readonly(ref(false)),
    lastError: ref(null),
  }),
}));

// ── Mock useTTSChunker, useTTSCondenser, useTTSToolBatcher ─────────

vi.mock('../useTTSChunker', () => ({
  useTTSChunker: () => ({
    feed: vi.fn(),
    flush: vi.fn(),
    reset: vi.fn(),
  }),
}));

vi.mock('../useTTSCondenser', () => ({
  useTTSCondenser: () => ({
    feed: vi.fn(),
    flush: vi.fn(),
    reset: vi.fn(),
  }),
}));

vi.mock('../useTTSToolBatcher', () => ({
  useTTSToolBatcher: () => ({
    feed: vi.fn(),
    flush: vi.fn(),
    reset: vi.fn(),
  }),
}));

// ── Import state machine (real) and orchestrator (under test) ───────

import {
  dispatch,
  abort,
  _forceStateForTesting,
} from '../useStateMachine';

import { useAudioOrchestrator } from '../useAudioOrchestrator';

// ── Helpers ─────────────────────────────────────────────────────────

function resetAllMocks() {
  mockPlayHandoff.mockClear();
  mockStartWorkingHum.mockClear();
  mockStopWorkingHum.mockClear();
  mockPlaySuccessChime.mockClear();
  mockPlayQuestionChime.mockClear();
  mockPlayPermissionChime.mockClear();
  mockPlayCancelTone.mockClear();
  mockPlayLoopListeningTick.mockClear();
  mockStartWatchdog.mockClear();
  mockCancelWatchdog.mockClear();
  mockTTSEnqueue.mockClear();
  mockTTSStop.mockClear();
  mockStopAll.mockClear();
  mockIsHumActive.mockReturnValue(false);
}

/**
 * Creates an orchestrator instance with default options.
 * Must be called AFTER _forceStateForTesting (which clears listeners).
 */
function createOrchestrator() {
  return useAudioOrchestrator({
    sessionId: 'test-session',
    sendMessage: vi.fn(),
    hasPendingQuestion: computed(() => false),
    hasPendingPermission: computed(() => false),
    connectionState: ref('connected'),
    voiceEnabled: ref(true),
  });
}

// ── Tests ───────────────────────────────────────────────────────────

beforeEach(() => {
  _useStateCache.clear();
  _forceStateForTesting('idle');
  resetAllMocks();
});

describe('transition → audio: entering agent_turn', () => {
  it('send_message → handoff + hum + watchdog', () => {
    const orch = createOrchestrator();

    // idle → user_turn → agent_turn (mimics send flow)
    dispatch('start_user_turn', 'voice_button');
    dispatch('start_agent_turn', 'send_message');

    expect(mockPlayHandoff).toHaveBeenCalledTimes(1);
    expect(mockStartWorkingHum).toHaveBeenCalledTimes(1);
    expect(mockStartWatchdog).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });

  it('session_busy_on_load → hum only (no handoff, no watchdog)', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'session_busy_on_load');

    expect(mockStartWorkingHum).toHaveBeenCalledTimes(1);
    expect(mockPlayHandoff).not.toHaveBeenCalled();
    // Watchdog is not started for session_busy_on_load since we
    // don't know when the turn started
    expect(mockStartWatchdog).not.toHaveBeenCalled();

    orch.cleanup();
  });

  it('auto_voice_initial → hum only (no handoff, no watchdog)', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'auto_voice_initial');

    expect(mockStartWorkingHum).toHaveBeenCalledTimes(1);
    expect(mockPlayHandoff).not.toHaveBeenCalled();
    expect(mockStartWatchdog).not.toHaveBeenCalled();

    orch.cleanup();
  });

  it('permission_response → no audio (returning from input)', () => {
    const orch = createOrchestrator();

    // Set up: agent_turn → awaiting_input → agent_turn
    dispatch('start_agent_turn', 'send_message');
    resetAllMocks(); // clear initial entry audio

    dispatch('await_input', 'permission_request');
    resetAllMocks(); // clear await_input audio

    dispatch('answer_input', 'permission_response');

    expect(mockPlayHandoff).not.toHaveBeenCalled();
    expect(mockStartWorkingHum).not.toHaveBeenCalled();
    expect(mockStartWatchdog).not.toHaveBeenCalled();

    orch.cleanup();
  });

  it('question_answered → no audio (returning from input)', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();

    dispatch('await_input', 'question_request');
    resetAllMocks();

    dispatch('answer_input', 'question_answered');

    expect(mockPlayHandoff).not.toHaveBeenCalled();
    expect(mockStartWorkingHum).not.toHaveBeenCalled();
    expect(mockStartWatchdog).not.toHaveBeenCalled();

    orch.cleanup();
  });

  it('autoVoiceArmed is consumed on → agent_turn, sets voiceInitiatedTurn', () => {
    const orch = createOrchestrator();

    // Arm auto-voice before transition
    orch._setAutoVoiceArmedForTesting(true);

    dispatch('start_agent_turn', 'session_busy_on_load');

    expect(orch.voiceInitiatedTurn.value).toBe(true);
    // autoVoiceArmed should be consumed (false)
    expect(orch._getAutoVoiceArmedForTesting()).toBe(false);

    orch.cleanup();
  });
});

describe('transition → audio: leaving agent_turn', () => {
  it('agent_turn → idle (normal) → success chime', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();

    dispatch('complete_turn', 'turn_complete');

    expect(mockPlaySuccessChime).toHaveBeenCalledTimes(1);
    expect(mockCancelWatchdog).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });

  it('agent_turn → idle via abort → no success chime (cancel tone already played)', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();

    abort('user_abort');

    // Abort handler plays cancel tone + stops hum, NOT success chime
    expect(mockPlaySuccessChime).not.toHaveBeenCalled();

    orch.cleanup();
  });

  it('agent_turn → awaiting_input via permission_request → stop hum + permission chime', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();

    dispatch('await_input', 'permission_request');

    expect(mockStopWorkingHum).toHaveBeenCalledTimes(1);
    expect(mockPlayPermissionChime).toHaveBeenCalledTimes(1);
    expect(mockCancelWatchdog).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });

  it('agent_turn → awaiting_input via question_request → stop hum + question chime', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();

    dispatch('await_input', 'question_request');

    expect(mockStopWorkingHum).toHaveBeenCalledTimes(1);
    expect(mockPlayQuestionChime).toHaveBeenCalledTimes(1);
    expect(mockCancelWatchdog).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });
});

describe('transition → audio: user_turn', () => {
  it('idle → user_turn via loop_recording_start → loop listening tick', () => {
    const orch = createOrchestrator();

    dispatch('start_user_turn', 'loop_recording_start');

    expect(mockPlayLoopListeningTick).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });

  it('idle → user_turn via voice_button → no loop tick (manual start)', () => {
    const orch = createOrchestrator();

    dispatch('start_user_turn', 'voice_button');

    expect(mockPlayLoopListeningTick).not.toHaveBeenCalled();

    orch.cleanup();
  });
});

describe('transition → audio: abort', () => {
  it('abort from agent_turn → stop hum + cancel tone + TTS "Stopped."', () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();
    mockIsHumActive.mockReturnValue(true);

    abort('abort_session');

    expect(mockStopWorkingHum).toHaveBeenCalledTimes(1);
    expect(mockPlayCancelTone).toHaveBeenCalledTimes(1);
    expect(mockCancelWatchdog).toHaveBeenCalledTimes(1);
    expect(mockTTSStop).toHaveBeenCalledTimes(1);
    expect(mockTTSEnqueue).toHaveBeenCalledWith('Stopped.');

    orch.cleanup();
  });

  it('abort from user_turn → cancel tone + TTS "Stopped." (no hum to stop)', () => {
    const orch = createOrchestrator();

    dispatch('start_user_turn', 'voice_button');
    resetAllMocks();

    abort('abort_session');

    expect(mockPlayCancelTone).toHaveBeenCalledTimes(1);
    expect(mockTTSEnqueue).toHaveBeenCalledWith('Stopped.');

    orch.cleanup();
  });

  it('abort from idle → minimal cleanup only', () => {
    const orch = createOrchestrator();

    abort('abort_session');

    // Already idle, so cancel tone still fires (user requested abort)
    // but no hum to stop
    expect(mockStopWorkingHum).not.toHaveBeenCalled();
    expect(mockPlayCancelTone).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });
});
