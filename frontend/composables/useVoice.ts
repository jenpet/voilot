/**
 * Voice recording composable with silence detection.
 *
 * Uses Web Audio API AnalyserNode to monitor volume and auto-stop
 * recording after a configurable silence duration.
 */

// Silence detection tuning constants
const SILENCE_THRESHOLD = 15       // RMS level below which we consider "silence" (0-255 scale)
const SILENCE_DURATION_MS = 1500   // How long silence must last before auto-stop
const MIN_RECORDING_MS = 500       // Minimum recording time before silence detection kicks in
const POLL_INTERVAL_MS = 100       // How often we check the audio level

export function useVoice() {
  const isRecording = ref(false)
  const audioLevel = ref(0)         // Current RMS level (0-255), useful for UI visualisation
  const mediaRecorder = ref<MediaRecorder | null>(null)
  const audioChunks = ref<Blob[]>([])
  const config = useRuntimeConfig()
  const { mark, reset: resetTimer } = useRoundTripTimer()

  // Silence detection state
  let audioContext: AudioContext | null = null
  let analyser: AnalyserNode | null = null
  let silencePollTimer: ReturnType<typeof setInterval> | null = null
  let silenceSince: number | null = null
  let recordingStartedAt = 0

  // Callback for auto-stop — set by the component
  let onAutoStop: (() => void) | null = null

  function setOnAutoStop(cb: () => void) {
    onAutoStop = cb
  }

  /** Compute RMS audio level from analyser frequency data. */
  function computeRMS(): number {
    if (!analyser) return 0
    const data = new Uint8Array(analyser.fftSize)
    analyser.getByteTimeDomainData(data)
    let sum = 0
    for (let i = 0; i < data.length; i++) {
      const val = (data[i] - 128) / 128  // normalise to -1..1
      sum += val * val
    }
    return Math.sqrt(sum / data.length) * 255
  }

  /** Start polling audio levels for silence detection. */
  function startSilenceDetection() {
    silenceSince = null
    silencePollTimer = setInterval(() => {
      const rms = computeRMS()
      audioLevel.value = Math.round(rms)

      const elapsed = performance.now() - recordingStartedAt
      if (elapsed < MIN_RECORDING_MS) return // too early

      if (rms < SILENCE_THRESHOLD) {
        if (silenceSince === null) {
          silenceSince = performance.now()
        } else if (performance.now() - silenceSince >= SILENCE_DURATION_MS) {
          // Silence threshold reached — auto-stop
          onAutoStop?.()
        }
      } else {
        // Sound detected — reset silence timer
        silenceSince = null
      }
    }, POLL_INTERVAL_MS)
  }

  function stopSilenceDetection() {
    if (silencePollTimer) {
      clearInterval(silencePollTimer)
      silencePollTimer = null
    }
    silenceSince = null
    audioLevel.value = 0

    if (audioContext) {
      audioContext.close().catch(() => {})
      audioContext = null
      analyser = null
    }
  }

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

      // Set up audio analysis for silence detection
      audioContext = new AudioContext()
      const source = audioContext.createMediaStreamSource(stream)
      analyser = audioContext.createAnalyser()
      analyser.fftSize = 512
      source.connect(analyser)

      recorder.start(100) // collect chunks every 100ms
      mediaRecorder.value = recorder
      isRecording.value = true
      recordingStartedAt = performance.now()

      startSilenceDetection()
    } catch (err) {
      console.error('Failed to start recording:', err)
    }
  }

  async function stopRecording(): Promise<string | null> {
    if (!mediaRecorder.value || !isRecording.value) return null

    stopSilenceDetection()

    // Reset timer and start STT phase measurement
    resetTimer()
    mark('stt', 'start')

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
          mark('stt', 'end')
          resolve(result.text || null)
        } catch {
          console.error('STT transcription failed')
          mark('stt', 'end')
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
    audioLevel: readonly(audioLevel),
    startRecording,
    stopRecording,
    setOnAutoStop,
  }
}
