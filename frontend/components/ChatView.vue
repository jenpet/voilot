<template>
  <div ref="scrollContainer" class="p-4 space-y-3 max-w-2xl mx-auto">
    <div v-if="messages.length === 0" class="flex items-center justify-center h-full text-surface-400">
      <p>Start a conversation...</p>
    </div>

    <ChatMessage
      v-for="msg in messages"
      :key="msg.id"
      :message="msg"
    />

    <div v-if="isStreaming" class="flex items-center gap-2 text-surface-400 text-sm">
      <span class="inline-block w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
      <span>Agent is responding...</span>
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
let isUserScrolled = false

// Track if user has scrolled up (to avoid fighting auto-scroll)
function onScroll() {
  if (!scrollContainer.value) return
  const el = scrollContainer.value
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
  isUserScrolled = distanceFromBottom > 100
}

function scrollToBottom() {
  if (isUserScrolled) return
  nextTick(() => {
    if (scrollContainer.value) {
      scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight
    }
  })
}

// Auto-scroll on new messages
watch(
  () => props.messages.length,
  () => scrollToBottom(),
)

// Auto-scroll on streaming content updates (last message content changes)
watch(
  () => props.messages.length > 0 ? props.messages[props.messages.length - 1].content : '',
  () => scrollToBottom(),
)

// Auto-scroll when streaming starts
watch(
  () => props.isStreaming,
  (val) => {
    if (val) scrollToBottom()
  },
)

onMounted(() => {
  scrollContainer.value?.addEventListener('scroll', onScroll, { passive: true })
})

onUnmounted(() => {
  scrollContainer.value?.removeEventListener('scroll', onScroll)
})
</script>
