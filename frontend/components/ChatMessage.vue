<template>
  <div
    class="rounded-xl px-4 py-3 max-w-[85%]"
    :class="message.role === 'user'
      ? 'ml-auto bg-blue-600/20 text-blue-100'
      : 'mr-auto bg-surface-800 text-surface-100'"
  >
    <p class="text-sm whitespace-pre-wrap">{{ message.content }}</p>
    <span class="block mt-1 text-xs text-surface-500">
      {{ message.role === 'user' ? 'You' : 'Agent' }}
    </span>
  </div>
</template>

<script setup lang="ts">
import type { Message } from '~/composables/useAgent'

defineProps<{
  message: Message
}>()
</script>
