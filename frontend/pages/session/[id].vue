<template>
   <div class="flex flex-col h-screen safe-top safe-bottom">
    <!-- Header -->
    <header class="flex items-center gap-3 px-4 py-5 bg-surface-800 border-b border-surface-700">
      <button
        class="p-1.5 rounded-lg hover:bg-surface-700 transition-colors"
        @click="router.push('/')"
      >
        <span class="text-surface-400">&larr;</span>
      </button>
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2">
          <h1 class="text-sm font-medium truncate">{{ session?.title || 'Untitled Session' }}</h1>
          <StatusIndicator />
        </div>
        <div class="flex items-center gap-2 mt-0.5">
          <AgentSelector :agent="session?.agent || 'planitect'" @select="setAgent" />
          <ModelSelector :model="session?.model || ''" :last-used-model="session?.lastUsedModel || ''" @select="setModel" />
        </div>
      </div>
      <!-- Contextual voice button: Stop when TTS playing, otherwise Voice ON/OFF -->
      <button
        v-if="voiceEnabled && isTTSPlaying"
        class="px-3 py-1.5 text-xs rounded-lg transition-colors bg-red-600/30 text-red-300 hover:bg-red-600/50"
        title="Stop playback"
        @click="stopTTS"
      >
        <span class="flex items-center gap-1">
          <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24">
            <rect x="6" y="6" width="12" height="12" rx="2" />
          </svg>
          Stop
        </span>
      </button>
      <button
        v-else
        class="px-3 py-1.5 text-xs rounded-lg transition-colors"
        :class="voiceEnabled
          ? 'bg-purple-600/30 text-purple-300 hover:bg-purple-600/40'
          : 'bg-surface-700 text-surface-400 hover:bg-surface-600'"
        :title="voiceEnabled ? 'Voice output ON' : 'Voice output OFF'"
        @click="unlockAudio(); toggleVoice()"
      >
        {{ voiceEnabled ? 'Voice ON' : 'Voice OFF' }}
      </button>
      <SettingsPanel />
    </header>

    <!-- Chat Messages -->
    <main class="flex-1 min-h-0">
      <ChatView
        :messages="messages"
        :is-streaming="isStreaming"
        :has-pending-permission="hasPendingPermission"
        :has-pending-question="hasPendingQuestion"
      />
    </main>

    <!-- Round-trip timing display (shows after a voice interaction) -->
    <RoundTripTimings v-if="showRoundTripTimings" />

    <!-- Input Area -->
    <div class="border-t border-surface-700 bg-surface-800 px-4 py-4">
      <!-- Custom answer hint when a question is pending -->
      <p v-if="hasPendingQuestion" class="text-xs text-indigo-400/70 mb-2 text-center max-w-2xl mx-auto">
        Type or speak a custom answer, or select an option above
      </p>
      <div class="flex items-end gap-3 max-w-2xl mx-auto">
        <template v-if="isBusy && !isRecording">
          <!-- Stop button — replaces input row when agent is streaming or TTS is playing (but not if user is recording) -->
          <button
            class="flex-1 p-3 rounded-xl bg-red-600 hover:bg-red-500 active:bg-red-700 transition-colors text-white font-medium text-sm flex items-center justify-center gap-2"
            @click="abortSession"
          >
            <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
              <rect x="6" y="6" width="12" height="12" rx="2" />
            </svg>
            Stop
          </button>
        </template>
        <template v-else>
          <textarea
            ref="inputRef"
            v-model="inputText"
            class="flex-1 bg-surface-700 rounded-xl px-4 py-3 text-sm resize-none outline-none focus:ring-2 focus:ring-blue-500/50 placeholder-surface-400"
            :rows="1"
            placeholder="Type a message or tap the mic..."
            @keydown.enter.exact.prevent="handleSend"
          />
          <VoiceButton
            class="flex-shrink-0"
            :keep-mic-open="voiceEnabled || loopRecordingActive"
            @transcription="handleTranscription"
          />
          <button
            class="flex-shrink-0 p-3 rounded-xl bg-blue-600 hover:bg-blue-500 transition-colors disabled:opacity-50"
            :disabled="!inputText.trim()"
            @click="handleSend"
          >
            <span class="text-sm">Send</span>
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { unlockAudio } from '~/composables/useTTS'
import { getState } from '~/composables/useStateMachine'

const route = useRoute()
const router = useRouter()
const sessionId = route.params.id as string
const { showRoundTripTimings } = useSettings()

// Initialize the action-gated state machine — registers the state accessor
// for debug logs so every entry includes the current interaction state.
useStateMachine()

const {
  session,
  messages,
  isStreaming,
  hasPendingPermission,
  hasPendingQuestion,
  isTTSPlaying,
  isRecording,
  voiceEnabled,
  loopRecordingActive,
  sendMessage,
  abortSession,
  stopTTS,
  setAgent,
  setModel,
  toggleVoice,
  cleanup,
} = useAgent(sessionId)

// Acoustic feedback: blip on recording start, double-blip on stop
useRecordingFeedback()

// Track whether cleanup has already been performed (prevent double cleanup
// from both onBeforeRouteLeave and onScopeDispose).
let cleanedUp = false

// Confirm before leaving when the agent is actively working.
// Three outcomes: stay, leave (stop agent), leave (let agent finish).
onBeforeRouteLeave((_to, _from, next) => {
  if (cleanedUp) {
    next()
    return
  }

  const state = getState()

  if (state === 'agent_turn' && isStreaming.value) {
    // Agent is actively working — ask the user
    const stop = window.confirm(
      'The agent is still working. Press OK to stop it, or Cancel to stay.',
    )
    if (stop) {
      cleanedUp = true
      cleanup({ abortBackend: true })
      next()
    } else {
      next(false)
    }
  } else {
    // Not actively streaming — clean up silently
    cleanedUp = true
    cleanup()
    next()
  }
})

// When a question is pending, the agent is technically "busy" (waiting for our
// answer), but we need the input field visible so the user can type a custom
// answer or use voice. Override isBusy to false in that case.
const isBusy = computed(() => {
  if (hasPendingQuestion.value) return false
  return isStreaming.value || isTTSPlaying.value
})

const inputText = ref('')
const inputRef = ref<HTMLTextAreaElement>()

function handleSend() {
  const text = inputText.value.trim()
  // Allow sending when a question is pending (custom answer), even though isStreaming is true
  if (!text || (isStreaming.value && !hasPendingQuestion.value)) return
  inputText.value = ''
  sendMessage(text, { origin: 'text' })
}

function handleTranscription(text: string) {
  inputText.value = text
  // Auto-send voice input — same override for pending questions
  const trimmed = inputText.value.trim()
  if (!trimmed || (isStreaming.value && !hasPendingQuestion.value)) return
  inputText.value = ''
  sendMessage(trimmed, { origin: 'voice' })
}
</script>
