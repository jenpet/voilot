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
 * - Thinking jingle:  ascending two-tone chime when a message is sent
 *
 * Blips are suppressed during the automatic conversational voice loop
 * (loopRecordingActive === true) so the user only hears a blip on
 * initial mic activation and final deactivation.
 *
 * Usage: call `useRecordingFeedback()` once in a component or plugin.
 * It watches the shared `isRecording` state from useVoice and plays
 * the appropriate sound on every transition.
 */

const SAMPLE_RATE = 24000;
const START_FREQ = 660;   // Original pitch for start
const STOP_FREQ = 440;    // Lower pitch for stop (A4)
const BLIP_DURATION_MS = 80;
const BLIP_VOLUME = 0.35;

// Thinking jingle: ascending two-tone chime
const THINK_FREQ_1 = 523;  // C5
const THINK_FREQ_2 = 659;  // E5
const THINK_TONE_MS = 70;
const THINK_GAP_MS = 30;
const THINK_VOLUME = 0.25;

// Pre-generated audio elements (created once, reused)
let _startAudio: HTMLAudioElement | null = null;
let _stopAudio: HTMLAudioElement | null = null;
let _thinkAudio: HTMLAudioElement | null = null;

/**
 * Generate a PCM sine-wave blip as a Float32Array.
 */
function generateSineBlip(
  freq: number,
  durationMs: number,
  volume: number,
  sampleRate: number,
): Float32Array {
  const numSamples = Math.floor((durationMs / 1000) * sampleRate);
  const samples = new Float32Array(numSamples);
  const fadeLen = Math.floor(sampleRate * 0.008); // 8ms fade

  for (let i = 0; i < numSamples; i++) {
    let amp = volume;
    // Fade in
    if (i < fadeLen) amp *= i / fadeLen;
    // Fade out
    if (i > numSamples - fadeLen) amp *= (numSamples - i) / fadeLen;

    samples[i] = amp * Math.sin(2 * Math.PI * freq * i / sampleRate);
  }
  return samples;
}

/**
 * Encode Float32 PCM samples into a WAV file as an ArrayBuffer.
 */
function encodeWav(samples: Float32Array, sampleRate: number): ArrayBuffer {
  const numSamples = samples.length;
  const bytesPerSample = 2; // 16-bit
  const dataSize = numSamples * bytesPerSample;
  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);

  // RIFF header
  writeString(view, 0, 'RIFF');
  view.setUint32(4, 36 + dataSize, true);
  writeString(view, 8, 'WAVE');

  // fmt chunk
  writeString(view, 12, 'fmt ');
  view.setUint32(16, 16, true);         // chunk size
  view.setUint16(20, 1, true);          // PCM format
  view.setUint16(22, 1, true);          // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * bytesPerSample, true); // byte rate
  view.setUint16(32, bytesPerSample, true);              // block align
  view.setUint16(34, 16, true);         // bits per sample

  // data chunk
  writeString(view, 36, 'data');
  view.setUint32(40, dataSize, true);

  // PCM samples (16-bit signed)
  for (let i = 0; i < numSamples; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(44 + i * 2, s * 0x7FFF, true);
  }

  return buffer;
}

function writeString(view: DataView, offset: number, str: string) {
  for (let i = 0; i < str.length; i++) {
    view.setUint8(offset + i, str.charCodeAt(i));
  }
}

/**
 * Create an Audio element from PCM samples.
 */
function createAudioFromSamples(samples: Float32Array): HTMLAudioElement {
  const wavBuffer = encodeWav(samples, SAMPLE_RATE);
  const blob = new Blob([wavBuffer], { type: 'audio/wav' });
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.volume = 1.0;
  return audio;
}

/**
 * Build the blip and jingle Audio elements.
 */
function ensureAudioElements() {
  if (_startAudio && _stopAudio && _thinkAudio) return;

  // Single high blip for start
  const startBlip = generateSineBlip(START_FREQ, BLIP_DURATION_MS, BLIP_VOLUME, SAMPLE_RATE);
  _startAudio = createAudioFromSamples(startBlip);

  // Single low blip for stop
  const stopBlip = generateSineBlip(STOP_FREQ, BLIP_DURATION_MS, BLIP_VOLUME, SAMPLE_RATE);
  _stopAudio = createAudioFromSamples(stopBlip);

  // Thinking jingle: two ascending tones with a short gap of silence
  const tone1 = generateSineBlip(THINK_FREQ_1, THINK_TONE_MS, THINK_VOLUME, SAMPLE_RATE);
  const gapSamples = Math.floor((THINK_GAP_MS / 1000) * SAMPLE_RATE);
  const gap = new Float32Array(gapSamples); // silence
  const tone2 = generateSineBlip(THINK_FREQ_2, THINK_TONE_MS, THINK_VOLUME, SAMPLE_RATE);
  const jingle = new Float32Array(tone1.length + gap.length + tone2.length);
  jingle.set(tone1, 0);
  jingle.set(gap, tone1.length);
  jingle.set(tone2, tone1.length + gap.length);
  _thinkAudio = createAudioFromSamples(jingle);
}

function playStopSound() {
  ensureAudioElements();
  if (_stopAudio) {
    _stopAudio.currentTime = 0;
    _stopAudio.play().catch(() => {});
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
  if (_startAudio) {
    _startAudio.currentTime = 0;
    _startAudio.play().catch(() => {});
  }
}

/**
 * Play the "thinking" jingle — a subtle ascending two-tone chime
 * indicating a message has been submitted and the agent is working.
 */
export function playThinkingJingle(): void {
  ensureAudioElements();
  if (_thinkAudio) {
    _thinkAudio.currentTime = 0;
    _thinkAudio.play().catch(() => {});
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
  watch(isRecording, (recording, wasRecording) => {
    // Only play on real transitions (not initial undefined->false)
    if (wasRecording === undefined || wasRecording === null) return;
    if (recording === wasRecording) return;

    if (!recording && !loopRecordingActive.value) {
      playStopSound();
    }
    // Start blip is NOT played here — it's called directly from
    // the tap handler via playStartBlip() to guarantee it works
    // on the very first user interaction.
  });
}
