import { describe, it, expect } from 'vitest';
import {
  generateSineBlip,
  generateSweep,
  concatSamples,
  generateSilence,
  encodeWav,
  SAMPLE_RATE,
} from '../audioSynth';

describe('audioSynth', () => {
  describe('generateSineBlip', () => {
    it('produces correct sample count for given duration', () => {
      const samples = generateSineBlip(440, 100, 0.5);
      const expected = Math.floor((100 / 1000) * SAMPLE_RATE);
      expect(samples.length).toBe(expected);
    });

    it('samples are within [-volume, +volume]', () => {
      const volume = 0.35;
      const samples = generateSineBlip(660, 80, volume);
      for (let i = 0; i < samples.length; i++) {
        expect(Math.abs(samples[i])).toBeLessThanOrEqual(volume + 0.001);
      }
    });

    it('applies fade-in (first samples are near zero)', () => {
      const samples = generateSineBlip(440, 100, 1.0);
      // First sample should be very close to zero due to fade-in
      expect(Math.abs(samples[0])).toBeLessThan(0.01);
    });

    it('applies fade-out (last samples are near zero)', () => {
      const samples = generateSineBlip(440, 100, 1.0);
      expect(Math.abs(samples[samples.length - 1])).toBeLessThan(0.01);
    });

    it('returns empty array for 0ms duration', () => {
      const samples = generateSineBlip(440, 0, 0.5);
      expect(samples.length).toBe(0);
    });
  });

  describe('generateSweep', () => {
    it('produces correct sample count', () => {
      const samples = generateSweep(350, 280, 150, 0.2);
      const expected = Math.floor((150 / 1000) * SAMPLE_RATE);
      expect(samples.length).toBe(expected);
    });

    it('samples are within [-volume, +volume]', () => {
      const volume = 0.2;
      const samples = generateSweep(500, 350, 120, volume);
      for (let i = 0; i < samples.length; i++) {
        expect(Math.abs(samples[i])).toBeLessThanOrEqual(volume + 0.001);
      }
    });
  });

  describe('concatSamples', () => {
    it('concatenates multiple arrays', () => {
      const a = new Float32Array([1, 2]);
      const b = new Float32Array([3, 4, 5]);
      const result = concatSamples(a, b);
      expect(result.length).toBe(5);
      expect(Array.from(result)).toEqual([1, 2, 3, 4, 5]);
    });

    it('handles empty arrays', () => {
      const a = new Float32Array([1]);
      const b = new Float32Array([]);
      const result = concatSamples(a, b);
      expect(result.length).toBe(1);
    });

    it('handles three or more arrays', () => {
      const a = new Float32Array([1]);
      const b = new Float32Array([2]);
      const c = new Float32Array([3]);
      const result = concatSamples(a, b, c);
      expect(Array.from(result)).toEqual([1, 2, 3]);
    });
  });

  describe('generateSilence', () => {
    it('produces correct sample count', () => {
      const samples = generateSilence(50);
      const expected = Math.floor((50 / 1000) * SAMPLE_RATE);
      expect(samples.length).toBe(expected);
    });

    it('all samples are zero', () => {
      const samples = generateSilence(10);
      for (let i = 0; i < samples.length; i++) {
        expect(samples[i]).toBe(0);
      }
    });
  });

  describe('encodeWav', () => {
    it('produces valid WAV header', () => {
      const samples = new Float32Array([0, 0.5, -0.5, 1.0]);
      const buffer = encodeWav(samples);
      const view = new DataView(buffer);

      // RIFF header
      expect(String.fromCharCode(view.getUint8(0), view.getUint8(1), view.getUint8(2), view.getUint8(3)))
        .toBe('RIFF');
      expect(String.fromCharCode(view.getUint8(8), view.getUint8(9), view.getUint8(10), view.getUint8(11)))
        .toBe('WAVE');

      // fmt chunk
      expect(view.getUint16(20, true)).toBe(1);  // PCM format
      expect(view.getUint16(22, true)).toBe(1);  // mono
      expect(view.getUint32(24, true)).toBe(SAMPLE_RATE);
      expect(view.getUint16(34, true)).toBe(16);  // 16-bit

      // data chunk
      expect(String.fromCharCode(view.getUint8(36), view.getUint8(37), view.getUint8(38), view.getUint8(39)))
        .toBe('data');
    });

    it('has correct total size', () => {
      const samples = new Float32Array(100);
      const buffer = encodeWav(samples);
      // 44 byte header + 100 samples * 2 bytes
      expect(buffer.byteLength).toBe(44 + 200);
    });

    it('clamps samples to [-1, 1]', () => {
      const samples = new Float32Array([2.0, -2.0]);
      const buffer = encodeWav(samples);
      const view = new DataView(buffer);
      // 2.0 clamped to 1.0 -> 0x7FFF
      expect(view.getInt16(44, true)).toBe(0x7FFF);
      // -2.0 clamped to -1.0 -> -0x7FFF
      expect(view.getInt16(46, true)).toBe(-0x7FFF);
    });
  });
});
