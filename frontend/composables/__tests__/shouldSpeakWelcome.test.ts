import { describe, it, expect } from 'vitest';
import { shouldSpeakWelcome } from '../useAgent';

describe('shouldSpeakWelcome', () => {
  it('returns true for voice-enabled fresh session with only welcome message', () => {
    const messages = [{ id: 'welcome-ses_123', role: 'assistant' }];
    expect(shouldSpeakWelcome(true, messages)).toBe(true);
  });

  it('returns false when user has already sent a message (reload case)', () => {
    const messages = [
      { id: 'welcome-ses_123', role: 'assistant' },
      { id: 'msg-1', role: 'user' },
      { id: 'msg-2', role: 'assistant' },
    ];
    expect(shouldSpeakWelcome(true, messages)).toBe(false);
  });

  it('returns false when voice is disabled', () => {
    const messages = [{ id: 'welcome-ses_123', role: 'assistant' }];
    expect(shouldSpeakWelcome(false, messages)).toBe(false);
  });

  it('returns false when messages array is empty', () => {
    expect(shouldSpeakWelcome(true, [])).toBe(false);
  });

  it('returns false when first message is not a welcome message (old session)', () => {
    const messages = [{ id: 'msg-1', role: 'assistant' }];
    expect(shouldSpeakWelcome(true, messages)).toBe(false);
  });
});
