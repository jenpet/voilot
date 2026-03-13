/**
 * Shared composable for measuring voice round-trip timing.
 *
 * Tracks five phases:
 *   1. STT        – stop recording -> transcribed text returned
 *   2. Agent TTFT – message sent -> first text token visible (time-to-first-token)
 *   3. Agent Full – message sent -> agent done (idle)
 *   4. TTS Synth  – TTS request sent -> audio blob received
 *   5. TTS Play   – audio starts playing -> audio finishes
 *
 * Usage: call `mark('phase', 'start')` / `mark('phase', 'end')` from the
 * instrumented composables. The reactive `timings` ref exposes the latest
 * completed round-trip for display in the UI.
 */

export type Phase = 'stt' | 'agent_ttft' | 'agent_full' | 'tts_synth' | 'tts_play'

export interface PhaseTiming {
  label: string
  ms: number
}

export interface RoundTripTimings {
  /** Individual phase durations in ms */
  phases: Record<Phase, PhaseTiming | null>
  /** Total wall-clock time from STT start to TTS playback end */
  totalMs: number | null
  /** Timestamp of the most recent measurement */
  measuredAt: number | null
}

const PHASE_LABELS: Record<Phase, string> = {
  stt: 'STT',
  agent_ttft: 'First Token',
  agent_full: 'Agent',
  tts_synth: 'TTS Synth',
  tts_play: 'TTS Play',
}

function emptyTimings(): RoundTripTimings {
  return {
    phases: {
      stt: null,
      agent_ttft: null,
      agent_full: null,
      tts_synth: null,
      tts_play: null,
    },
    totalMs: null,
    measuredAt: null,
  }
}

// Module-level singleton state so every call site shares the same data.
// We intentionally avoid `useState` here because we need sub-millisecond
// precision marks that don't trigger Vue reactivity on every mark().
const marks: Record<string, number> = {}
let _timings: Ref<RoundTripTimings> | null = null

export function useRoundTripTimer() {
  // Lazily create the shared reactive ref (works both SSR and client)
  if (!_timings) {
    _timings = ref<RoundTripTimings>(emptyTimings()) as Ref<RoundTripTimings>
  }
  const timings = _timings

  /** Record a high-resolution timestamp for a phase boundary. */
  function mark(phase: Phase, edge: 'start' | 'end') {
    const key = `${phase}:${edge}`
    marks[key] = performance.now()

    // When an end mark arrives, compute duration and update reactive state
    if (edge === 'end') {
      const startKey = `${phase}:start`
      const startTime = marks[startKey]
      if (startTime != null) {
        const ms = Math.round(marks[key] - startTime)
        timings.value = {
          ...timings.value,
          phases: {
            ...timings.value.phases,
            [phase]: { label: PHASE_LABELS[phase], ms },
          },
          measuredAt: Date.now(),
        }
      }
    }

    // Compute total when the final phase ends
    if (phase === 'tts_play' && edge === 'end' && marks['stt:start'] != null) {
      timings.value = {
        ...timings.value,
        totalMs: Math.round(marks['tts_play:end'] - marks['stt:start']),
      }
    }
  }

  /** Check whether a voice round-trip is currently active (STT phase started). */
  function isActive(): boolean {
    return marks['stt:start'] != null
  }

  /** Reset all marks and timings for a new round-trip. */
  function reset() {
    for (const key of Object.keys(marks)) {
      delete marks[key]
    }
    timings.value = emptyTimings()
  }

  return {
    timings: readonly(timings),
    mark,
    isActive,
    reset,
  }
}
