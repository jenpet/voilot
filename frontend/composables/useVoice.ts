/**
 * Voice recording composable with silence detection and mic monitoring.
 *
 * Singleton pattern via useState — all callers share the same mic/state.
 *
 * Uses Web Audio API AnalyserNode to monitor volume and auto-stop
 * recording after a configurable silence duration.
 *
 * Monitoring mode: keeps the mic open without recording. When sustained
 * speech is detected (word-level threshold), fires a callback so the
 * caller can stop TTS and seamlessly transition into full recording.
 */

// Silence detection tuning constants
const SILENCE_THRESHOLD = 15       // RMS level below which we consider "silence" (0-255 scale)
const SILENCE_DURATION_MS = 1500   // How long silence must last before auto-stop
const MIN_RECORDING_MS = 500       // Minimum recording time before silence detection kicks in
const POLL_INTERVAL_MS = 100       // How often we check the audio level

// Speech interrupt detection constants (for monitoring mode)
const SPEECH_THRESHOLD = 20        // RMS level above which we consider "speech" (slightly higher than silence)
const SPEECH_SUSTAIN_MS = 400      // How long speech must be sustained to trigger interrupt (~1-2 words)

// Module-level state (singleton across all useVoice() calls)
let _audioContext: AudioContext | null = null
let _analyser: AnalyserNode | null = null
let _micStream: MediaStream | null = null
let _mediaRecorder: MediaRecorder | null = null
let _audioChunks: Blob[] = []

// Timers
let _silencePollTimer: ReturnType<typeof setInterval> | null = null
let _silenceSince: number | null = null
let _recordingStartedAt = 0
let _monitorPollTimer: ReturnType<typeof setInterval> | null = null
let _speechSince: number | null = null

// Callbacks
let _onAutoStop: (() => void) | null = null
let _onSpeechDetected: (() => void) | null = null

