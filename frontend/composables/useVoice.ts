export function useVoice() {
  const isRecording = ref(false)
  const mediaRecorder = ref<MediaRecorder | null>(null)
  const audioChunks = ref<Blob[]>([])
  const config = useRuntimeConfig()

  async function startRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          sampleRate: 16000,
        },
      })

      const recorder = new MediaRecorder(stream, {
        mimeType: MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
          ? 'audio/webm;codecs=opus'
          : 'audio/webm',
      })

      audioChunks.value = []

      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          audioChunks.value.push(event.data)
        }
      }

      recorder.start(100) // collect chunks every 100ms
      mediaRecorder.value = recorder
      isRecording.value = true
    } catch (err) {
      console.error('Failed to start recording:', err)
    }
  }

  async function stopRecording(): Promise<string | null> {
    if (!mediaRecorder.value || !isRecording.value) return null

    return new Promise((resolve) => {
      const recorder = mediaRecorder.value!

      recorder.onstop = async () => {
        const audioBlob = new Blob(audioChunks.value, {
          type: recorder.mimeType,
        })

        // Stop all tracks to release the microphone
        recorder.stream.getTracks().forEach(track => track.stop())

        // Send to STT service
        try {
          const result = await $fetch<{ text: string }>(
            `${config.public.backendUrl}/api/stt/transcribe`,
            {
              method: 'POST',
              headers: {
                'Content-Type': recorder.mimeType,
              },
              body: audioBlob,
            },
          )
          resolve(result.text || null)
        } catch {
          console.error('STT transcription failed')
          resolve(null)
        }
      }

      recorder.stop()
      isRecording.value = false
      mediaRecorder.value = null
    })
  }

  return {
    isRecording: readonly(isRecording),
    startRecording,
    stopRecording,
  }
}
