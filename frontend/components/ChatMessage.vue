<template>
  <div
    class="rounded-xl px-4 py-3 max-w-[85%]"
    :class="messageClasses"
  >
    <!-- Tool use / tool result -->
    <div v-if="message.type === 'tool_use' || message.type === 'tool_result'" class="flex items-start gap-2">
      <span class="text-xs mt-0.5 flex-shrink-0">
        {{ message.type === 'tool_use' ? '⚙' : '✓' }}
      </span>
      <div class="min-w-0">
        <p class="text-xs font-medium text-surface-300">
          {{ toolTitle }}
        </p>
        <p v-if="message.content && message.type === 'tool_result'" class="text-xs text-surface-400 mt-1 truncate">
          {{ message.content.slice(0, 200) }}
        </p>
      </div>
    </div>

    <!-- System message -->
    <div v-else-if="message.role === 'system'" class="text-xs text-surface-400 italic">
      {{ message.content }}
    </div>

    <!-- Regular text message -->
    <div v-else>
      <p class="text-sm whitespace-pre-wrap break-words">{{ message.content }}</p>
      <span class="block mt-1 text-xs text-surface-500">
        {{ message.role === 'user' ? 'You' : 'Agent' }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Message } from '~/composables/useAgent'

const props = defineProps<{
  message: Message
}>()

const messageClasses = computed(() => {
  if (props.message.role === 'user') {
    return 'ml-auto bg-blue-600/20 text-blue-100'
  }
  if (props.message.role === 'system') {
    return 'mx-auto bg-surface-800/50 text-surface-400 max-w-full text-center'
  }
  if (props.message.type === 'tool_use' || props.message.type === 'tool_result') {
    return 'mr-auto bg-surface-800/60 border border-surface-700/50 text-surface-300'
  }
  return 'mr-auto bg-surface-800 text-surface-100'
})

const toolTitle = computed(() => {
  const tool = props.message.meta?.tool as string | undefined
  const status = props.message.meta?.status as string | undefined
  const title = props.message.meta?.title as string | undefined
  if (title) return title
  if (tool) return `${tool}${status ? ` (${status})` : ''}`
  return props.message.content
})
</script>
