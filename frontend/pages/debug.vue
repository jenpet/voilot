<template>
  <div class="min-h-screen bg-gray-900 text-gray-100 p-6 font-mono text-sm">
    <h1 class="text-xl font-bold mb-4">Voilot Browser Diagnostics</h1>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">Environment</h2>
      <ul class="space-y-1">
        <li><span class="text-gray-400">User Agent:</span> {{ userAgent }}</li>
        <li><span class="text-gray-400">Protocol:</span> {{ protocol }}</li>
        <li><span class="text-gray-400">Host:</span> {{ host }}</li>
        <li>
          <span class="text-gray-400">Secure Context:</span>
          <span :class="isSecure ? 'text-green-400' : 'text-red-400'">{{ isSecure }}</span>
        </li>
      </ul>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">Media APIs</h2>
      <ul class="space-y-1">
        <li>
          <span class="text-gray-400">navigator.mediaDevices:</span>
          <span :class="hasMediaDevices ? 'text-green-400' : 'text-red-400'">{{ hasMediaDevices }}</span>
        </li>
        <li>
          <span class="text-gray-400">getUserMedia:</span>
          <span :class="hasGetUserMedia ? 'text-green-400' : 'text-red-400'">{{ hasGetUserMedia }}</span>
        </li>
        <li>
          <span class="text-gray-400">MediaRecorder:</span>
          <span :class="hasMediaRecorder ? 'text-green-400' : 'text-red-400'">{{ hasMediaRecorder }}</span>
        </li>
        <li>
          <span class="text-gray-400">AudioContext:</span>
          <span :class="hasAudioContext ? 'text-green-400' : 'text-red-400'">{{ hasAudioContext }}</span>
        </li>
      </ul>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">MIME Type Support</h2>
      <ul class="space-y-1">
        <li v-for="m in mimeResults" :key="m.type">
          <span class="text-gray-400">{{ m.type }}:</span>
          <span :class="m.supported ? 'text-green-400' : 'text-red-400'">{{ m.supported }}</span>
        </li>
      </ul>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">Mic Access Test (Vue @click)</h2>
      <button
        class="px-4 py-2 bg-blue-600 rounded hover:bg-blue-500 active:bg-blue-700 text-white mb-3"
        @click="testMic"
        :disabled="micTesting"
      >
        {{ micTesting ? 'Testing...' : 'Test getUserMedia' }}
      </button>
      <div v-if="micResult !== null" class="mt-2">
        <p :class="micResult.ok ? 'text-green-400' : 'text-red-400'">
          {{ micResult.ok ? 'SUCCESS' : 'FAILED' }}
        </p>
        <p class="text-gray-300 mt-1">{{ micResult.detail }}</p>
        <p v-if="micResult.tracks" class="text-gray-400 mt-1">Tracks: {{ micResult.tracks }}</p>
      </div>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">Mic Access Test (Raw onclick)</h2>
      <p class="text-gray-400 text-xs mb-2">Bypasses Vue event system — raw DOM onclick handler</p>
      <button
        ref="rawMicBtn"
        class="px-4 py-2 bg-green-600 rounded hover:bg-green-500 active:bg-green-700 text-white mb-3"
      >
        Test getUserMedia (raw)
      </button>
      <div id="raw-mic-result" class="mt-2"></div>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">User Activation Check</h2>
      <p class="text-gray-400 text-xs mb-2">Checks navigator.userActivation state on tap</p>
      <button
        ref="activationBtn"
        class="px-4 py-2 bg-yellow-600 rounded hover:bg-yellow-500 active:bg-yellow-700 text-white mb-3"
      >
        Check Activation
      </button>
      <div id="activation-result" class="mt-2"></div>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">Recording Test</h2>
      <button
        class="px-4 py-2 bg-purple-600 rounded hover:bg-purple-500 active:bg-purple-700 text-white mb-3"
        @click="testRecording"
        :disabled="recordingTesting"
      >
        {{ recordingTesting ? 'Recording 2s...' : 'Test Record + STT' }}
      </button>
      <div v-if="recordResult !== null" class="mt-2">
        <p :class="recordResult.ok ? 'text-green-400' : 'text-red-400'">
          {{ recordResult.ok ? 'SUCCESS' : 'FAILED' }}
        </p>
        <p class="text-gray-300 mt-1">{{ recordResult.detail }}</p>
      </div>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">Audio Output Test</h2>
      <p class="text-gray-400 text-xs mb-2">Plays a 440Hz beep for 0.5s via AudioContext (no TTS needed)</p>
      <button
        class="px-4 py-2 bg-teal-600 rounded hover:bg-teal-500 active:bg-teal-700 text-white mb-3"
        @click="testBeep"
      >
        Play Beep (440Hz)
      </button>
      <div v-if="beepResult" class="mt-2">
        <pre class="text-gray-300 text-xs whitespace-pre-wrap">{{ beepResult }}</pre>
      </div>
    </section>

    <section class="mb-6">
      <h2 class="text-lg font-semibold mb-2 text-blue-400">TTS Playback Test</h2>
      <p class="text-gray-400 text-xs mb-2">Tests AudioContext + TTS synthesis + Web Audio API playback</p>
      <button
        class="px-4 py-2 bg-orange-600 rounded hover:bg-orange-500 active:bg-orange-700 text-white mb-3"
        @click="testTTS"
        :disabled="ttsTesting"
      >
        {{ ttsTesting ? 'Playing...' : 'Test TTS Playback' }}
      </button>
      <div v-if="ttsResult !== null" class="mt-2">
        <p :class="ttsResult.ok ? 'text-green-400' : 'text-red-400'">
          {{ ttsResult.ok ? 'SUCCESS' : 'FAILED' }}
        </p>
        <pre class="text-gray-300 mt-1 text-xs whitespace-pre-wrap">{{ ttsResult.detail }}</pre>
      </div>
    </section>

    <NuxtLink to="/" class="text-blue-400 underline">Back to sessions</NuxtLink>
  </div>
