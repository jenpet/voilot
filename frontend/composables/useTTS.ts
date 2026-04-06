interface TTSQueueItem {
  text: string
  // Web Audio API source node for the currently playing item
  sourceNode?: AudioBufferSourceNode
}

// ─── Persistent AudioContext for iOS Safari ──────────────────────
//
// iOS Safari requires that an AudioContext is created/resumed inside
// a user-gesture handler (tap/click).  Once resumed, it stays
// "running" and can play audio programmatically at any later time.
//
// We expose `unlockAudio()` which must be called synchronously
// inside the mic-button tap or the "Voice ON" toggle.  It creates
// (or resumes) a single global AudioContext and plays a tiny silent
// buffer so the OS audio session is fully activated.
//
// All subsequent TTS playback goes through this same AudioContext
// via `decodeAudioData()` + `AudioBufferSourceNode`, which is
// always allowed because the context is already in "running" state.

let _audioCtx: AudioContext | null = null

function getOrCreateAudioContext(): AudioContext {
  if (!_audioCtx) {
    _audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)()
  }
  return _audioCtx
}

// A tiny WAV file (44 bytes header + 2 bytes of audio = 1 sample at near-zero amplitude).
// This is NOT silent — it's a single sample at amplitude 1/32768 which is inaudible
// but enough to convince iOS that this page plays "media" audio, promoting the
// web audio session to "playback" category that overrides the mute/silent switch.
//
// Without this, AudioContext output is suppressed when the physical mute switch is on.
const TINY_WAV_B64 =
  'UklGRi4AAABXQVZFZm10IBAAAAABAAEARKwAAESsAAABAAgAZGF0YQoAAACA'

/**
 * Unlock iOS Safari's audio playback restriction AND override the mute switch.
 *
 * MUST be called synchronously inside a user-gesture handler
 * (tap / click) — e.g. the mic button or "Voice ON" toggle.
 *
 * Two things happen:
 * 1. The shared AudioContext is created/resumed and a silent buffer
 *    is played through it — this "unlocks" programmatic audio.
 * 2. An HTMLAudioElement plays a near-silent WAV at full volume —
 *    this promotes the page's audio session to "playback" mode,
 *    which overrides the iOS hardware mute/silent switch.
 *
 * This function is intentionally synchronous so it can be
 * called before the first `await` in a click handler without
 * breaking the user-activation chain.
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

  // 2. Play a near-silent WAV via HTMLAudioElement to promote to "playback" category.
  //    This overrides the iOS mute switch for all subsequent audio on this page.
  try {
    const audio = new Audio(`data:audio/wav;base64,${TINY_WAV_B64}`)
    audio.volume = 1.0 // must NOT be 0 — iOS ignores zero-volume for session promotion
    audio.play().catch(() => {})
  } catch {
    // best effort
  }

  console.log('[TTS] unlockAudio: ctx.state =', ctx.state)
}

export function useTTS() {
  const queue = useState<TTSQueueItem[]>('tts-queue', () => [])
  const isPlaying = ref(false)
  const backendUrl = resolveBackendUrl()
  const { mark, isActive: isTimerActive } = useRoundTripTimer()

  // Track whether the first TTS item in a round-trip has been marked
  let firstSynthMarked = false

  // Abort controller — cancelled by stop() to interrupt in-flight fetches
  // and pending decodes/playback.
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

      console.log('[TTS] Fetching audio for:', item.text.substring(0, 60) + '...')

      // Fetch audio from TTS service
      const response = await fetch(`${backendUrl}/api/tts/synthesize`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: item.text }),
        signal,
      })

      if (!response.ok) {
        const errBody = await response.text().catch(() => '')
        throw new Error(`TTS synthesis failed: ${response.status} ${errBody}`)
      }

      // Check abort after fetch completes (response may have arrived
      // just before abort was signalled)
      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')

      const contentType = response.headers.get('Content-Type') || 'unknown'
      console.log('[TTS] Got audio response, Content-Type:', contentType)

      const audioBlob = await response.blob()
      console.log('[TTS] Blob size:', audioBlob.size, 'type:', audioBlob.type)

      if (shouldMark && isFirstItem) mark('tts_synth', 'end')

      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')

      // ── Web Audio API playback (works on iOS Safari) ──
      const ctx = getOrCreateAudioContext()
      console.log('[TTS] AudioContext state:', ctx.state)

      // Ensure context is running
      if (ctx.state === 'suspended') {
        console.log('[TTS] Resuming suspended AudioContext...')
        await ctx.resume()
        console.log('[TTS] AudioContext state after resume:', ctx.state)
      }

      // Decode the audio data
      const arrayBuffer = await audioBlob.arrayBuffer()
      console.log('[TTS] ArrayBuffer size:', arrayBuffer.byteLength)

      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')

      const audioBuffer = await ctx.decodeAudioData(arrayBuffer)
      console.log('[TTS] Decoded audio buffer: duration=', audioBuffer.duration, 'sampleRate=', audioBuffer.sampleRate)

      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')

      // Mark TTS playback start on first item
      if (shouldMark && isFirstItem) mark('tts_play', 'start')

      // Play using AudioBufferSourceNode
      await new Promise<void>((resolve, reject) => {
        const source = ctx.createBufferSource()
        source.buffer = audioBuffer
        source.connect(ctx.destination)

        // Store source on queue item so stop() can cancel it
        item.sourceNode = source

        // Listen for abort to stop playback mid-stream
        const onAbort = () => {
          try { source.stop() } catch { /* may already be stopped */ }
          resolve()
        }
        signal.addEventListener('abort', onAbort, { once: true })

        source.onended = () => {
          signal.removeEventListener('abort', onAbort)
          console.log('[TTS] Playback ended for item')
          resolve()
        }

        try {
          source.start(0)
          console.log('[TTS] Playback started')
        } catch (err) {
          signal.removeEventListener('abort', onAbort)
          console.error('[TTS] source.start() failed:', err)
          reject(err)
        }
      })
    } catch (err) {
      // Silently ignore abort errors — they're expected from stop()
      if (err instanceof DOMException && err.name === 'AbortError') {
        console.log('[TTS] Playback aborted')
      } else {
        console.error('[TTS] Playback error:', err)
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
    console.log('[TTS] Enqueue:', text.substring(0, 80) + (text.length > 80 ? '...' : ''))
    queue.value.push({ text })
    processQueue()
  }

  // Stop current playback and clear queue
  function stop() {
    console.log('[TTS] Stop requested, queue length:', queue.value.length)

    // Abort any in-flight fetch, decode, or playback
    if (_abortCtrl) {
      _abortCtrl.abort()
      _abortCtrl = null
    }

    const current = queue.value[0]
    if (current?.sourceNode) {
      try {
        current.sourceNode.stop()
      } catch {
        // may already be stopped
      }
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
