<template>
  <div class="relative">
    <button
      class="p-3 rounded-xl transition-all select-none"
      :class="buttonClasses"
      :title="isRecording ? 'Tap to stop & send' : 'Tap to speak'"
      @click="toggle"
    >
      <!-- Audio level ring (shown while recording or monitoring) -->
      <div
        v-if="isRecording || isMonitoring"
        class="absolute inset-0 rounded-xl border-2 transition-opacity"
        :class="isRecording ? 'border-red-400' : 'border-purple-400'"
        :style="{ opacity: Math.min(audioLevel / 40, 1) }"
      />
      <svg
        class="w-5 h-5 relative"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          v-if="!isRecording"
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z"
        />
        <!-- Stop square icon when recording -->
        <rect
          v-else
          x="6" y="6" width="12" height="12" rx="2"
          fill="currentColor"
          stroke="none"
        />
      </svg>
    </button>
    <!-- Recording indicator dot -->
    <span
      v-if="isRecording"
      class="absolute -top-1 -right-1 flex h-3 w-3"
    >
      <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
      <span class="relative inline-flex rounded-full h-3 w-3 bg-red-500" />
    </span>
    <!-- Monitoring indicator dot (purple, no ping) -->
    <span
      v-else-if="isMonitoring"
      class="absolute -top-1 -right-1 flex h-3 w-3"
    >
      <span class="relative inline-flex rounded-full h-3 w-3 bg-purple-500" />
    </span>
    <!-- Status text -->
    <span
      v-if="statusText"
      class="absolute -top-8 left-1/2 -translate-x-1/2 whitespace-nowrap text-xs px-2 py-0.5 rounded"
      :class="lastError ? 'bg-red-900/80 text-red-300' : 'bg-surface-700 text-surface-300'"
    >
      {{ statusText }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { unlockAudio } from '~/composables/useTTS'
import { playStartBlip } from '~/composables/useRecordingFeedback'
import { useDebugLog } from '~/composables/useDebugLog'

const { log } = useDebugLog()

const props = defineProps<{
  keepMicOpen?: boolean
}>()

const emit = defineEmits<{
  transcription: [text: string]
}>()

const {
  isRecording,
  isMonitoring,
  audioLevel,
  lastError,
  acquireMicStream,
  startRecording: doStart,
  stopRecording: doStop,
  setOnAutoStop,
} = useVoice()

// Shared loop flag — clear it on manual stop so the stop blip plays
const loopRecordingActive = useState<boolean>('voice-loop-active', () => false)

// Track whether this component initiated the current recording
// (as opposed to the interrupt flow in useAgent)
let manualRecordingActive = false

const statusText = ref('')
const isProcessing = ref(false)
let statusTimeout: ReturnType<typeof setTimeout> | null = null

const buttonClasses = computed(() => {
  if (isProcessing.value) {
    return 'bg-amber-600/50 cursor-wait opacity-70'
  }
  if (isRecording.value) {
    return 'bg-red-600 hover:bg-red-500 animate-pulse scale-110'
  }
  return 'bg-surface-700 hover:bg-surface-600'
})

function showStatus(text: string, duration = 2000) {
  statusText.value = text
  if (statusTimeout) clearTimeout(statusTimeout)
  if (duration > 0) {
    statusTimeout = setTimeout(() => { statusText.value = '' }, duration)
  }
}

// Auto-stop from silence detection during manual recordings — finish
// through the same path as a user tapping stop so VoiceButton handles
// transcription and emits the result.
setOnAutoStop(async () => {
  if (!manualRecordingActive) return
  await finishRecording()
})

// Watch for external recording state changes (e.g., interrupt flow started/stopped recording)
watch(isRecording, (recording) => {
  if (recording && !isProcessing.value && !manualRecordingActive) {
    // Recording started externally (interrupt flow) — show listening status
    showStatus('Listening...', 0)
  } else if (!recording && !isProcessing.value) {
    // Recording stopped externally (auto-stop in useAgent) — clear status
    manualRecordingActive = false
    showStatus('', 0)
  }
})

// Surface mic errors to the UI
watch(lastError, (err) => {
  if (err) {
    showStatus(err, 5000)
  }
})

async function finishRecording() {
  if (isProcessing.value) return  // Prevent double-finish from auto-stop + manual tap
  manualRecordingActive = false
  // Clear loop flag BEFORE stopping so the stop blip plays (useRecordingFeedback
  // checks this flag on isRecording transitions)
  loopRecordingActive.value = false
  isProcessing.value = true
  showStatus('Transcribing...', 0)
  try {
    const text = await doStop(props.keepMicOpen)
    if (text) {
      showStatus('', 0)
      emit('transcription', text)
    } else {
      // No usable transcription — silently reset without announcing failure.
      // Whether the recording was too short, ambient noise only, or STT
      // returned empty, the user doesn't need a spoken "I didn't hear anything"
      // interruption. A brief visual hint is sufficient.
      showStatus('No speech detected', 2000)
    }
  } finally {
    isProcessing.value = false
  }
}

async function toggle() {
  if (isProcessing.value) return
  log('info', 'ui', 'voice_button_tap', { isRecording: isRecording.value })

  // Unlock iOS Safari audio playback on first user gesture.
  // Must happen synchronously inside the tap handler so the browser
  // considers the page "activated" for programmatic audio.play().
  unlockAudio()

  if (isRecording.value) {
    await finishRecording()
  } else {
    // Play start blip directly in the gesture context so it works
    // on the very first tap (browser autoplay policy requires this).
    playStartBlip()

    // CRITICAL: acquireMicStream() must be the FIRST await in this handler.
    // iOS Safari requires getUserMedia to run inside transient user activation
    // (the synchronous call chain of a tap). Any prior await, ref mutation,
    // or async gap can expire the activation and cause NotAllowedError.
    try {
      const stream = await acquireMicStream()
      statusText.value = ''
      manualRecordingActive = true
      await doStart(stream)
      if (isRecording.value) {
        showStatus('Listening...', 0)
      } else {
        manualRecordingActive = false
      }
    } catch (err) {
      manualRecordingActive = false
      const msg = err instanceof Error ? err.message : 'Mic access failed'
      log('error', 'ui', 'voice_button_error', { error: msg })
      showStatus(msg, 5000)
    }
  }
}
</script>