</template>

<script setup lang="ts">
import { unlockAudio } from '~/composables/useTTS'

const userAgent = ref('')
const protocol = ref('')
const host = ref('')
const isSecure = ref(false)
const hasMediaDevices = ref(false)
const hasGetUserMedia = ref(false)
const hasMediaRecorder = ref(false)
const hasAudioContext = ref(false)

const mimeTypes = [
  'audio/webm;codecs=opus',
  'audio/webm',
  'audio/mp4',
  'audio/aac',
  'audio/ogg;codecs=opus',
  'audio/wav',
]
const mimeResults = ref<{ type: string; supported: boolean }[]>([])

const micTesting = ref(false)
const micResult = ref<{ ok: boolean; detail: string; tracks?: string } | null>(null)

const recordingTesting = ref(false)
const recordResult = ref<{ ok: boolean; detail: string } | null>(null)

const ttsTesting = ref(false)
const ttsResult = ref<{ ok: boolean; detail: string } | null>(null)
const beepResult = ref('')

const rawMicBtn = ref<HTMLButtonElement | null>(null)
const activationBtn = ref<HTMLButtonElement | null>(null)
const config = useRuntimeConfig()

onMounted(() => {
  userAgent.value = navigator.userAgent
  protocol.value = window.location.protocol
  host.value = window.location.host
  isSecure.value = window.isSecureContext

  hasMediaDevices.value = !!navigator.mediaDevices
  hasGetUserMedia.value = !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia)
  hasMediaRecorder.value = typeof MediaRecorder !== 'undefined'
  hasAudioContext.value = !!(window.AudioContext || (window as any).webkitAudioContext)

  mimeResults.value = mimeTypes.map(type => ({
    type,
    supported: hasMediaRecorder.value ? MediaRecorder.isTypeSupported(type) : false,
  }))

  // Raw onclick handler — bypasses Vue event system entirely
  if (rawMicBtn.value) {
    rawMicBtn.value.onclick = function () {
      const resultEl = document.getElementById('raw-mic-result')!
      const ua = (navigator as any).userActivation
      const activationInfo = ua
        ? `isActive=${ua.isActive}, hasBeenActive=${ua.hasBeenActive}`
        : 'userActivation API not available'

      resultEl.innerHTML = `<p class="text-gray-400">Activation: ${activationInfo}</p><p class="text-gray-400">Calling getUserMedia...</p>`

      // Call getUserMedia IMMEDIATELY — no awaits, no state updates before it
      navigator.mediaDevices.getUserMedia({ audio: true })
        .then(function (stream) {
          const tracks = stream.getAudioTracks()
          const info = tracks.map(function (t) { return t.label + ' (' + t.readyState + ')' }).join(', ')
          resultEl.innerHTML = `<p class="text-green-400">SUCCESS</p><p class="text-gray-300">Tracks: ${info}</p><p class="text-gray-400">Activation: ${activationInfo}</p>`
          tracks.forEach(function (t) { t.stop() })
        })
        .catch(function (err) {
          const name = err instanceof Error ? err.name : 'Unknown'
          const msg = err instanceof Error ? err.message : String(err)
          resultEl.innerHTML = `<p class="text-red-400">FAILED</p><p class="text-gray-300">${name}: ${msg}</p><p class="text-gray-400">Activation: ${activationInfo}</p>`
        })
    }
  }

  // User activation check button
  if (activationBtn.value) {
    activationBtn.value.onclick = function () {
      const resultEl = document.getElementById('activation-result')!
      const ua = (navigator as any).userActivation
      if (ua) {
        resultEl.innerHTML = `<p class="text-gray-300">isActive: <span class="${ua.isActive ? 'text-green-400' : 'text-red-400'}">${ua.isActive}</span></p><p class="text-gray-300">hasBeenActive: <span class="${ua.hasBeenActive ? 'text-green-400' : 'text-red-400'}">${ua.hasBeenActive}</span></p>`
      } else {
        resultEl.innerHTML = `<p class="text-yellow-400">navigator.userActivation not available in this browser</p>`
      }
    }
  }
})

