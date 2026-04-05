/**
 * Central audio feedback manager for Voilot's voice-first experience.
 *
 * Owns all non-recording audio cues (handoff, working hum, chimes,
 * error tones, etc.) and enforces priority, de-duplication, and
 * ordered playback rules.
 *
 * Design contract:
 * - Exactly one audio behavior at a time (priority-based).
 * - Working hum fades out after HUM_FADE_START_MS, spoken check-ins
 *   every CHECK_IN_INTERVAL_MS while agent is streaming.
 * - All chimes/tones that interrupt the hum must await stopWorkingHum()
 *   before playing.
 * - Debounce identical cues within DEBOUNCE_MS.
 *
 * All sounds are programmatic sine-wave WAVs played via HTMLAudioElement
 * (not Web Audio API oscillators — AEC suppresses those while mic is open).
 */

import {
  generateSineBlip,
  generateSweep,
  concatSamples,
  generateSilence,
  createAudioFromSamples,
  SAMPLE_RATE,
} from './audioSynth';

// ── Timing constants ────────────────────────────────────────────────
export const HUM_FADE_START_MS = 17_000;   // Start fading hum after ~17s
export const HUM_FADE_DURATION_MS = 3_000; // Fade takes 3s (total ~20s)
export const CHECK_IN_INTERVAL_MS = 30_000;
export const HUM_HARD_CAP_MS = 90_000;
export const WATCHDOG_TIMEOUT_MS = 5_000;
export const DEBOUNCE_MS = 700;

// ── Volume hierarchy ────────────────────────────────────────────────
// TTS voice plays at 1.0 (loudest). All synth sounds sit below TTS
// but are clearly audible. Grouped into tiers:
const VOL_ALERT   = 0.55;  // error, warning, permission — attention-critical
const VOL_BLIP    = 0.50;  // recording start/stop blips (in useRecordingFeedback)
const VOL_CHIME   = 0.45;  // handoff, question, success, cancel, STT failure
const VOL_SUBTLE  = 0.35;  // loop tick, mode signature, reconnect chime
const VOL_HUM     = 0.20;  // ambient heartbeat working loop

// ── Sound definitions ───────────────────────────────────────────────

/** Ascending two-tone chime (reuses existing thinking jingle frequencies). */
function buildHandoffTone(): Float32Array {
  const t1 = generateSineBlip(523, 70, VOL_CHIME, SAMPLE_RATE);  // C5
  const gap = generateSilence(30, SAMPLE_RATE);
  const t2 = generateSineBlip(659, 70, VOL_CHIME, SAMPLE_RATE);  // E5
  return concatSamples(t1, gap, t2);
}

/**
 * Working heartbeat loop chunk — one full beat cycle at 40 BPM (1500ms).
 * Mid-range (90 Hz), less subby, more chest feel.
 * Loops seamlessly via the ended→replay handler.
 */
