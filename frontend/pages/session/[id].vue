<template>
  <div class="flex flex-col h-screen">
    <!-- Header -->
    <header class="flex items-center gap-3 px-4 py-3 bg-surface-800 border-b border-surface-700">
      <button
        class="p-1.5 rounded-lg hover:bg-surface-700 transition-colors"
        @click="router.push('/')"
      >
        <span class="text-surface-400">&larr;</span>
      </button>
      <div class="flex-1 min-w-0">
        <h1 class="text-sm font-medium truncate">{{ session?.title || 'Untitled Session' }}</h1>
        <ModeToggle :mode="session?.mode || 'plan'" @toggle="toggleMode" />
      </div>
    </header>

    <!-- Chat Messages -->
    <ChatView
      class="flex-1 overflow-y-auto"
      :messages="messages"
      :is-streaming="isStreaming"
    />

    <!-- Input Area -->
    <div class="border-t border-surface-700 bg-surface-800 p-4">
      <div class="flex items-end gap-3 max-w-2xl mx-auto">
        <textarea
          ref="inputRef"
          v-model="inputText"
          class="flex-1 bg-surface-700 rounded-xl px-4 py-3 text-sm resize-none outline-none focus:ring-2 focus:ring-blue-500/50 placeholder-surface-400"
          :rows="1"
          placeholder="Type a message or hold the mic to speak..."
          @keydown.enter.exact.prevent="sendMessage"
        />
        <VoiceButton
          class="flex-shrink-0"
          @transcription="handleTranscription"
        />
        <button
          class="flex-shrink-0 p-3 rounded-xl bg-blue-600 hover:bg-blue-500 transition-colors disabled:opacity-50"
          :disabled="!inputText.trim() || isStreaming"
          @click="sendMessage"
        >
          <span class="text-sm">Send</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const route = useRoute()
const router = useRouter()
const sessionId = route.params.id as string

const { session, messages, isStreaming, sendMessage: doSendMessage, toggleMode } = useAgent(sessionId)

const inputText = ref('')
const inputRef = ref<HTMLTextAreaElement>()

async function sendMessage() {
  const text = inputText.value.trim()
  if (!text || isStreaming.value) return
  inputText.value = ''
  await doSendMessage(text)
}

function handleTranscription(text: string) {
  inputText.value = text
  // Auto-send voice input
  sendMessage()
}
</script>