async function testMic() {
  micTesting.value = true
  micResult.value = null
  try {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true },
    })
    const tracks = stream.getAudioTracks()
    const trackInfo = tracks.map(t => `${t.label} (${t.readyState})`).join(', ')
    micResult.value = { ok: true, detail: 'getUserMedia succeeded', tracks: trackInfo }
    // Clean up
    tracks.forEach(t => t.stop())
  } catch (err) {
    const name = err instanceof Error ? err.name : 'Unknown'
    const msg = err instanceof Error ? err.message : String(err)
    micResult.value = { ok: false, detail: `${name}: ${msg}` }
  }
  micTesting.value = false
}

async function testRecording() {
  recordingTesting.value = true
  recordResult.value = null
  try {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true },
    })

    // Pick MIME type
    const candidates = ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4', 'audio/aac']
    let mimeType: string | undefined
    for (const c of candidates) {
      if (MediaRecorder.isTypeSupported(c)) { mimeType = c; break }
    }

    const opts: MediaRecorderOptions = {}
    if (mimeType) opts.mimeType = mimeType

    const recorder = new MediaRecorder(stream, opts)
    const chunks: Blob[] = []

    recorder.ondataavailable = (e) => {
      if (e.data.size > 0) chunks.push(e.data)
    }

    recorder.start(100)

    // Record for 2 seconds
    await new Promise(resolve => setTimeout(resolve, 2000))

    const blob = await new Promise<Blob>((resolve) => {
      recorder.onstop = () => {
        resolve(new Blob(chunks, { type: recorder.mimeType }))
      }
      recorder.stop()
    })

    stream.getAudioTracks().forEach(t => t.stop())

    let detail = `Recorded ${blob.size} bytes as ${mimeType || 'default'}`

    // Try sending to STT
    try {
      const result = await $fetch<{ text: string }>(
        `${config.public.backendUrl}/api/stt/transcribe`,
        {
          method: 'POST',
          headers: { 'Content-Type': mimeType || 'application/octet-stream' },
          body: blob,
        },
      )
      detail += ` | STT: "${result.text}"`
      recordResult.value = { ok: true, detail }
    } catch (sttErr) {
      const msg = sttErr instanceof Error ? sttErr.message : String(sttErr)
      detail += ` | STT failed: ${msg}`
      recordResult.value = { ok: false, detail }
    }
  } catch (err) {
    const name = err instanceof Error ? err.name : 'Unknown'
    const msg = err instanceof Error ? err.message : String(err)
    recordResult.value = { ok: false, detail: `${name}: ${msg}` }
  }
  recordingTesting.value = false
}

