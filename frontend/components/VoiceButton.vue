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
      class="absolute -top-8 left-1/2 -translate-x-1/2 whitespace-nowrap text-xs px-2 py-0.5 rounded bg-surface-700 text-surface-300"
    >
      {{ statusText }}
    </span>
  </div>
</template>

<script setup lang="ts">
const emit = defineEmits<{
  transcription: [text: string]
}>()

const {
  isRecording,
  isMonitoring,
  audioLevel,
  startRecording: doStart,
  stopRecording: doStop,
} = useVoice()

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

// Watch for external recording state changes (e.g., interrupt flow started/stopped recording)
watch(isRecording, (recording) => {
  if (recording && !isProcessing.value) {
    // Recording started externally (interrupt flow) — show listening status
    showStatus('Listening...', 0)
  } else if (!recording && !isProcessing.value) {
    // Recording stopped externally (auto-stop in useAgent) — clear status
    showStatus('', 0)
  }
})

async function finishRecording() {
  isProcessing.value = true
  showStatus('Transcribing...', 0)
  try {
    const text = await doStop()
    if (text) {
      showStatus('', 0)
      emit('transcription', text)
    } else {
      showStatus('No speech detected', 2000)
    }
  } finally {
    isProcessing.value = false
  }
}

async function toggle() {
  if (isProcessing.value) return

  if (isRecording.value) {
    await finishRecording()
  } else {
    statusText.value = ''
    await doStart()
    if (isRecording.value) {
      showStatus('Listening...', 0)
    }
  }
}
</script>
