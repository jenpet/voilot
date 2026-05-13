interface TTSQueueItem {
  text: string
}

// ─── Debug logging ──────────────────────────────────────────────────
import { useDebugLog } from './useDebugLog';
import type { DebugLogLevel } from './useDebugLog';
import { warmUpBlips } from './useRecordingFeedback';

function _log(level: DebugLogLevel, event: string, data?: Record<string, unknown>) {
  try {
    const { log } = useDebugLog();
    log(level, 'tts', event, data);
  } catch {
    // Composable not available outside setup — ignore
  }
}

// ─── iOS Safari audio unlock ────────────────────────────────────────
//
// iOS Safari requires two things for reliable programmatic audio:
//
// 1. An AudioContext created/resumed inside a user-gesture handler.
//    Once resumed it stays "running" and grants persistent autoplay
//    permission to the page's audio session.
//
// 2. An HTMLAudioElement played at non-zero volume during the gesture
//    to promote the audio session to "playback" category, which
//    overrides the hardware mute/silent switch.
//
// For TTS playback we use a **single pre-created HTMLAudioElement**
// that is warmed up during `unlockAudio()`.  iOS Safari allows
// `.play()` on a previously-played element even outside the gesture
// window, unlike freshly-created `new Audio()` instances which are
// subject to the autoplay gate each time.

let _audioCtx: AudioContext | null = null

function getOrCreateAudioContext(): AudioContext {
  if (!_audioCtx) {
    _audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)()
  }
  return _audioCtx
}

// A tiny WAV file (44 bytes header + 2 bytes of audio = 1 sample at near-zero amplitude).
// NOT silent — amplitude 1/32768 is inaudible but enough to convince iOS that this page
// plays "media" audio, promoting the session to "playback" category (mute switch override).
const TINY_WAV_B64 =
  'UklGRi4AAABXQVZFZm10IBAAAAABAAEARKwAAESsAAABAAgAZGF0YQoAAACA'

// ─── Pre-created reusable HTMLAudioElement for TTS playback ─────────
//
// Created once during unlockAudio() and reused for every TTS chunk.
// iOS Safari allows .play() on a previously-played element without
// requiring a fresh user gesture, unlike new Audio() instances.
let _ttsAudioEl: HTMLAudioElement | null = null

/** Get or create the shared TTS playback element. */
function getTTSAudioEl(): HTMLAudioElement {
  if (!_ttsAudioEl) {
    _ttsAudioEl = new Audio()
    _ttsAudioEl.volume = 1.0
  }
  return _ttsAudioEl
}

/**
 * Unlock iOS Safari's audio playback restriction AND override the mute switch.
 *
 * MUST be called synchronously inside a user-gesture handler
 * (tap / click) — e.g. the mic button or "Voice ON" toggle.
 *
 * Three things happen:
 * 1. The shared AudioContext is created/resumed and a silent buffer
 *    is played through it — this grants persistent autoplay permission.
 * 2. An HTMLAudioElement plays a near-silent WAV at full volume —
 *    this promotes the page's audio session to "playback" mode,
 *    overriding the iOS hardware mute/silent switch.
 * 3. The shared TTS HTMLAudioElement is warmed up by playing the
 *    same tiny WAV — this "unlocks" it so subsequent .play() calls
 *    with blob URLs work outside the gesture window.
 */
export function unlockAudio(): void {
  const ctx = getOrCreateAudioContext()

  // Resume if suspended (Safari creates contexts in suspended state).
  if (ctx.state === 'suspended') {
    ctx.resume().catch(() => {})
  }

  // 1. Play a tiny silent buffer through AudioContext
  try {
    const buf = ctx.createBuffer(1, 1, ctx.sampleRate)
    const src = ctx.createBufferSource()
    src.buffer = buf
    src.connect(ctx.destination)
    src.start(0)
  } catch {
    // best effort
  }

  // 2. Play a near-silent WAV via a throwaway HTMLAudioElement to promote
  //    the audio session to "playback" category (mute switch override).
  try {
    const audio = new Audio(`data:audio/wav;base64,${TINY_WAV_B64}`)
    audio.volume = 1.0
    audio.play().catch(() => {})
  } catch {
    // best effort
  }

  // 3. Warm up the shared TTS element by playing the same tiny WAV.
  //    This grants it autoplay permission that persists beyond the gesture.
  try {
    const ttsEl = getTTSAudioEl()
    ttsEl.src = `data:audio/wav;base64,${TINY_WAV_B64}`
    ttsEl.play().catch(() => {})
  } catch {
    // best effort
  }

  // 4. Pre-warm recording blip HTMLAudioElements so Bluetooth codec
  //    is negotiated before the first real playStartBlip() call.
  warmUpBlips();
}

