/**
 * Acoustic feedback for recording state changes.
 *
 * Plays blip sounds via an HTML Audio element with programmatically
 * generated WAV data URIs.  This avoids the Web Audio API oscillator
 * path which browsers' echo cancellation (AEC) actively suppresses
 * while the microphone is open.
 *
 * - Recording starts: single blip (660 Hz) — manual tap only
 * - Recording stops:  single lower blip (440 Hz) — manual stop only
 *
 * Blips are suppressed during the automatic conversational voice loop
 * (loopRecordingActive === true) so the user only hears a blip on
 * initial mic activation and final deactivation.
 *
 * Stop blip is also suppressed when `suppressStopBlip` is true — this
 * prevents the "double beep" when a voice-originated message triggers
 * both a stop blip and a handoff tone in quick succession.
 *
 * Usage: call `useRecordingFeedback()` once in a component or plugin.
 * It watches the shared `isRecording` state from useVoice and plays
 * the appropriate sound on every transition.
 */

import {
  generateSineBlip,
  createAudioFromSamples,
  SAMPLE_RATE,
} from './audioSynth';
import { useDebugLog } from './useDebugLog';
import type { DebugLogLevel } from './useDebugLog';

function _log(level: DebugLogLevel, event: string, data?: Record<string, unknown>) {
  try {
    const { log } = useDebugLog();
    log(level, 'audio-feedback', event, data);
  } catch {
    // Composable not available outside setup — ignore
  }
}

const START_FREQ = 660;   // Original pitch for start
const STOP_FREQ = 440;    // Lower pitch for stop (A4)
const BLIP_DURATION_MS = 80;
const BLIP_VOLUME = 0.50;

// Pre-generated audio elements (created once, reused)
let _startAudio: HTMLAudioElement | null = null;
let _stopAudio: HTMLAudioElement | null = null;

/**
 * When true, the next stop-blip is suppressed. Automatically resets
 * to false after the blip is skipped (one-shot flag).
 *
 * Set this before sending a voice-originated message so the stop blip
 * doesn't overlap with the handoff tone from useAudioFeedback.
 */
let _suppressStopBlip = false;

/**
 * Suppress the next stop blip (one-shot).
 * Called by useAgent before sending a voice-originated message.
 */
export function suppressNextStopBlip(): void {
  _suppressStopBlip = true;
}

/**
 * Build the blip Audio elements.
 */
function ensureAudioElements() {
  if (_startAudio && _stopAudio) return;

  // Single high blip for start
  const startBlip = generateSineBlip(START_FREQ, BLIP_DURATION_MS, BLIP_VOLUME, SAMPLE_RATE);
  _startAudio = createAudioFromSamples(startBlip);

  // Single low blip for stop
  const stopBlip = generateSineBlip(STOP_FREQ, BLIP_DURATION_MS, BLIP_VOLUME, SAMPLE_RATE);
  _stopAudio = createAudioFromSamples(stopBlip);
}

function playStopSound() {
  ensureAudioElements();
  if (_stopAudio) {
    _stopAudio.currentTime = 0;
    _stopAudio.play().catch(() => {});
  }
}

/**
 * Pre-warm the blip HTMLAudioElement instances so the browser (and
 * Bluetooth codec) treats them as user-gesture-activated.
 *
 * Must be called synchronously inside a user-gesture handler —
 * typically from `unlockAudio()`.  Each element is played at zero
 * volume then immediately paused, which:
 *   1. Satisfies autoplay-policy requirements for future plays.
 *   2. Spins up the Bluetooth A2DP/AAC codec so the next real play()
 *      doesn't suffer a ~600 ms cold-start delay that swallows the
 *      80 ms blip entirely.
 */
/**
 * Pre-warm the browser / Bluetooth audio path so the first real blip
 * plays without codec cold-start delay.
 *
 * Instead of touching the actual blip elements (which causes volume
 * race conditions with playStartBlip), we play a separate short
 * silent WAV through a throwaway HTMLAudioElement. This is enough to:
 *   1. Activate the browser's audio session from a user gesture.
 *   2. Spin up the Bluetooth A2DP/AAC codec (~600ms negotiation).
 *
 * Must be called synchronously inside a user-gesture handler —
 * typically from `unlockAudio()`.
 */
export function warmUpBlips(): void {
  // Ensure the real blip elements exist (lazy init).
  ensureAudioElements();

  // Play a tiny near-silent WAV through a throwaway element.
  // Duration matches the blip length so the codec is fully active
  // by the time playStartBlip() fires on the next gesture.
  try {
    const silence = generateSineBlip(440, BLIP_DURATION_MS, 0.01, SAMPLE_RATE);
    const el = createAudioFromSamples(silence);
    el.play().catch(() => {});
  } catch {
    // best effort
  }
}

/**
 * Play the start-recording blip.
 *
 * Must be called directly from a user-gesture handler (click/tap)
 * so the browser allows playback even on the very first interaction.
 * This is exported and called from VoiceButton's toggle() function.
 */
export function playStartBlip(): void {
  ensureAudioElements();
  _log('debug', 'play_start_blip');
  if (_startAudio) {
    _startAudio.currentTime = 0;
    _startAudio.play().catch(() => {});
  }
}

export function useRecordingFeedback() {
  const isRecording = useState<boolean>('voice-recording');
  const loopRecordingActive = useState<boolean>('voice-loop-active', () => false);

  // Pre-build the audio elements so first playback is instant
  if (import.meta.client) {
    ensureAudioElements();
  }

  // Stop blip is driven by state — works reliably because by the time
  // recording stops, the Audio elements have already been activated
  // by the user gesture that started recording.
  // Suppressed during loop recordings to avoid constant beeping.
  // Also suppressed when suppressStopBlip flag is set (voice-originated
  // messages use handoff tone instead, preventing double-beep).
  watch(isRecording, (recording, wasRecording) => {
    // Only play on real transitions (not initial undefined->false)
    if (wasRecording === undefined || wasRecording === null) return;
    if (recording === wasRecording) return;

    if (!recording && !loopRecordingActive.value) {
      if (_suppressStopBlip) {
        _suppressStopBlip = false;
        _log('debug', 'stop_blip_suppressed');
        return;
      }
      _log('debug', 'play_stop_blip');
      playStopSound();
    }
    // Start blip is NOT played here — it's called directly from
    // the tap handler via playStartBlip() to guarantee it works
    // on the very first user interaction.
  });
}

/**
 * Reset internal state for testing. Not for production use.
 */
export function _resetRecordingFeedbackForTesting(): void {
  _startAudio = null;
  _stopAudio = null;
  _suppressStopBlip = false;
}