function testBeep() {
  // Call unlockAudio first to override mute switch
  unlockAudio()

  const log: string[] = []
  try {
    const ctx = new (window.AudioContext || (window as any).webkitAudioContext)()
    log.push(`AudioContext: state=${ctx.state}, sampleRate=${ctx.sampleRate}`)

    if (ctx.state === 'suspended') {
      ctx.resume().then(() => {
        log.push(`Resumed: state=${ctx.state}`)
        beepResult.value = log.join('\n')
      }).catch(() => {})
    }

    // Create a 440Hz sine wave oscillator
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.type = 'sine'
    osc.frequency.value = 440
    gain.gain.value = 0.3
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.start()
    osc.stop(ctx.currentTime + 0.5)
    osc.onended = () => {
      log.push('Beep finished!')
      beepResult.value = log.join('\n')
      ctx.close()
    }
    log.push('Oscillator started (0.5s, 440Hz)')
    beepResult.value = log.join('\n')
  } catch (err) {
    log.push(`Error: ${err}`)
    beepResult.value = log.join('\n')
  }
}

async function testTTS() {
  ttsTesting.value = true
  ttsResult.value = null
  const log: string[] = []

  try {
    // Step 1: Unlock audio (must happen in user gesture — we're in a click handler)
    unlockAudio()
    const ctx = new (window.AudioContext || (window as any).webkitAudioContext)()
    log.push(`AudioContext created: state=${ctx.state}, sampleRate=${ctx.sampleRate}`)

    if (ctx.state === 'suspended') {
      await ctx.resume()
      log.push(`AudioContext resumed: state=${ctx.state}`)
    }

    // Step 2: Fetch TTS audio from backend
    log.push('Fetching TTS audio...')
    ttsResult.value = { ok: true, detail: log.join('\n') }

    const response = await fetch(`${config.public.backendUrl}/api/tts/synthesize`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: 'Hello! This is a test of the text to speech system.' }),
    })

    log.push(`Response: status=${response.status}, Content-Type=${response.headers.get('Content-Type')}`)

    if (!response.ok) {
      const errBody = await response.text().catch(() => '')
      throw new Error(`TTS request failed: ${response.status} ${errBody}`)
    }

    const blob = await response.blob()
    log.push(`Blob: size=${blob.size}, type=${blob.type}`)
    ttsResult.value = { ok: true, detail: log.join('\n') }

    // Step 3: Decode audio with Web Audio API
    const arrayBuffer = await blob.arrayBuffer()
    log.push(`ArrayBuffer: ${arrayBuffer.byteLength} bytes`)

    const audioBuffer = await ctx.decodeAudioData(arrayBuffer)
    log.push(`Decoded: duration=${audioBuffer.duration.toFixed(2)}s, channels=${audioBuffer.numberOfChannels}, sampleRate=${audioBuffer.sampleRate}`)
    ttsResult.value = { ok: true, detail: log.join('\n') }

    // Step 4: Play audio
    log.push('Playing audio...')
    ttsResult.value = { ok: true, detail: log.join('\n') }

    await new Promise<void>((resolve, reject) => {
      const source = ctx.createBufferSource()
      source.buffer = audioBuffer
      source.connect(ctx.destination)
      source.onended = () => {
        log.push('Playback completed!')
        resolve()
      }
      try {
        source.start(0)
        log.push('source.start(0) called successfully')
      } catch (err) {
        log.push(`source.start() error: ${err}`)
        reject(err)
      }
    })

    ttsResult.value = { ok: true, detail: log.join('\n') }
    ctx.close()
  } catch (err) {
    const name = err instanceof Error ? err.name : 'Unknown'
    const msg = err instanceof Error ? err.message : String(err)
    log.push(`ERROR: ${name}: ${msg}`)
    ttsResult.value = { ok: false, detail: log.join('\n') }
  }
  ttsTesting.value = false
}
</script>