export function useVoice() {
  // Shared reactive state via useState (survives HMR and is shared across components)
  const isRecording = useState('voice-recording', () => false)
  const isMonitoring = useState('voice-monitoring', () => false)
  const audioLevel = useState('voice-level', () => 0)

  const config = useRuntimeConfig()
  const { mark, reset: resetTimer } = useRoundTripTimer()

  function setOnAutoStop(cb: () => void) {
    _onAutoStop = cb
  }

  function setOnSpeechDetected(cb: () => void) {
    _onSpeechDetected = cb
  }

  /** Compute RMS audio level from analyser frequency data. */
  function computeRMS(): number {
    if (!_analyser) return 0
    const data = new Uint8Array(_analyser.fftSize)
    _analyser.getByteTimeDomainData(data)
    let sum = 0
    for (let i = 0; i < data.length; i++) {
      const val = (data[i] - 128) / 128  // normalise to -1..1
      sum += val * val
    }
    return Math.sqrt(sum / data.length) * 255
  }

  /** Acquire mic stream and set up analyser. Reuses existing if available. */
  async function ensureMicAndAnalyser(): Promise<void> {
    if (_micStream && _audioContext && _analyser) return

    const stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: true,
        noiseSuppression: true,
        sampleRate: 16000,
      },
    })

    _micStream = stream
    _audioContext = new AudioContext()
    const source = _audioContext.createMediaStreamSource(stream)
    _analyser = _audioContext.createAnalyser()
    _analyser.fftSize = 512
    source.connect(_analyser)
  }

  /** Release mic stream and audio context. */
  function releaseMic() {
    if (_micStream) {
      _micStream.getTracks().forEach(track => track.stop())
      _micStream = null
    }
    if (_audioContext) {
      _audioContext.close().catch(() => {})
      _audioContext = null
      _analyser = null
    }
    audioLevel.value = 0
  }

  // ─── Silence Detection (during recording) ───────────────────────

  function startSilenceDetection() {
    _silenceSince = null
    _silencePollTimer = setInterval(() => {
      const rms = computeRMS()
      audioLevel.value = Math.round(rms)

      const elapsed = performance.now() - _recordingStartedAt
      if (elapsed < MIN_RECORDING_MS) return

      if (rms < SILENCE_THRESHOLD) {
        if (_silenceSince === null) {
          _silenceSince = performance.now()
        } else if (performance.now() - _silenceSince >= SILENCE_DURATION_MS) {
          _onAutoStop?.()
        }
      } else {
        _silenceSince = null
      }
    }, POLL_INTERVAL_MS)
  }

  function stopSilenceDetection() {
    if (_silencePollTimer) {
      clearInterval(_silencePollTimer)
      _silencePollTimer = null
    }
    _silenceSince = null
  }

  // ─── Monitoring Mode (interrupt detection) ──────────────────────

  async function startMonitoring() {
    if (isMonitoring.value || isRecording.value) return

    try {
      await ensureMicAndAnalyser()
      isMonitoring.value = true
      _speechSince = null

      _monitorPollTimer = setInterval(() => {
        const rms = computeRMS()
        audioLevel.value = Math.round(rms)

        if (rms >= SPEECH_THRESHOLD) {
          if (_speechSince === null) {
            _speechSince = performance.now()
          } else if (performance.now() - _speechSince >= SPEECH_SUSTAIN_MS) {
            // Sustained speech detected — fire callback
            stopMonitoringPoll()
            _onSpeechDetected?.()
          }
        } else {
          _speechSince = null
        }
      }, POLL_INTERVAL_MS)
    } catch (err) {
      console.error('Failed to start mic monitoring:', err)
      isMonitoring.value = false
    }
  }

  function stopMonitoringPoll() {
    if (_monitorPollTimer) {
      clearInterval(_monitorPollTimer)
      _monitorPollTimer = null
    }
    _speechSince = null
    isMonitoring.value = false
  }

  function stopMonitoring() {
    stopMonitoringPoll()
    releaseMic()
  }

  // ─── Recording ──────────────────────────────────────────────────

  async function startRecording() {
    try {
      await ensureMicAndAnalyser()

      const recorder = new MediaRecorder(_micStream!, {
        mimeType: MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
          ? 'audio/webm;codecs=opus'
          : 'audio/webm',
      })

      _audioChunks = []

      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          _audioChunks.push(event.data)
        }
      }

      recorder.start(100)
      _mediaRecorder = recorder
      isRecording.value = true
      _recordingStartedAt = performance.now()

      startSilenceDetection()
    } catch (err) {
      console.error('Failed to start recording:', err)
    }
  }

  /**
   * Transition from monitoring mode to recording mode.
   * Reuses the already-open mic stream — no gap in audio capture.
   */
  async function startRecordingFromMonitor() {
    if (!_micStream || !_audioContext || !_analyser) {
      return startRecording()
    }

    // Stop monitoring poll but keep mic open
    stopMonitoringPoll()

    try {
      const recorder = new MediaRecorder(_micStream, {
        mimeType: MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
          ? 'audio/webm;codecs=opus'
          : 'audio/webm',
      })

      _audioChunks = []

      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          _audioChunks.push(event.data)
        }
      }

      recorder.start(100)
      _mediaRecorder = recorder
      isRecording.value = true
      _recordingStartedAt = performance.now()

      startSilenceDetection()
    } catch (err) {
      console.error('Failed to start recording from monitor:', err)
    }
  }

  async function stopRecording(): Promise<string | null> {
    if (!_mediaRecorder || !isRecording.value) return null

    stopSilenceDetection()

    // Reset timer and start STT phase measurement
    resetTimer()
    mark('stt', 'start')

    return new Promise((resolve) => {
      const recorder = _mediaRecorder!

      recorder.onstop = async () => {
        const audioBlob = new Blob(_audioChunks, {
          type: recorder.mimeType,
        })

        // Release the mic
        releaseMic()

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
      _mediaRecorder = null
    })
  }

  return {
    isRecording: readonly(isRecording),
    isMonitoring: readonly(isMonitoring),
    audioLevel: readonly(audioLevel),
    startRecording,
    startRecordingFromMonitor,
    stopRecording,
    startMonitoring,
    stopMonitoring,
    setOnAutoStop,
    setOnSpeechDetected,
  }
}
