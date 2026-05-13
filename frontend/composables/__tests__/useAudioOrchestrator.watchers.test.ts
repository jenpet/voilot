/**
 * Reactive watcher tests for useAudioOrchestrator.
 *
 * Tests the reactive rules that respond to ref changes (not SM transitions):
 * - TTS playing → stops hum
 * - TTS done + agent done → tryCompleteTurn
 * - Voice toggled off → stops monitoring + TTS
 * - Connection state changes → warning/reconnect tones + announcements
 *
 * These tests mutate refs and flush Vue's nextTick to trigger watchers.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { ref, computed, readonly, nextTick } from 'vue';

// ── Nuxt auto-import stubs ──────────────────────────────────────────

vi.stubGlobal('ref', ref);
vi.stubGlobal('computed', computed);
vi.stubGlobal('readonly', readonly);

// We need real `watch` for watcher tests — do NOT stub it
const { watch: vueWatch } = await import('vue');
vi.stubGlobal('watch', vueWatch);
vi.stubGlobal('useState', (key: string, init?: () => any) => ref(init ? init() : undefined));
vi.stubGlobal('useSuppressInitialTools', () => ({
  suppressInitialTools: readonly(ref(true)),
  _setSuppressInitialTools: vi.fn(),
}));

// ── Mock useDebugLog ────────────────────────────────────────────────

vi.mock('../useDebugLog', () => ({
  useDebugLog: () => ({ log: vi.fn() }),
  setDebugLogStateAccessor: vi.fn(),
}));

// ── Mock useAudioFeedback ───────────────────────────────────────────

const mockStopWorkingHum = vi.fn().mockResolvedValue(undefined);
const mockPlayWarningTone = vi.fn();
const mockPlayReconnectChime = vi.fn();
const mockIsHumActive = vi.fn().mockReturnValue(false);
const mockCancelWatchdog = vi.fn();

vi.mock('../useAudioFeedback', () => ({
  playHandoff: vi.fn(),
  startWorkingHum: vi.fn(),
  stopWorkingHum: (...args: any[]) => mockStopWorkingHum(...args),
  playSuccessChime: vi.fn(),
  playQuestionChime: vi.fn(),
  playPermissionChime: vi.fn(),
  playCancelTone: vi.fn(),
  playLoopListeningTick: vi.fn(),
  startWatchdog: vi.fn(),
  cancelWatchdog: (...args: any[]) => mockCancelWatchdog(...args),
  setTTSEnqueue: vi.fn(),
  notifyToolActivity: vi.fn(),
  initAudioFeedback: vi.fn(),
  stopAll: vi.fn(),
  isHumActive: (...args: any[]) => mockIsHumActive(...args),
  playWarningTone: (...args: any[]) => mockPlayWarningTone(...args),
  playReconnectChime: (...args: any[]) => mockPlayReconnectChime(...args),
}));

// ── Mock useTTS with controllable refs ──────────────────────────────

const mockTTSIsPlaying = ref(false);
const mockTTSEnqueue = vi.fn();
const mockTTSStop = vi.fn();

vi.mock('../useTTS', () => ({
  useTTS: () => ({
    enqueue: (...args: any[]) => mockTTSEnqueue(...args),
    stop: (...args: any[]) => mockTTSStop(...args),
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

// ── Mock TTS pipeline composables ───────────────────────────────────

vi.mock('../useTTSChunker', () => ({
  useTTSChunker: () => ({ feed: vi.fn(), flush: vi.fn(), reset: vi.fn() }),
}));

vi.mock('../useTTSCondenser', () => ({
  useTTSCondenser: () => ({ feed: vi.fn(), flush: vi.fn(), reset: vi.fn() }),
}));

vi.mock('../useTTSToolBatcher', () => ({
  useTTSToolBatcher: () => ({ feed: vi.fn(), flush: vi.fn(), reset: vi.fn() }),
}));

// ── Import SM ───────────────────────────────────────────────────────

import {
  dispatch,
  _forceStateForTesting,
  getState,
} from '../useStateMachine';

import { useAudioOrchestrator } from '../useAudioOrchestrator';

// ── Helpers ─────────────────────────────────────────────────────────

function resetAllMocks() {
  mockStopWorkingHum.mockClear();
  mockPlayWarningTone.mockClear();
  mockPlayReconnectChime.mockClear();
  mockTTSEnqueue.mockClear();
  mockTTSStop.mockClear();
  mockIsHumActive.mockReturnValue(false);
  mockCancelWatchdog.mockClear();
}

function createOrchestrator(overrides: Record<string, any> = {}) {
  return useAudioOrchestrator({
    sessionId: 'test-session',
    sendMessage: vi.fn(),
    hasPendingQuestion: computed(() => false),
    hasPendingPermission: computed(() => false),
    connectionState: overrides.connectionState ?? ref('connected'),
    voiceEnabled: overrides.voiceEnabled ?? ref(true),
    ...overrides,
  });
}

// ── Tests ───────────────────────────────────────────────────────────

beforeEach(() => {
  _forceStateForTesting('idle');
  resetAllMocks();
  mockTTSIsPlaying.value = false;
});

describe('watcher: TTS playing → hum control', () => {
  it('TTS starts playing → stops working hum', async () => {
    const orch = createOrchestrator();

    // Put SM in agent_turn with hum active
    dispatch('start_agent_turn', 'send_message');
    resetAllMocks();
    mockIsHumActive.mockReturnValue(true);

    // TTS starts playing
    mockTTSIsPlaying.value = true;
    await nextTick();

    expect(mockStopWorkingHum).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });

  it('TTS stops + agent done → triggers tryCompleteTurn (agent_turn → idle)', async () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    // Simulate agent streaming done
    orch.notifyStreamingDone({ abortedTurn: false });

    // TTS was playing, now stops
    mockTTSIsPlaying.value = true;
    await nextTick();

    mockTTSIsPlaying.value = false;
    await nextTick();

    // Should have completed the turn
    expect(getState()).toBe('idle');

    orch.cleanup();
  });

  it('TTS stops but agent NOT done → stays in agent_turn', async () => {
    const orch = createOrchestrator();

    dispatch('start_agent_turn', 'send_message');
    // Do NOT call notifyStreamingDone — agent is still streaming

    mockTTSIsPlaying.value = true;
    await nextTick();

    mockTTSIsPlaying.value = false;
    await nextTick();

    // Should still be in agent_turn
    expect(getState()).toBe('agent_turn');

    orch.cleanup();
  });
});

describe('watcher: connection state', () => {
  it('connection lost → warning tone + "Connection lost." announcement', async () => {
    const connectionState = ref('connected');
    const orch = createOrchestrator({ connectionState });
    resetAllMocks();

    connectionState.value = 'disconnected';
    await nextTick();

    expect(mockPlayWarningTone).toHaveBeenCalledTimes(1);
    expect(mockTTSEnqueue).toHaveBeenCalledWith('Connection lost.');

    orch.cleanup();
  });

  it('initial connection → no reconnect chime (silent)', async () => {
    const connectionState = ref('disconnected');
    const orch = createOrchestrator({ connectionState });
    resetAllMocks();

    connectionState.value = 'connected';
    await nextTick();

    expect(mockPlayReconnectChime).not.toHaveBeenCalled();
    expect(mockTTSEnqueue).not.toHaveBeenCalled();

    orch.cleanup();
  });

  it('connection restored after disconnect → reconnect chime + "Reconnected."', async () => {
    const connectionState = ref('disconnected');
    const orch = createOrchestrator({ connectionState });

    // Initial connection (silent)
    connectionState.value = 'connected';
    await nextTick();
    resetAllMocks();

    // Disconnect
    connectionState.value = 'disconnected';
    await nextTick();
    resetAllMocks();

    // Reconnect
    connectionState.value = 'connected';
    await nextTick();

    expect(mockPlayReconnectChime).toHaveBeenCalledTimes(1);
    expect(mockTTSEnqueue).toHaveBeenCalledWith('Reconnected.');

    orch.cleanup();
  });
});

describe('watcher: voice toggled off', () => {
  it('voiceEnabled off → stops TTS playback', async () => {
    const orch = createOrchestrator();

    // Enable voice first
    orch._setVoiceEnabledForTesting(true);
    await nextTick();
    resetAllMocks();

    // Disable voice
    orch._setVoiceEnabledForTesting(false);
    await nextTick();

    expect(mockTTSStop).toHaveBeenCalledTimes(1);

    orch.cleanup();
  });
});
