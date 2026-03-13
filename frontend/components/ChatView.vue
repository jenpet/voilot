<template>
  <div ref="scrollContainer" class="p-4 space-y-4 max-w-2xl mx-auto">
    <div v-if="messages.length === 0" class="flex items-center justify-center h-full text-surface-400">
      <p>Start a conversation...</p>
    </div>

    <ChatMessage
      v-for="(msg, index) in messages"
      :key="index"
      :message="msg"
    />

    <div v-if="isStreaming" class="flex items-center gap-2 text-surface-400 text-sm">
      <span class="animate-pulse">Thinking...</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Message } from '~/composables/useAgent'

const props = defineProps<{
  messages: Message[]
  isStreaming: boolean
}>()

const scrollContainer = ref<HTMLElement>()

// Auto-scroll to bottom on new messages
watch(
  () => props.messages.length,
  () => {
    nextTick(() => {
      if (scrollContainer.value) {
        scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight
      }
    })
  },
)
</script>
