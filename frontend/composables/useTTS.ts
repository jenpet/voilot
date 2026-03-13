interface TTSQueueItem {
  text: string
  audio?: HTMLAudioElement
}

export function useTTS() {
  const queue = useState<TTSQueueItem[]>('tts-queue', () => [])
  const isPlaying = ref(false)
  const config = useRuntimeConfig()

  // Process the queue sequentially
  async function processQueue() {
    if (isPlaying.value || queue.value.length === 0) return

    isPlaying.value = true
    const item = queue.value[0]

    try {
      // Fetch audio from TTS service
      const response = await fetch(`${config.public.backendUrl}/api/tts/synthesize`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: item.text }),
      })

      if (!response.ok) throw new Error('TTS synthesis failed')

      const audioBlob = await response.blob()
      const audioUrl = URL.createObjectURL(audioBlob)
      const audio = new Audio(audioUrl)

      item.audio = audio

      await new Promise<void>((resolve, reject) => {
        audio.onended = () => {
          URL.revokeObjectURL(audioUrl)
          resolve()
        }
        audio.onerror = () => {
          URL.revokeObjectURL(audioUrl)
          reject(new Error('Audio playback failed'))
        }
        audio.play()
      })
    } catch (err) {
      console.error('TTS playback error:', err)
    } finally {
      // Remove processed item
      queue.value.shift()
      isPlaying.value = false

      // Process next item if available
      if (queue.value.length > 0) {
        processQueue()
      }
    }
  }

  // Add text to the TTS queue
  function enqueue(text: string) {
    queue.value.push({ text })
    processQueue()
  }

  // Stop current playback and clear queue
  function stop() {
    const current = queue.value[0]
    if (current?.audio) {
      current.audio.pause()
      current.audio.currentTime = 0
    }
    queue.value = []
    isPlaying.value = false
  }

  return {
    queue: readonly(queue),
    isPlaying: readonly(isPlaying),
    enqueue,
    stop,
  }
}