function buildHumChunk(): Float32Array {
  const bpm = 40;
  const beatMs = 60_000 / bpm; // 1500ms
  const numSamples = Math.floor((beatMs / 1000) * SAMPLE_RATE);
  const samples = new Float32Array(numSamples);
  const vol = VOL_HUM;

  const lubFreq = 90;
  const dubFreq = 110;
  const lubDecay = 28;
  const dubDecay = 38;
  const dubDelaySec = 0.19;
  const dubAmpScale = 0.5;
  const resonanceFreq = 70;
  const resonanceAmp = 0.12;

  const lubLen = Math.floor(0.12 * SAMPLE_RATE);
  const dubDelay = Math.floor(dubDelaySec * SAMPLE_RATE);
  const dubLen = Math.floor(0.09 * SAMPLE_RATE);
  const resLen = Math.floor(0.4 * SAMPLE_RATE);

  for (let i = 0; i < numSamples; i++) {
    let s = 0;

    // Lub (first thump)
    if (i < lubLen) {
      const lt = i / SAMPLE_RATE;
      const env = Math.exp(-lt * lubDecay);
      s += env * Math.sin(2 * Math.PI * lubFreq * lt)
        + 0.4 * env * Math.sin(2 * Math.PI * lubFreq * 1.5 * lt)
        + 0.2 * env * Math.sin(2 * Math.PI * lubFreq * 2.2 * lt);
    }

    // Dub (second thump — softer, slightly higher)
    if (i >= dubDelay && i < dubDelay + dubLen) {
      const dt = (i - dubDelay) / SAMPLE_RATE;
      const env = dubAmpScale * Math.exp(-dt * dubDecay);
      s += env * Math.sin(2 * Math.PI * dubFreq * dt)
        + 0.3 * env * Math.sin(2 * Math.PI * dubFreq * 1.6 * dt);
    }

    // Body resonance tail
    if (i < resLen) {
      const rt = i / SAMPLE_RATE;
      const rEnv = resonanceAmp * Math.exp(-rt * 8);
      s += rEnv * Math.sin(2 * Math.PI * resonanceFreq * rt);
    }

    samples[i] = vol * s;
  }
  return samples;
}

/** Descending two-tone: attention + friendly. */
function buildQuestionChime(): Float32Array {
  const t1 = generateSineBlip(1047, 90, VOL_CHIME, SAMPLE_RATE);  // C6
  const gap = generateSilence(20, SAMPLE_RATE);
  const t2 = generateSineBlip(880, 90, VOL_CHIME, SAMPLE_RATE);   // A5
  return concatSamples(t1, gap, t2);
}

/** Descending two-tone: slightly urgent. */
function buildPermissionChime(): Float32Array {
  const t1 = generateSineBlip(587, 90, VOL_ALERT, SAMPLE_RATE);  // D5
  const gap = generateSilence(20, SAMPLE_RATE);
  const t2 = generateSineBlip(440, 90, VOL_ALERT, SAMPLE_RATE);  // A4
  return concatSamples(t1, gap, t2);
}

/** Ascending triad: positive resolution. */
function buildSuccessChime(): Float32Array {
  const t1 = generateSineBlip(523, 80, VOL_CHIME, SAMPLE_RATE);  // C5
  const g1 = generateSilence(20, SAMPLE_RATE);
  const t2 = generateSineBlip(659, 80, VOL_CHIME, SAMPLE_RATE);  // E5
  const g2 = generateSilence(20, SAMPLE_RATE);
  const t3 = generateSineBlip(784, 80, VOL_CHIME, SAMPLE_RATE);  // G5
  return concatSamples(t1, g1, t2, g2, t3);
}

/** Single low tone: generic error. */
function buildErrorTone(): Float32Array {
  return generateSineBlip(330, 200, VOL_ALERT, SAMPLE_RATE);
}

/** Alternating two tones: system warning. */
function buildWarningTone(): Float32Array {
  const t1 = generateSineBlip(440, 120, VOL_ALERT, SAMPLE_RATE);
  const gap = generateSilence(20, SAMPLE_RATE);
  const t2 = generateSineBlip(330, 120, VOL_ALERT, SAMPLE_RATE);
  return concatSamples(t1, gap, t2);
}

/** Single short tap: mode identity. */
function buildModeSignature(): Float32Array {
  return generateSineBlip(392, 100, VOL_SUBTLE, SAMPLE_RATE);
}

/** Very short tick: "mic is hot" in voice loop. */
function buildLoopListeningTick(): Float32Array {
  return generateSineBlip(1200, 30, VOL_SUBTLE, SAMPLE_RATE);
}

/** Descending sweep: "didn't catch that". */
function buildSTTFailureTone(): Float32Array {
  return generateSweep(350, 280, 150, VOL_CHIME, SAMPLE_RATE);
}

/** Descending sweep: abort confirmed. */
function buildCancelTone(): Float32Array {
  return generateSweep(500, 350, 120, VOL_CHIME, SAMPLE_RATE);
}

