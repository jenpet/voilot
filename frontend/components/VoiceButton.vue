<template>
  <button
    class="p-3 rounded-xl transition-all"
    :class="isRecording
      ? 'bg-red-600 hover:bg-red-500 animate-pulse'
      : 'bg-surface-700 hover:bg-surface-600'"
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
</template>

<script setup lang="ts">
const emit = defineEmits<{
  transcription: [text: string]
}>()

const { isRecording, startRecording: doStart, stopRecording: doStop } = useVoice()

async function startRecording() {
  await doStart()
}

async function stopRecording() {
  const text = await doStop()
  if (text) {
    emit('transcription', text)
  }
}
</script>
