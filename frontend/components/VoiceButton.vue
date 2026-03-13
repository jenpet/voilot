<template>
  <div class="relative">
    <button
      class="p-3 rounded-xl transition-all select-none"
      :class="buttonClasses"
      :title="isRecording ? 'Release to send' : 'Hold to speak'"
      @mousedown="startRecording"
      @mouseup="stopRecording"
      @mouseleave="stopRecording"
      @touchstart.prevent="startRecording"
      @touchend.prevent="stopRecording"
    >
      <svg
        class="w-5 h-5"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z"
        />
      </svg>
    </button>
    <!-- Recording indicator -->
    <span
      v-if="isRecording"
      class="absolute -top-1 -right-1 flex h-3 w-3"
    >
      <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
      <span class="relative inline-flex rounded-full h-3 w-3 bg-red-500" />
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

const { isRecording, startRecording: doStart, stopRecording: doStop } = useVoice()

const statusText = ref('')
let statusTimeout: ReturnType<typeof setTimeout> | null = null

const buttonClasses = computed(() => {
  if (isRecording.value) {
    return 'bg-red-600 hover:bg-red-500 animate-pulse scale-110'
  }
  return 'bg-surface-700 hover:bg-surface-600'
})

function showStatus(text: string, duration = 2000) {
  statusText.value = text
  if (statusTimeout) clearTimeout(statusTimeout)
  statusTimeout = setTimeout(() => { statusText.value = '' }, duration)
}

async function startRecording() {
  statusText.value = ''
  await doStart()
  if (isRecording.value) {
    showStatus('Recording...', 10000)
  }
}

async function stopRecording() {
  if (!isRecording.value) return
  showStatus('Transcribing...', 10000)
  const text = await doStop()
  if (text) {
    showStatus('', 0)
    emit('transcription', text)
  } else {
    showStatus('No speech detected', 2000)
  }
}
</script>