/** Ascending two-tone: connection restored. */
function buildReconnectChime(): Float32Array {
  const t1 = generateSineBlip(600, 65, VOL_SUBTLE, SAMPLE_RATE);
  const gap = generateSilence(20, SAMPLE_RATE);
  const t2 = generateSineBlip(800, 65, VOL_SUBTLE, SAMPLE_RATE);
  return concatSamples(t1, gap, t2);
}

// ── Audio element cache ─────────────────────────────────────────────

type SoundName =
  | 'handoff'
  | 'hum'
  | 'questionChime'
  | 'permissionChime'
  | 'successChime'
  | 'errorTone'
  | 'warningTone'
  | 'modeSignature'
  | 'loopListeningTick'
  | 'sttFailureTone'
  | 'cancelTone'
  | 'reconnectChime';

const _builders: Record<SoundName, () => Float32Array> = {
  handoff: buildHandoffTone,
  hum: buildHumChunk,
  questionChime: buildQuestionChime,
  permissionChime: buildPermissionChime,
  successChime: buildSuccessChime,
  errorTone: buildErrorTone,
  warningTone: buildWarningTone,
  modeSignature: buildModeSignature,
  loopListeningTick: buildLoopListeningTick,
  sttFailureTone: buildSTTFailureTone,
  cancelTone: buildCancelTone,
  reconnectChime: buildReconnectChime,
};

let _cache: Record<string, HTMLAudioElement> | null = null;

function ensureCache(): Record<string, HTMLAudioElement> {
  if (_cache) return _cache;
  _cache = {};
  for (const [name, build] of Object.entries(_builders)) {
    _cache[name] = createAudioFromSamples(build());
  }
  return _cache;
}

function playOneShot(name: SoundName): void {
  const cache = ensureCache();
  const audio = cache[name];
  if (audio) {
    audio.currentTime = 0;
    audio.play().catch(() => {});
  }
}

// ── Hum loop state ──────────────────────────────────────────────────

let _humPlaying = false;
let _humFadeTimer: ReturnType<typeof setTimeout> | null = null;
let _humStopTimer: ReturnType<typeof setTimeout> | null = null;
let _humLoopHandler: (() => void) | null = null;
let _humStopResolve: (() => void) | null = null;
let _checkInTimer: ReturnType<typeof setInterval> | null = null;
let _humHardCapTimer: ReturnType<typeof setTimeout> | null = null;

// Watchdog state
let _watchdogTimer: ReturnType<typeof setTimeout> | null = null;

// Debounce tracking
const _lastPlayTime: Record<string, number> = {};

// Tool activity tracking (for spoken check-in context)
let _toolsActiveThisTurn = false;

// TTS enqueue callback — set by the consumer (useAgent) to allow
// spoken check-ins without a circular dependency on useTTS.
let _enqueueTTS: ((text: string) => void) | null = null;

/**
 * Register the TTS enqueue function. Called once by the consumer
 * so the audio feedback system can produce spoken check-ins.
 */
export function setTTSEnqueue(fn: (text: string) => void): void {
  _enqueueTTS = fn;
}

/** Mark that tools are active this turn (for check-in context). */
export function notifyToolActivity(): void {
  _toolsActiveThisTurn = true;
}

// ── Debounce helper ─────────────────────────────────────────────────

function shouldDebounce(name: string): boolean {
  const now = Date.now();
  const last = _lastPlayTime[name] || 0;
  if (now - last < DEBOUNCE_MS) return true;
  _lastPlayTime[name] = now;
  return false;
}

// ── Public API ──────────────────────────────────────────────────────

/** Initialize audio elements eagerly. Call once on client side. */
export function initAudioFeedback(): void {
  ensureCache();
}

/** Play the handoff tone (message submitted to agent). */
export function playHandoff(): void {
  if (shouldDebounce('handoff')) return;
  playOneShot('handoff');
}

