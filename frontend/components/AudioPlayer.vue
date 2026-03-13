<template>
  <div v-if="queue.length > 0" class="fixed bottom-24 right-4 z-50">
    <button
      class="p-2 rounded-full bg-surface-800 border border-surface-600 text-surface-300 hover:text-white transition-colors"
      @click="stop"
    >
      <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
        <rect x="6" y="6" width="12" height="12" rx="1" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
const { queue, stop } = useTTS()
</script>
