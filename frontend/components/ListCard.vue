<template>
  <div
    class="block w-full text-left p-4 rounded-xl bg-bg-secondary hover:bg-bg-elevated border border-bg-elevated hover:border-bg-elevated transition-colors cursor-pointer"
  >
    <div class="flex items-center gap-3">
      <!-- Column 1: Main content (truncates) -->
      <div class="min-w-0 flex-1">
        <slot />
      </div>
      <!-- Column 2: Timestamp -->
      <span v-if="timestamp" class="text-xs text-text-muted whitespace-nowrap flex-shrink-0">{{ formatTimestamp(timestamp) }}</span>
      <!-- Column 3: Delete -->
      <slot name="actions">
        <button
          v-if="deletable"
          class="p-1.5 rounded-lg hover:bg-bg-elevated text-text-muted hover:text-accent-warn transition-colors flex-shrink-0"
          @click.stop="$emit('delete')"
        >
          &times;
        </button>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  timestamp?: number
  deletable?: boolean
}>()

defineEmits<{
  delete: []
}>()

function formatTimestamp(ts: number): string {
  const d = new Date(ts)
  const now = new Date()
  const diffMs = now.getTime() - d.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  if (diffMins < 1) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`
  const diffHours = Math.floor(diffMins / 60)
  if (diffHours < 24) return `${diffHours}h ago`
  const diffDays = Math.floor(diffHours / 24)
  if (diffDays < 30) return `${diffDays}d ago`
  const diffMonths = Math.floor(diffDays / 30)
  if (diffMonths < 12) return `${diffMonths}mo ago`
  return `${Math.floor(diffMonths / 12)}y ago`
}
</script>
