<template>
  <div class="bg-bg-primary/60 border border-bg-elevated/50 text-text-primary">
    <!-- Header: summary + total duration + chevron toggle -->
    <button
      class="w-full flex items-center gap-2.5 px-4 py-3 text-left hover:bg-bg-elevated/30 transition-colors"
      @click="expanded = !expanded"
    >
      <span class="text-xs flex-shrink-0 text-text-muted">⚙</span>
      <span class="text-xs font-medium text-text-primary flex-1 min-w-0">
        {{ summaryText }}
      </span>
      <span v-if="totalDuration" class="text-xs text-text-muted flex-shrink-0 tabular-nums">
        {{ totalDuration }}
      </span>
      <!-- Chevron -->
      <svg
        class="w-3.5 h-3.5 text-text-muted flex-shrink-0 transition-transform duration-200"
        :class="{ 'rotate-180': expanded }"
        viewBox="0 0 20 20"
        fill="currentColor"
      >
        <path
          fill-rule="evenodd"
          d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
          clip-rule="evenodd"
        />
      </svg>
    </button>

    <!-- Expanded: one row per invocation -->
    <div v-if="expanded" class="border-t border-bg-elevated/40 py-1">
      <div
        v-for="inv in invocations"
        :key="inv.id"
        class="flex items-center gap-2.5 px-4 py-1.5"
      >
        <span class="text-xs flex-shrink-0" :class="inv.iconClass">{{ inv.icon }}</span>
        <span class="text-xs text-text-primary flex-1 min-w-0 truncate">
          {{ inv.title }}
        </span>
        <span v-if="inv.duration" class="text-xs text-text-muted flex-shrink-0 tabular-nums">
          {{ inv.duration }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Message } from '~/composables/useAgent';

const props = defineProps<{
  messages: Message[];
}>();

const expanded = ref(false);

// ─── Distinct tool type count for parent summary ───────────────────

const summaryText = computed(() => {
  const toolNames = new Set<string>();
  for (const m of props.messages) {
    if (m.type === 'tool_use') {
      const name = (m.meta?.tool as string) || 'tool';
      toolNames.add(name);
    }
  }
  const count = toolNames.size;
  if (count === 0) return 'Used tools for this step';
  if (count === 1) return `Used ${[...toolNames][0]} for this step`;
  return `Used ${count} tools for this step`;
});

// ─── Total duration across all invocations ─────────────────────────

const totalDuration = computed(() => {
  let totalMs = 0;
  let hasDuration = false;
  for (const m of props.messages) {
    const d = m.meta?.durationMs as number | undefined;
    if (d && d > 0) {
      totalMs += d;
      hasDuration = true;
    }
  }
  return hasDuration ? formatDuration(totalMs) : '';
});

// ─── Per-invocation rows ───────────────────────────────────────────

interface Invocation {
  id: string;
  icon: string;
  iconClass: string;
  title: string;
  duration: string;
}

const invocations = computed<Invocation[]>(() => {
  return props.messages.map((m) => {
    const tool = (m.meta?.tool as string) || '';
    const title = (m.meta?.title as string) || '';
    const status = (m.meta?.status as string) || '';
    const durationMs = (m.meta?.durationMs as number) || 0;

    let icon: string;
    let iconClass: string;
    if (m.type === 'tool_result') {
      if (status === 'error') {
        icon = '✗';
        iconClass = 'text-accent-warn';
      } else {
        icon = '✓';
        iconClass = 'text-accent/70';
      }
    } else {
      icon = '⚙';
      iconClass = 'text-text-muted';
    }

    // Build a descriptive title
    let displayTitle: string;
    if (title) {
      displayTitle = title;
    } else if (tool) {
      if (m.type === 'tool_result' && m.content) {
        // Show first line of result as context
        const firstLine = m.content.split('\n')[0].slice(0, 120);
        displayTitle = `${tool} — ${firstLine}`;
      } else {
        displayTitle = tool;
      }
    } else {
      displayTitle = m.content || 'tool';
    }

    return {
      id: m.id,
      icon,
      iconClass,
      title: displayTitle,
      duration: durationMs > 0 ? formatDuration(durationMs) : '',
    };
  });
});

// ─── Helpers ───────────────────────────────────────────────────────

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)}s`;
  const m = Math.floor(s / 60);
  const rem = Math.round(s % 60);
  return `${m}m ${rem}s`;
}
</script>
