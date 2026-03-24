<template>
  <div ref="scrollContainer" class="p-4 space-y-3 max-w-2xl mx-auto">
    <div v-if="messages.length === 0" class="flex items-center justify-center h-full text-surface-400">
      <p>Start a conversation...</p>
    </div>

    <template v-for="(item, idx) in groupedMessages" :key="item.key">
      <!-- Tool group: collapsible block for consecutive tool_use / tool_result -->
      <ToolGroup
        v-if="item.kind === 'tool_group'"
        :messages="item.messages"
      />
      <!-- Regular message -->
      <ChatMessage
        v-else
        :message="item.message"
      />
    </template>

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

// Group consecutive tool_use / tool_result messages into collapsible blocks.
// Everything else stays as a single message.
interface SingleItem {
  kind: 'single'
  key: string
  message: Message
}
interface ToolGroupItem {
  kind: 'tool_group'
  key: string
  messages: Message[]
}
type GroupedItem = SingleItem | ToolGroupItem

const groupedMessages = computed<GroupedItem[]>(() => {
  const result: GroupedItem[] = []
  let toolBatch: Message[] = []

  function flushToolBatch() {
    if (toolBatch.length === 0) return
    result.push({
      kind: 'tool_group',
      key: `tg-${toolBatch[0].id}`,
      messages: [...toolBatch],
    })
    toolBatch = []
  }

  for (const msg of props.messages) {
    const isTool = msg.type === 'tool_use' || msg.type === 'tool_result'
    if (isTool) {
      toolBatch.push(msg)
    } else {
      flushToolBatch()
      result.push({ kind: 'single', key: msg.id, message: msg })
    }
  }
  flushToolBatch()
  return result
})

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