/**
 * Start the working hum loop. Idempotent — no-op if already active.
 * Hum fades out after HUM_FADE_START_MS, then spoken check-ins
 * begin every CHECK_IN_INTERVAL_MS.
 */
export function startWorkingHum(): void {
  if (_humPlaying) return;
  _humPlaying = true;
  _toolsActiveThisTurn = false;

  const cache = ensureCache();
  const audio = cache.hum;
  if (!audio) return;

  // Loop the hum chunk
  audio.volume = 1.0;
  audio.currentTime = 0;
  _humLoopHandler = () => {
    if (_humPlaying) {
      audio.currentTime = 0;
      audio.play().catch(() => {});
    }
  };
  audio.addEventListener('ended', _humLoopHandler);
  audio.play().catch(() => {});

  // Schedule fade-out
  _humFadeTimer = setTimeout(() => {
    fadeOutHum(audio, HUM_FADE_DURATION_MS);
  }, HUM_FADE_START_MS);

  // Schedule spoken check-ins after fade completes
  const checkInDelay = HUM_FADE_START_MS + HUM_FADE_DURATION_MS;
  _humStopTimer = setTimeout(() => {
    startCheckIns();
  }, checkInDelay);

  // Hard cap
  _humHardCapTimer = setTimeout(() => {
    stopWorkingHum();
    if (_enqueueTTS) {
      _enqueueTTS('Agent is still working, please wait.');
    }
  }, HUM_HARD_CAP_MS);
}

function fadeOutHum(audio: HTMLAudioElement, durationMs: number): void {
  const steps = 20;
  const interval = durationMs / steps;
  const startVolume = audio.volume;
  let step = 0;

  const fadeInterval = setInterval(() => {
    step++;
    audio.volume = Math.max(0, startVolume * (1 - step / steps));
    if (step >= steps) {
      clearInterval(fadeInterval);
      audio.pause();
      // Don't set _humPlaying = false — check-ins take over
    }
  }, interval);
}

function startCheckIns(): void {
  if (_checkInTimer) return;
  _checkInTimer = setInterval(() => {
    if (!_humPlaying || !_enqueueTTS) return;
    const msg = _toolsActiveThisTurn
      ? 'Still working, running tools.'
      : 'Still working.';
    _enqueueTTS(msg);
  }, CHECK_IN_INTERVAL_MS);
}

/**
 * Stop the working hum. Returns a Promise that resolves once audio
 * is fully stopped, ensuring chimes can play after without overlap.
 * Idempotent — resolves immediately if hum is not active.
 */
export function stopWorkingHum(): Promise<void> {
  return new Promise<void>((resolve) => {
    if (!_humPlaying) {
      resolve();
      return;
    }

    _humPlaying = false;
    _toolsActiveThisTurn = false;

    // Clear all timers
    if (_humFadeTimer) { clearTimeout(_humFadeTimer); _humFadeTimer = null; }
    if (_humStopTimer) { clearTimeout(_humStopTimer); _humStopTimer = null; }
    if (_humHardCapTimer) { clearTimeout(_humHardCapTimer); _humHardCapTimer = null; }
    if (_checkInTimer) { clearInterval(_checkInTimer); _checkInTimer = null; }

    const cache = _cache;
    const audio = cache?.hum;
    if (audio) {
      if (_humLoopHandler) {
        audio.removeEventListener('ended', _humLoopHandler);
        _humLoopHandler = null;
      }
      audio.pause();
      audio.volume = 1.0;
      audio.currentTime = 0;
    }

    // Resolve on next microtask to ensure audio engine has released
    queueMicrotask(resolve);
  });
}

/** Is the working hum currently active? */
export function isHumActive(): boolean {
  return _humPlaying;
}

/** Play question chime. Caller must await stopWorkingHum() first. */
export function playQuestionChime(): void {
  if (shouldDebounce('questionChime')) return;
  playOneShot('questionChime');
}

