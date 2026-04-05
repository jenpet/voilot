/**
 * Low-level audio synthesis utilities for generating PCM tones
 * as HTMLAudioElement instances.
 *
 * Shared between useRecordingFeedback and useAudioFeedback.
 * All audio is generated as WAV blobs played via HTMLAudioElement
 * (not Web Audio API oscillators) because browser AEC suppresses
 * oscillator output while the mic is open.
 */

const SAMPLE_RATE = 24000;

/** Generate a PCM sine-wave blip as a Float32Array. */
export function generateSineBlip(
  freq: number,
  durationMs: number,
  volume: number,
  sampleRate: number = SAMPLE_RATE,
): Float32Array {
  const numSamples = Math.floor((durationMs / 1000) * sampleRate);
  const samples = new Float32Array(numSamples);
  const fadeLen = Math.floor(sampleRate * 0.008); // 8ms fade

  for (let i = 0; i < numSamples; i++) {
    let amp = volume;
    if (i < fadeLen) amp *= i / fadeLen;
    if (i > numSamples - fadeLen) amp *= (numSamples - i) / fadeLen;
    samples[i] = amp * Math.sin(2 * Math.PI * freq * i / sampleRate);
  }
  return samples;
}

/**
 * Generate a frequency sweep (glide) from startFreq to endFreq.
 * Useful for descending/ascending transition tones.
 */
export function generateSweep(
  startFreq: number,
  endFreq: number,
  durationMs: number,
  volume: number,
  sampleRate: number = SAMPLE_RATE,
): Float32Array {
  const numSamples = Math.floor((durationMs / 1000) * sampleRate);
  const samples = new Float32Array(numSamples);
  const fadeLen = Math.floor(sampleRate * 0.008);

  for (let i = 0; i < numSamples; i++) {
    const t = i / numSamples;
    const freq = startFreq + (endFreq - startFreq) * t;
    let amp = volume;
    if (i < fadeLen) amp *= i / fadeLen;
    if (i > numSamples - fadeLen) amp *= (numSamples - i) / fadeLen;
    samples[i] = amp * Math.sin(2 * Math.PI * freq * i / sampleRate);
  }
  return samples;
}

/** Concatenate multiple Float32Arrays into one. */
export function concatSamples(...arrays: Float32Array[]): Float32Array {
  const totalLength = arrays.reduce((sum, a) => sum + a.length, 0);
  const result = new Float32Array(totalLength);
  let offset = 0;
  for (const arr of arrays) {
    result.set(arr, offset);
    offset += arr.length;
  }
  return result;
}

/** Create a silence buffer of the given duration. */
export function generateSilence(
  durationMs: number,
  sampleRate: number = SAMPLE_RATE,
): Float32Array {
  return new Float32Array(Math.floor((durationMs / 1000) * sampleRate));
}

/** Encode Float32 PCM samples into a WAV file ArrayBuffer. */
export function encodeWav(samples: Float32Array, sampleRate: number = SAMPLE_RATE): ArrayBuffer {
  const numSamples = samples.length;
  const bytesPerSample = 2;
  const dataSize = numSamples * bytesPerSample;
  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);

  writeString(view, 0, 'RIFF');
  view.setUint32(4, 36 + dataSize, true);
  writeString(view, 8, 'WAVE');
  writeString(view, 12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * bytesPerSample, true);
  view.setUint16(32, bytesPerSample, true);
  view.setUint16(34, 16, true);
  writeString(view, 36, 'data');
  view.setUint32(40, dataSize, true);

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

/** Create an HTMLAudioElement from PCM samples. */
export function createAudioFromSamples(
  samples: Float32Array,
  sampleRate: number = SAMPLE_RATE,
): HTMLAudioElement {
  const wavBuffer = encodeWav(samples, sampleRate);
  const blob = new Blob([wavBuffer], { type: 'audio/wav' });
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.volume = 1.0;
  return audio;
}

export { SAMPLE_RATE };
