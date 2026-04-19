# Plan: Settings Panel + Pluggable Client-Side STT

## Goal
Add a settings infrastructure to voilot and implement three pluggable STT providers:
1. **Server** (existing faster-whisper via backend)
2. **Web Speech API** (iOS on-device Siri recognition)
3. **Whisper WASM** (Transformers.js running in a Web Worker)

UI: Slide-over settings panel accessible via gear icon on both pages.
Voice: Dual-path recording — always capture audio blob alongside Web Speech API for fallback.

## New Files

### 1. `composables/useSettings.ts`
Reactive localStorage-backed settings store with types:
```ts
type STTProvider = 'server' | 'web-speech-api' | 'whisper-wasm';
type WhisperModel = 'onnx-community/whisper-tiny.en' | 'onnx-community/whisper-base.en';
type WhisperDtype = 'q8' | 'fp16' | 'q4';

interface STTSettings {
  provider: STTProvider;
  whisperModel: WhisperModel;
  whisperDtype: WhisperDtype;
}

interface VoilotSettings {
  stt: STTSettings;
}
```
- Default: `{ stt: { provider: 'server', whisperModel: 'onnx-community/whisper-tiny.en', whisperDtype: 'q8' } }`
- Uses `useState` for singleton reactive state, `watch` deep to persist to `localStorage` key `voilot-settings`
- Exports `settingsPanelOpen` ref for controlling the slide-over visibility
- Functions: `updateSTT(partial)`, `resetToDefaults()`

### 2. `composables/useSTTProvider.ts`
STT provider abstraction that returns the active provider based on settings:
```ts
interface STTProvider {
  name: string;
  type: 'batch' | 'streaming';
  transcribe(audioBlob: Blob, mimeType: string): Promise<string | null>;  // batch providers
  startStreaming(onInterim: (text: string) => void, onFinal: (text: string) => void): void;  // streaming
  stopStreaming(): void;
  isReady: Ref<boolean>;
  isLoading: Ref<boolean>;
  loadProgress: Ref<number>;
  error: Ref<string | null>;
}
```
- Reads `useSettings().settings.value.stt.provider`
- Returns a computed provider that switches implementation based on the setting
- Handles lazy initialization of whisper-wasm provider

### 3. `composables/stt/server.ts`
Extract current server STT logic from `useVoice.ts` lines 384-401:
- `transcribe(audioBlob, mimeType)` → `POST ${backendUrl}/api/stt/transcribe` with Content-Type header
- `isReady` = always true (server is always available when backend is reachable)
- `type` = `'batch'`

### 4. `composables/stt/web-speech.ts`
Web Speech API provider:
- `type` = `'streaming'`
- Uses `webkitSpeechRecognition` (prefixed for Safari)
- `startStreaming(onInterim, onFinal)` creates a new recognition instance, starts it
- `stopStreaming()` calls `recognition.stop()`
- Handles events: `onresult` (interim + final), `onerror`, `onend`
- `isReady` = `typeof webkitSpeechRecognition !== 'undefined'`
- Config: `lang = 'en-US'`, `continuous = false`, `interimResults = true`

### 5. `composables/stt/whisper-wasm.ts`
Transformers.js Whisper provider:
- `type` = `'batch'`
- Creates a Web Worker (`workers/whisper-worker.ts`) on first use
- `transcribe(audioBlob, mimeType)`:
  1. Decode blob to 16kHz mono Float32Array via AudioContext.decodeAudioData()
  2. Transfer Float32Array to worker via postMessage (transferable)
  3. Worker runs pipeline, returns text
- `isReady` = model loaded in worker
- `isLoading` = model downloading
- `loadProgress` = 0-100 from worker progress events
- `error` = any load/inference error
- Exposes `loadModel()` for manual preloading from settings UI
- Reads model/dtype from settings

### 6. `workers/whisper-worker.ts`
Web Worker running Transformers.js pipeline:
```ts
import { pipeline, env } from '@huggingface/transformers';

let transcriber: any = null;

self.onmessage = async (e) => {
  if (e.data.type === 'load') {
    // Load model with progress reporting
    transcriber = await pipeline('automatic-speech-recognition', e.data.model, {
      dtype: e.data.dtype,
      progress_callback: (progress) => {
        self.postMessage({ type: 'progress', progress: progress.progress });
      },
    });
    self.postMessage({ type: 'ready' });
  } else if (e.data.type === 'transcribe') {
    const result = await transcriber(e.data.audio);
    self.postMessage({ type: 'result', text: result.text });
  }
};
```

### 7. `components/SettingsPanel.vue`
Slide-over panel from the right:
- Controlled by `useSettings().settingsPanelOpen`
- Backdrop overlay with click-to-close
- Sections:
  - **Speech Recognition**: Radio group for server / Web Speech API / On-Device Whisper
  - **Whisper Model** (shown when whisper-wasm selected): tiny.en (~41 MB) / base.en (~77 MB)
  - **Download Model** button with progress bar (when whisper-wasm selected)
  - Status indicator: Ready / Not Available / Downloading