/** Play permission chime. Caller must await stopWorkingHum() first. */
export function playPermissionChime(): void {
  if (shouldDebounce('permissionChime')) return;
  playOneShot('permissionChime');
}

/** Play success chime. Caller must await stopWorkingHum() first. */
export function playSuccessChime(): void {
  if (shouldDebounce('successChime')) return;
  playOneShot('successChime');
}

/** Play generic error tone. Caller must await stopWorkingHum() first. */
export function playErrorTone(): void {
  if (shouldDebounce('errorTone')) return;
  playOneShot('errorTone');
}

/** Play system warning tone (disconnect, mic denied). */
export function playWarningTone(): void {
  if (shouldDebounce('warningTone')) return;
  playOneShot('warningTone');
}

/** Play mode switch signature. */
export function playModeSignature(): void {
  if (shouldDebounce('modeSignature')) return;
  playOneShot('modeSignature');
}

/** Play subtle tick when voice loop auto-starts recording. */
export function playLoopListeningTick(): void {
  if (shouldDebounce('loopListeningTick')) return;
  playOneShot('loopListeningTick');
}

/** Play STT failure tone. */
export function playSTTFailureTone(): void {
  if (shouldDebounce('sttFailureTone')) return;
  playOneShot('sttFailureTone');
}

/**
 * Play STT failure tone + spoken announcement.
 * Used in manual recording mode where the user needs verbal feedback.
 */
export function announceSTTFailure(): void {
  playSTTFailureTone();
  if (_enqueueTTS) {
    _enqueueTTS("I didn't hear anything.");
  }
}

/** Play cancel/abort tone. */
export function playCancelTone(): void {
  if (shouldDebounce('cancelTone')) return;
  playOneShot('cancelTone');
}

/** Play reconnect chime. */
export function playReconnectChime(): void {
  if (shouldDebounce('reconnectChime')) return;
  playOneShot('reconnectChime');
}

// ── Watchdog ────────────────────────────────────────────────────────

/**
 * Start a watchdog timer. If no agent event arrives (caller must
 * call cancelWatchdog()) within timeoutMs, stops the hum and plays
 * an error cue.
 */
export function startWatchdog(timeoutMs: number = WATCHDOG_TIMEOUT_MS): void {
  cancelWatchdog();
  _watchdogTimer = setTimeout(async () => {
    _watchdogTimer = null;
    await stopWorkingHum();
    playErrorTone();
    if (_enqueueTTS) {
      _enqueueTTS("I didn't catch that. Please try again.");
    }
  }, timeoutMs);
}

/** Cancel the watchdog (agent acknowledged the message). */
export function cancelWatchdog(): void {
  if (_watchdogTimer) {
    clearTimeout(_watchdogTimer);
    _watchdogTimer = null;
  }
}

/** Is the watchdog currently running? */
export function isWatchdogActive(): boolean {
  return _watchdogTimer !== null;
}

// ── Reset (for testing) ─────────────────────────────────────────────

/**
 * Reset all internal state. Only for use in tests.
 */
export function _resetForTesting(): void {
  _humPlaying = false;
  if (_humFadeTimer) { clearTimeout(_humFadeTimer); _humFadeTimer = null; }
  if (_humStopTimer) { clearTimeout(_humStopTimer); _humStopTimer = null; }
  if (_humHardCapTimer) { clearTimeout(_humHardCapTimer); _humHardCapTimer = null; }
  if (_checkInTimer) { clearInterval(_checkInTimer); _checkInTimer = null; }
  if (_watchdogTimer) { clearTimeout(_watchdogTimer); _watchdogTimer = null; }
  _humLoopHandler = null;
  _humStopResolve = null;
  _toolsActiveThisTurn = false;
  _enqueueTTS = null;
  _cache = null;
  for (const key of Object.keys(_lastPlayTime)) {
    delete _lastPlayTime[key];
  }
}
