<template>
  <div
    v-if="timings.measuredAt"
    class="flex flex-col gap-1.5 px-4 py-2.5 bg-surface-800 border-t border-surface-700 text-xs font-mono"
  >
    <!-- Phase bar -->
    <div class="flex items-center gap-1 h-4">
      <template v-for="phase in orderedPhases" :key="phase.key">
        <div
          v-if="phase.timing"
          class="h-full rounded-sm flex items-center justify-center text-[10px] font-medium leading-none overflow-hidden"
          :class="phaseColor(phase.key)"
          :style="{ flexGrow: phase.timing.ms, minWidth: '32px' }"
          :title="`${phase.timing.label}: ${formatMs(phase.timing.ms)}`"
        >
          {{ phase.timing.label }}
        </div>
      </template>
    </div>
    <!-- Phase details -->
    <div class="flex items-center gap-3 text-surface-400">
      <template v-for="phase in orderedPhases" :key="phase.key">
        <span v-if="phase.timing" class="flex items-center gap-1">
          <span
            class="inline-block w-2 h-2 rounded-full"
            :class="phaseDotColor(phase.key)"
          />
          {{ phase.timing.label }}: {{ formatMs(phase.timing.ms) }}
        </span>
      </template>
      <span v-if="timings.totalMs != null" class="ml-auto text-surface-300 font-semibold">
        Total: {{ formatMs(timings.totalMs) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Phase, PhaseTiming } from '~/composables/useRoundTripTimer';

const { timings } = useRoundTripTimer()

const PHASE_ORDER: Phase[] = ['stt', 'agent_ttft', 'agent_full', 'tts_synth', 'tts_play']

const orderedPhases = computed(() =>
  PHASE_ORDER.map(key => ({
    key,
    timing: timings.value.phases[key],
  })),
)

function formatMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function phaseColor(phase: Phase): string {
  const map: Record<Phase, string> = {
    stt: 'bg-amber-600/70 text-amber-100',
    agent_ttft: 'bg-blue-600/70 text-blue-100',
    agent_full: 'bg-indigo-600/70 text-indigo-100',
    tts_synth: 'bg-purple-600/70 text-purple-100',
    tts_play: 'bg-green-600/70 text-green-100',
  }
  return map[phase]
}

function phaseDotColor(phase: Phase): string {
  const map: Record<Phase, string> = {
    stt: 'bg-amber-500',
    agent_ttft: 'bg-blue-500',
    agent_full: 'bg-indigo-500',
    tts_synth: 'bg-purple-500',
    tts_play: 'bg-green-500',
  }
  return map[phase]
}
</script>
