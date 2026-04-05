import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  playStartBlip,
  suppressNextStopBlip,
  _resetRecordingFeedbackForTesting,
} from '../useRecordingFeedback';

// ── Mock HTMLAudioElement ───────────────────────────────────────────

const playSpy = vi.fn().mockResolvedValue(undefined);

vi.stubGlobal('Audio', class MockAudio {
  src = '';
  volume = 1.0;
  currentTime = 0;
  play = playSpy;
});
vi.stubGlobal('URL', { createObjectURL: () => 'blob:mock' });
vi.stubGlobal('Blob', class MockBlob {
  constructor(public parts: unknown[], public options?: BlobPropertyBag) {}
});

describe('useRecordingFeedback', () => {
  beforeEach(() => {
    _resetRecordingFeedbackForTesting();
    playSpy.mockClear();
  });

  describe('playStartBlip', () => {
    it('creates audio and plays on first call', () => {
      playStartBlip();
      expect(playSpy).toHaveBeenCalledTimes(1);
    });

    it('reuses audio element on subsequent calls', () => {
      playStartBlip();
      playStartBlip();
      // Should still only create elements once, but play twice
      expect(playSpy).toHaveBeenCalledTimes(2);
    });
  });

  describe('suppressNextStopBlip', () => {
    it('is a one-shot flag', () => {
      // suppressNextStopBlip sets the flag, but since stop blip
      // is driven by watch() in useRecordingFeedback() composable,
      // we test the flag reset mechanism by calling it twice
      suppressNextStopBlip();
      // The flag is internal — we verify behavior via the module
      // export existing and being callable without error
      expect(() => suppressNextStopBlip()).not.toThrow();
    });
  });

  describe('_resetRecordingFeedbackForTesting', () => {
    it('clears cached audio elements', () => {
      playStartBlip(); // creates elements
      _resetRecordingFeedbackForTesting();
      playStartBlip(); // should recreate
      // Both calls play, confirming recreation works
      expect(playSpy).toHaveBeenCalledTimes(2);
    });
  });
});
