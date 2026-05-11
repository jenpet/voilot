<template>
   <div class="fixed inset-0 flex flex-col safe-top safe-bottom">
    <!-- Header -->
    <header class="bg-bg-secondary border-b border-bg-elevated">
      <div class="flex items-center gap-3 px-4 py-5 max-w-[1200px] mx-auto">
      <button
        class="p-1.5 rounded-lg hover:bg-bg-secondary transition-colors"
        @click="router.push('/')"
      >
        <span class="text-text-muted">&larr;</span>
      </button>
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2">
          <h1
            v-if="!editingTitle && session"
            class="text-sm font-medium truncate cursor-pointer hover:text-accent transition-colors"
            @click="startEditTitle"
          >{{ session.title || 'Untitled Session' }}</h1>
          <span
            v-else-if="!editingTitle && !session"
            class="inline-block w-full h-4 bg-bg-elevated rounded animate-pulse"
          />
          <input
            v-else
            ref="titleInputRef"
            v-model="editTitleText"
            class="text-sm font-medium bg-bg-primary border border-accent rounded px-1.5 py-0.5 outline-none w-full max-w-[300px]"
            maxlength="80"
            @keydown.enter.prevent="saveTitle"
            @keydown.escape.prevent="cancelEditTitle"
            @blur="saveTitle"
          />
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
        class="px-3 py-1.5 text-xs rounded-lg transition-colors bg-accent-warn/20 text-accent-warn hover:bg-accent-warn/30"
        title="Stop playback"
        @click="stopPlayback"
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
          ? 'bg-accent/20 text-accent hover:bg-accent/25'
          : 'bg-bg-secondary text-text-muted hover:bg-bg-elevated'"
        :title="voiceEnabled ? 'Voice output ON' : 'Voice output OFF'"
        @click="unlockAudio(); toggleVoice()"
      >
        {{ voiceEnabled ? 'Voice ON' : 'Voice OFF' }}
      </button>
      <SettingsPanel />
      </div>
    </header>

    <!-- Chat Messages -->
    <main class="flex-1 min-h-0">
      <ChatView
        ref="chatViewRef"
        :messages="messages"
        :is-streaming="isStreaming"
        :is-loading="isLoading"
        :has-pending-permission="hasPendingPermission"
        :has-pending-question="hasPendingQuestion"
        :agent-name="session?.agent"
      />
    </main>

    <!-- Round-trip timing display (shows after a voice interaction) -->
    <RoundTripTimings v-if="showRoundTripTimings" />

    <!-- Input Area -->
    <div class="border-t border-bg-elevated bg-bg-secondary">
      <div class="px-4 pt-4 max-w-[1200px] mx-auto" :class="keyboardVisible ? 'pb-4' : 'pb-8'">
      <!-- Custom answer hint when a question is pending -->
      <p v-if="hasPendingQuestion" class="text-xs text-accent/70 mb-2 text-center">
        Type or speak a custom answer, or select an option above
      </p>
      <div class="flex items-end gap-2">
        <template v-if="isBusy && !isRecording">
          <!-- Stop button — replaces input row when agent is streaming or TTS is playing (but not if user is recording) -->
          <button
            class="flex-1 p-3 rounded-xl bg-accent-warn hover:bg-accent-warn active:bg-accent-warn transition-colors text-white font-medium text-sm flex items-center justify-center gap-2"
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
            class="flex-1 bg-bg-secondary border border-bg-elevated rounded-xl px-4 py-3 text-sm resize-none outline-none focus:border-accent focus:ring-1 focus:ring-accent/50 placeholder-text-muted"
            style="max-height: 6rem; overflow-y: auto;"
            :rows="1"
            placeholder="Message..."
            @input="autoGrow"
            @focus="handleInputFocus"
            @keydown.enter.exact.prevent="handleSend"
          />
          <VoiceButton
            class="flex-shrink-0"
            :keep-mic-open="voiceEnabled || loopRecordingActive"
            @transcription="handleTranscription"
          />
          <button
            class="flex-shrink-0 p-3 rounded-xl transition-colors"
            :class="inputText.trim() ? 'bg-accent text-bg-primary' : 'bg-bg-elevated text-text-muted opacity-50'"
            :disabled="!inputText.trim()"
            @click="handleSend"
          >
            <span class="text-sm font-medium">Send</span>
          </button>
        </template>
      </div>
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
const { renameSession } = useSession()

// Inline title editing
const editingTitle = ref(false)
const editTitleText = ref('')
const titleInputRef = ref<HTMLInputElement>()

function startEditTitle() {
  editTitleText.value = session.value?.title || ''
  editingTitle.value = true
  nextTick(() => titleInputRef.value?.select())
}

async function saveTitle() {
  if (!editingTitle.value) return
  editingTitle.value = false
  const newTitle = editTitleText.value.trim()
  const oldTitle = session.value?.title || ''
  if (newTitle === oldTitle) return
  const ok = await renameSession(sessionId, newTitle)
  if (ok) {
    setTitle(newTitle, newTitle !== '')
  }
}

function cancelEditTitle() {
  editingTitle.value = false
}

// Initialize the action-gated state machine — registers the state accessor
// for debug logs so every entry includes the current interaction state.
useStateMachine()

const {
  session,
  messages,
  isStreaming,
  isLoading,
  hasPendingPermission,
  hasPendingQuestion,
  isTTSPlaying,
  isRecording,
  voiceEnabled,
  loopRecordingActive,
  sendMessage,
  abortSession,
  stopPlayback,
  setTitle,
  setAgent,
  setModel,
  toggleVoice,
  cleanup,
} = useAgent(sessionId)

// Dynamic page title: "<session title> | voilot"
// TODO: include worktree name once worktree info is available on the session
// object (e.g. "<session> | <worktree> | voilot")
useHead({
  title: computed(() => {
    const title = session.value?.title || 'Untitled Session';
    return `${title} | voilot`;
  }),
})

const chatViewRef = ref<{ jumpToBottom: () => void }>()

// Detect iOS virtual keyboard for footer padding adjustment
const keyboardVisible = ref(false)
onMounted(() => {
  if (window.visualViewport) {
    const threshold = 0.75
    const onResize = () => {
      keyboardVisible.value = window.visualViewport!.height < window.innerHeight * threshold
    }
    window.visualViewport.addEventListener('resize', onResize)
    onUnmounted(() => window.visualViewport?.removeEventListener('resize', onResize))
  }
})
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

function autoGrow() {
  const el = inputRef.value;
  if (!el) return;
  el.style.height = 'auto';
  el.style.height = Math.min(el.scrollHeight, 96) + 'px';
}

function handleInputFocus() {
  setTimeout(() => {
    chatViewRef.value?.jumpToBottom();
  }, 300);
}

function handleSend() {
  const text = inputText.value.trim()
  // Allow sending when a question is pending (custom answer), even though isStreaming is true
  if (!text || (isStreaming.value && !hasPendingQuestion.value)) return
  inputText.value = ''
  // Reset textarea height
  if (inputRef.value) inputRef.value.style.height = 'auto';
  sendMessage(text, { origin: 'text' })
  nextTick(() => chatViewRef.value?.jumpToBottom())
}

function handleTranscription(text: string) {
  inputText.value = text
  // Auto-send voice input — same override for pending questions
  const trimmed = inputText.value.trim()
  if (!trimmed || (isStreaming.value && !hasPendingQuestion.value)) return
  inputText.value = ''
  if (inputRef.value) inputRef.value.style.height = 'auto';
  sendMessage(trimmed, { origin: 'voice' })
  nextTick(() => chatViewRef.value?.jumpToBottom())
}
</script>