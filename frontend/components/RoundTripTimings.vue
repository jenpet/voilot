<template>
  <div
    v-if="timings.measuredAt"
    class="flex flex-col gap-1.5 px-4 py-2.5 bg-bg-primary border-t border-bg-elevated text-xs font-mono"
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
    <div class="flex items-center gap-3 text-text-muted">
      <template v-for="phase in orderedPhases" :key="phase.key">
        <span v-if="phase.timing" class="flex items-center gap-1">
          <span
            class="inline-block w-2 h-2 rounded-full"
            :class="phaseDotColor(phase.key)"
          />
          {{ phase.timing.label }}: {{ formatMs(phase.timing.ms) }}
        </span>
      </template>
      <span v-if="timings.totalMs != null" class="ml-auto text-text-primary font-semibold">
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
    stt: 'bg-accent-secondary/70 text-text-primary',
    agent_ttft: 'bg-accent/70 text-text-primary',
    agent_full: 'bg-accent/50 text-text-primary',
    tts_synth: 'bg-accent-warn/70 text-text-primary',
    tts_play: 'bg-accent/30 text-text-primary',
  }
  return map[phase]
}

function phaseDotColor(phase: Phase): string {
  const map: Record<Phase, string> = {
    stt: 'bg-accent-secondary',
    agent_ttft: 'bg-accent',
    agent_full: 'bg-accent',
    tts_synth: 'bg-accent-warn',
    tts_play: 'bg-accent',
  }
  return map[phase]
}
</script>
