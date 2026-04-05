import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  playHandoff,
  startWorkingHum,
  stopWorkingHum,
  isHumActive,
  playQuestionChime,
  playSuccessChime,
  playErrorTone,
  playCancelTone,
  playLoopListeningTick,
  playSTTFailureTone,
  playReconnectChime,
  playWarningTone,
  playPermissionChime,
  playModeSignature,
  startWatchdog,
  cancelWatchdog,
  isWatchdogActive,
  setTTSEnqueue,
  notifyToolActivity,
  initAudioFeedback,
  _resetForTesting,
  DEBOUNCE_MS,
  WATCHDOG_TIMEOUT_MS,
} from '../useAudioFeedback';

// ── Mock HTMLAudioElement ───────────────────────────────────────────
// happy-dom provides Audio but play() doesn't really work.
// We spy on the prototype to verify calls.

let playSpy: ReturnType<typeof vi.spyOn>;
let pauseSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  _resetForTesting();
  vi.useFakeTimers();

  playSpy = vi.spyOn(HTMLAudioElement.prototype, 'play')
    .mockResolvedValue(undefined);
  pauseSpy = vi.spyOn(HTMLAudioElement.prototype, 'pause')
    .mockImplementation(() => {});
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  _resetForTesting();
});

describe('useAudioFeedback', () => {
  describe('initAudioFeedback', () => {
    it('creates audio elements without throwing', () => {
      expect(() => initAudioFeedback()).not.toThrow();
    });
  });

  describe('one-shot sounds', () => {
    it('playHandoff triggers audio playback', () => {
      playHandoff();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playQuestionChime triggers audio playback', () => {
      playQuestionChime();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playSuccessChime triggers audio playback', () => {
      playSuccessChime();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playErrorTone triggers audio playback', () => {
      playErrorTone();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playCancelTone triggers audio playback', () => {
      playCancelTone();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playLoopListeningTick triggers audio playback', () => {
      playLoopListeningTick();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playSTTFailureTone triggers audio playback', () => {
      playSTTFailureTone();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playReconnectChime triggers audio playback', () => {
      playReconnectChime();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playWarningTone triggers audio playback', () => {
      playWarningTone();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playPermissionChime triggers audio playback', () => {
      playPermissionChime();
      expect(playSpy).toHaveBeenCalled();
    });

    it('playModeSignature triggers audio playback', () => {
      playModeSignature();
      expect(playSpy).toHaveBeenCalled();
    });
  });

  describe('debounce', () => {
    it('suppresses identical sound within DEBOUNCE_MS', () => {
      playHandoff();
      const callCount = playSpy.mock.calls.length;

      // Immediately play again — should be debounced
      playHandoff();
      expect(playSpy.mock.calls.length).toBe(callCount);
    });

    it('allows same sound after DEBOUNCE_MS', () => {
      playHandoff();
      const callCount = playSpy.mock.calls.length;

      vi.advanceTimersByTime(DEBOUNCE_MS + 1);
      playHandoff();
      expect(playSpy.mock.calls.length).toBe(callCount + 1);
    });

    it('allows different sounds within DEBOUNCE_MS', () => {
      playHandoff();
      const callCount = playSpy.mock.calls.length;

      playSuccessChime();
      expect(playSpy.mock.calls.length).toBe(callCount + 1);
    });
  });

  describe('working hum lifecycle', () => {
    it('startWorkingHum sets hum active', () => {
      expect(isHumActive()).toBe(false);
      startWorkingHum();
      expect(isHumActive()).toBe(true);
    });

    it('startWorkingHum is idempotent', () => {
      startWorkingHum();
      const callCount = playSpy.mock.calls.length;

      startWorkingHum(); // second call — should be no-op
      expect(playSpy.mock.calls.length).toBe(callCount);
    });

    it('stopWorkingHum resolves and clears hum state', async () => {
      startWorkingHum();
      expect(isHumActive()).toBe(true);

      await stopWorkingHum();
      expect(isHumActive()).toBe(false);
    });

    it('stopWorkingHum resolves immediately when hum is not active', async () => {
      expect(isHumActive()).toBe(false);
      await stopWorkingHum(); // should not throw or hang
      expect(isHumActive()).toBe(false);
    });

    it('stopWorkingHum pauses audio', async () => {
      startWorkingHum();
      await stopWorkingHum();
      expect(pauseSpy).toHaveBeenCalled();
    });

    it('stopWorkingHum can be called multiple times safely', async () => {
      startWorkingHum();
      await stopWorkingHum();
      await stopWorkingHum();
      expect(isHumActive()).toBe(false);
    });
  });

  describe('spoken check-ins', () => {
    it('calls TTS with generic message after fade + interval', () => {
      const ttsSpy = vi.fn();
      setTTSEnqueue(ttsSpy);
      startWorkingHum();

      // Advance past fade start (17s) + fade duration (3s) + check-in interval (30s)
      vi.advanceTimersByTime(17_000 + 3_000 + 30_000);
      expect(ttsSpy).toHaveBeenCalledWith('Still working.');
    });

    it('includes tool context when tools are active', () => {
      const ttsSpy = vi.fn();
      setTTSEnqueue(ttsSpy);
      startWorkingHum();
      notifyToolActivity();

      vi.advanceTimersByTime(17_000 + 3_000 + 30_000);
      expect(ttsSpy).toHaveBeenCalledWith('Still working, running tools.');
    });

    it('stops check-ins when hum is stopped', async () => {
      const ttsSpy = vi.fn();
      setTTSEnqueue(ttsSpy);
      startWorkingHum();

      // Stop before any check-in fires
      await stopWorkingHum();
      vi.advanceTimersByTime(100_000);
      expect(ttsSpy).not.toHaveBeenCalled();
    });
  });

  describe('hard cap', () => {
    it('stops hum and speaks at 90s', () => {
      const ttsSpy = vi.fn();
      setTTSEnqueue(ttsSpy);
      startWorkingHum();

      vi.advanceTimersByTime(90_000);
      expect(isHumActive()).toBe(false);
      expect(ttsSpy).toHaveBeenCalledWith('Agent is still working, please wait.');
    });
  });

  describe('watchdog', () => {
    it('starts and reports active', () => {
      startWatchdog();
      expect(isWatchdogActive()).toBe(true);
    });

    it('cancelWatchdog clears the timer', () => {
      startWatchdog();
      cancelWatchdog();
      expect(isWatchdogActive()).toBe(false);
    });

    it('fires error tone and TTS after timeout', async () => {
      const ttsSpy = vi.fn();
      setTTSEnqueue(ttsSpy);
      startWorkingHum();
      startWatchdog();

      vi.advanceTimersByTime(WATCHDOG_TIMEOUT_MS);
      // The watchdog callback is async: it awaits stopWorkingHum() which
      // uses queueMicrotask(resolve). Flush the microtask queue so the
      // await completes and the rest of the callback (playErrorTone, TTS) runs.
      await vi.advanceTimersByTimeAsync(0);

      expect(isHumActive()).toBe(false);
      expect(isWatchdogActive()).toBe(false);
      expect(ttsSpy).toHaveBeenCalledWith("I didn't catch that. Please try again.");
    });

    it('does not fire if cancelled in time', () => {
      const ttsSpy = vi.fn();
      setTTSEnqueue(ttsSpy);
      startWatchdog();

      vi.advanceTimersByTime(WATCHDOG_TIMEOUT_MS - 100);
      cancelWatchdog();
      vi.advanceTimersByTime(200);

      expect(ttsSpy).not.toHaveBeenCalled();
    });
  });
});
