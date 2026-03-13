<template>
  <div
    class="block w-full text-left p-4 rounded-xl bg-surface-800 hover:bg-surface-750 border border-surface-700 hover:border-surface-600 transition-colors cursor-pointer"
  >
    <div class="flex items-start justify-between">
      <div class="min-w-0 flex-1">
        <h3 class="font-medium truncate">{{ session.title || 'Untitled Session' }}</h3>
        <div class="flex items-center gap-2 mt-1">
          <span
            class="inline-block px-2 py-0.5 text-xs rounded-full"
            :class="session.mode === 'plan'
              ? 'bg-amber-500/20 text-amber-400'
              : 'bg-green-500/20 text-green-400'"
          >
            {{ session.mode === 'plan' ? 'Planning' : 'Implementing' }}
          </span>
          <span class="text-xs text-surface-400">{{ session.id.slice(0, 8) }}</span>
        </div>
      </div>
      <button
        class="p-1.5 rounded-lg hover:bg-surface-600 text-surface-400 hover:text-red-400 transition-colors"
        @click.stop="$emit('delete')"
      >
        &times;
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Session } from '~/composables/useSession'

defineProps<{
  session: Session
}>()

defineEmits<{
  delete: []
}>()
</script>