export function useTTS() {
  const queue = useState<TTSQueueItem[]>('tts-queue', () => [])
  const isPlaying = ref(false)
  const backendUrl = resolveBackendUrl()
  const { mark, isActive: isTimerActive } = useRoundTripTimer()

  // Track whether the first TTS item in a round-trip has been marked
  let firstSynthMarked = false

  // Callback fired when the TTS queue drains completely after processing.
  // Used by useAgent to trigger complete_turn when agent is also done.
  let _onQueueDrained: (() => void) | null = null

  // Abort controller — cancelled by stop() to interrupt in-flight fetches
  // and pending playback.
  let _abortCtrl: AbortController | null = null

  // Process the queue sequentially
  async function processQueue() {
    if (isPlaying.value || queue.value.length === 0) return

    isPlaying.value = true
    const item = queue.value[0]
    const shouldMark = isTimerActive()
    const isFirstItem = !firstSynthMarked

    // Fresh abort controller for this item
    _abortCtrl = new AbortController()
    const signal = _abortCtrl.signal

    try {
      // Mark TTS synthesis start (only for the first item in a round-trip)
      if (shouldMark && isFirstItem) {
        mark('tts_synth', 'start')
        firstSynthMarked = true
      }

      _log('debug', 'synth_start', { text: item.text.substring(0, 60) })

      // Fetch audio from TTS service
      const response = await fetch(`${backendUrl}/api/tts/synthesize`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: item.text }),
        signal,
      })

      if (!response.ok) {
        const errBody = await response.text().catch(() => '')
        _log('error', 'synth_failed', { status: response.status, body: errBody.substring(0, 200) })
        throw new Error(`TTS synthesis failed: ${response.status} ${errBody}`)
      }

      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')

      const audioBlob = await response.blob()

      if (shouldMark && isFirstItem) mark('tts_synth', 'end')
      _log('debug', 'synth_complete', { blobSize: audioBlob.size })

      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')

      // ── HTMLAudioElement playback via pre-created reusable element ──
      const blobUrl = URL.createObjectURL(audioBlob)

      // Mark TTS playback start on first item
      if (shouldMark && isFirstItem) mark('tts_play', 'start')

      await new Promise<void>((resolve) => {
        const audio = getTTSAudioEl()

        // Listen for abort to stop playback mid-stream
        const cleanup = () => {
          signal.removeEventListener('abort', onAbort)
          audio.onended = null
          audio.onerror = null
          URL.revokeObjectURL(blobUrl)
        }

        const onAbort = () => {
          audio.pause()
          // Reset src to empty data URI (not '') to keep the element in a
          // "has played" state for iOS autoplay purposes.
          audio.removeAttribute('src')
          audio.load()
          cleanup()
          resolve()
        }
        signal.addEventListener('abort', onAbort, { once: true })

        audio.onended = () => {
          cleanup()
          resolve()
        }

        audio.onerror = () => {
          cleanup()
          _log('error', 'playback_error', { error: 'HTMLAudioElement error' })
          resolve() // resolve (not reject) to advance the queue
        }

        audio.src = blobUrl
        audio.play()
          .then(() => {
            _log('debug', 'playback_started', { duration: audio.duration, blobSize: audioBlob.size })
          })
          .catch((err) => {
            cleanup()
            _log('error', 'playback_error', { error: err instanceof Error ? err.message : String(err) })
            resolve() // resolve to advance the queue
          })
      })
    } catch (err) {
      // Silently ignore abort errors — they're expected from stop()
      if (err instanceof DOMException && err.name === 'AbortError') {
        _log('debug', 'playback_aborted')
      } else {
        _log('error', 'playback_error', { error: err instanceof Error ? err.message : String(err) })
        // Mark synth end on error if it was the first item and synth start was marked
        if (shouldMark && isFirstItem && firstSynthMarked) {
          mark('tts_synth', 'end')
        }
      }
    } finally {
      // Remove processed item
      queue.value.shift()
      isPlaying.value = false

      // Mark TTS playback end when queue is fully drained
      if (queue.value.length === 0) {
        _log('info', 'queue_drained')
        if (shouldMark) mark('tts_play', 'end')
        // Reset first-item flag for next round-trip
        firstSynthMarked = false

        // Notify listener that the queue has fully drained
        _onQueueDrained?.()
      }

      // Process next item if available
      if (queue.value.length > 0) {
        processQueue()
      }
    }
  }

  // Add text to the TTS queue
  function enqueue(text: string) {
    _log('debug', 'enqueue', { text: text.substring(0, 80), queueLength: queue.value.length })
    queue.value.push({ text })
    processQueue()
  }

  // Register a callback fired when the queue drains after processing.
  function onQueueDrained(cb: () => void) {
    _onQueueDrained = cb
  }

  // Stop current playback and clear queue
  function stop() {
    _log('info', 'stop', { queueLength: queue.value.length })

    // Abort any in-flight fetch or playback
    if (_abortCtrl) {
      _abortCtrl.abort()
      _abortCtrl = null
    }

    // Stop currently playing audio element
    if (_ttsAudioEl) {
      _ttsAudioEl.pause()
      _ttsAudioEl.removeAttribute('src')
      _ttsAudioEl.load()
    }

    queue.value = []
    isPlaying.value = false
    firstSynthMarked = false
    // NOTE: _onQueueDrained is intentionally preserved — it is registered
    // once during useAgent() setup and must survive stop/abort cycles.
    // The guards in the callback (agentDone, voiceEnabled, etc.) already
    // prevent premature turn completion.
  }

  return {
    queue: readonly(queue),
    isPlaying: readonly(isPlaying),
    enqueue,
    onQueueDrained,
    stop,
  }
}
