<template>
  <div class="flex items-center gap-1.5">
    <span class="text-xs text-surface-400">Mode:</span>
    <button
      class="text-xs px-2 py-0.5 rounded-full transition-colors"
      :class="mode === 'plan'
        ? 'bg-amber-500/20 text-amber-400 hover:bg-amber-500/30'
        : 'bg-green-500/20 text-green-400 hover:bg-green-500/30'"
      @click="$emit('toggle')"
    >
      {{ mode === 'plan' ? 'Planning' : 'Implementing' }}
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  mode: 'plan' | 'implement'
}>()

defineEmits<{
  toggle: []
}>()
</script>