- Tailwind dark theme matching existing UI (bg-surface-800, text-surface-100, etc.)
- Transition: slide from right with fade backdrop

## Modified Files

### 8. `composables/useVoice.ts`
Major refactor of `stopRecording()`:
- Import and use `useSTTProvider()` instead of hardcoded `$fetch`
- When provider.type === 'batch': send blob to `provider.transcribe()`
- When provider.type === 'streaming': Web Speech API results are collected in parallel
- Add `startStreamingRecognition()` / `stopStreamingRecognition()` for dual-path
- In `startRecording()`: if provider is streaming, also start Web Speech API
- In `stopRecording()`:
  - Stop MediaRecorder
  - If streaming provider, use streaming result if available
  - If streaming result is empty/failed, fall back to batch transcription of recorded blob via server

### 9. `app.vue`
Add `<SettingsPanel />` component at root level (after `<NuxtPage />`):
```vue
<template>
  <div class="min-h-screen bg-surface-900 text-surface-100">
    <NuxtPage />
    <SettingsPanel />
  </div>
</template>
```

### 10. `pages/index.vue`
Add gear icon button in the header between StatusIndicator and "+ New Session":
```vue
<button @click="settingsPanelOpen = true" class="p-1.5 rounded-lg hover:bg-surface-700">
  <!-- gear SVG icon -->
</button>
```

### 11. `pages/session/[id].vue`
Add gear icon in the header (next to Voice ON/OFF button):
```vue
<button @click="settingsPanelOpen = true" class="p-1.5 rounded-lg hover:bg-surface-700">
  <!-- gear SVG icon -->
</button>
```

### 12. `package.json`
Add dependency:
```
"@huggingface/transformers": "^4.0.0"
```

## Implementation Order

| Step | Description | Dependencies |
|------|-------------|-------------|
| 1 | `useSettings.ts` composable | — |
| 2 | `SettingsPanel.vue` component (STT section) | Step 1 |
| 3 | Wire panel into `app.vue` + gear icons in `index.vue`, `session/[id].vue` | Step 2 |
| 4 | `useSTTProvider.ts` + `stt/server.ts` (extract from useVoice) | Step 1 |
| 5 | Modify `useVoice.ts` to use `useSTTProvider` | Step 4 |
| 6 | `stt/web-speech.ts` provider | Step 4 |
| 7 | Dual-path in `useVoice.ts` (parallel recording + streaming) | Steps 5, 6 |
| 8 | `npm install @huggingface/transformers`, create `workers/whisper-worker.ts` | — |
| 9 | `stt/whisper-wasm.ts` provider with download progress | Steps 4, 8 |
| 10 | Wire download UI in settings panel (progress bar, status) | Steps 2, 9 |
| 11 | Build verification and test | All |

## Key Technical Details

### Dual-Path Recording (Step 7)
When Web Speech API is the selected provider:
1. `startRecording()` starts BOTH MediaRecorder and `webkitSpeechRecognition`
2. Web Speech API streams interim results (can be shown in UI)
3. On stop: prefer Web Speech API final result
4. If recognition returned empty/errored, fall back to sending recorded blob to server STT
5. Audio blob is always captured regardless of provider

### AudioContext 16kHz Decoding (Whisper WASM)
```ts
async function decodeToFloat32(blob: Blob): Promise<Float32Array> {
  const arrayBuffer = await blob.arrayBuffer();
  const audioCtx = new AudioContext({ sampleRate: 16000 });
  const audioBuffer = await audioCtx.decodeAudioData(arrayBuffer);
  const pcm = audioBuffer.getChannelData(0); // mono
  audioCtx.close();
  return pcm;
}
```
Note: Safari may not honor the `sampleRate` constructor option. If decoding returns a different sample rate, manual resampling is needed. Test on iOS.

### Web Worker Communication Protocol
```
Main → Worker:  { type: 'load', model: string, dtype: string }
Worker → Main:  { type: 'progress', progress: number }  // 0-100
Worker → Main:  { type: 'ready' }
Main → Worker:  { type: 'transcribe', audio: Float32Array }  // transferable
Worker → Main:  { type: 'result', text: string }
Worker → Main:  { type: 'error', message: string }
```

### Model Sizes
| Model | dtype | Download |
|-------|-------|----------|
| whisper-tiny.en | q8 | ~41 MB |
| whisper-tiny.en | fp16 | ~76 MB |
| whisper-base.en | q8 | ~77 MB |
| whisper-base.en | fp16 | ~146 MB |

Models are automatically cached by Transformers.js via the Cache API.

## Risks & Mitigations
1. **Web Speech API flaky on iOS** → Dual-path with server fallback
2. **Whisper WASM 3-5s latency** → Acceptable for voilot's use case; show spinner
3. **iOS cache eviction** → Settings panel shows model status; re-download button
4. **AudioContext sampleRate on Safari** → Test; add resampling fallback if needed
5. **Vite bundling of Web Worker** → Use `new Worker(new URL('./workers/whisper-worker.ts', import.meta.url), { type: 'module' })`
