interface TTSQueueItem {
  text: string
  audio?: HTMLAudioElement
}

export function useTTS() {
  const queue = useState<TTSQueueItem[]>('tts-queue', () => [])
  const isPlaying = ref(false)
  const config = useRuntimeConfig()
  const { mark, isActive: isTimerActive } = useRoundTripTimer()

  // Track whether the first TTS item in a round-trip has been marked
  let firstSynthMarked = false

  // Process the queue sequentially
  async function processQueue() {
    if (isPlaying.value || queue.value.length === 0) return

    isPlaying.value = true
    const item = queue.value[0]
    const shouldMark = isTimerActive()
    const isFirstItem = !firstSynthMarked

    try {
      // Mark TTS synthesis start (only for the first item in a round-trip)
      if (shouldMark && isFirstItem) {
        mark('tts_synth', 'start')
        firstSynthMarked = true
      }

      // Fetch audio from TTS service
      const response = await fetch(`${config.public.backendUrl}/api/tts/synthesize`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: item.text }),
      })

      if (!response.ok) throw new Error('TTS synthesis failed')

      const audioBlob = await response.blob()
      if (shouldMark && isFirstItem) mark('tts_synth', 'end')

      const audioUrl = URL.createObjectURL(audioBlob)
      const audio = new Audio(audioUrl)

      item.audio = audio

      // Mark TTS playback start on first item
      if (shouldMark && isFirstItem) mark('tts_play', 'start')

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
      // Mark synth end on error if it was the first item and synth start was marked
      if (shouldMark && isFirstItem && firstSynthMarked) {
        mark('tts_synth', 'end')
      }
    } finally {
      // Remove processed item
      queue.value.shift()
      isPlaying.value = false

      // Mark TTS playback end when queue is fully drained
      if (queue.value.length === 0) {
        if (shouldMark) mark('tts_play', 'end')
        // Reset first-item flag for next round-trip
        firstSynthMarked = false
      }

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
    firstSynthMarked = false
  }

  return {
    queue: readonly(queue),
    isPlaying: readonly(isPlaying),
    enqueue,
    stop,
  }
}
