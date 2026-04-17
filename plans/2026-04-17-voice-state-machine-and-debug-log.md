# Plan: Full Interaction State Machine and Structured Debug Log

**Status:** ready
**Created:** 2026-04-17
**Author:** jenpet + planitect

## Goal
Replace the current implicit state management across the entire voice interaction lifecycle with an explicit state machine covering the full round-trip: from mic open through STT, agent processing, TTS playback, and back to listening. Introduce a structured, downloadable debug log that captures everything happening across this lifecycle. This makes it possible to diagnose issues anywhere in the flow by reviewing a detailed protocol of what actually happened.

## Context
The current interaction flow spans multiple composables (`useVoice`, `useAgent`, `useTTS`, `useWebSocket`, plus chunker/condenser/filter/batcher helpers) with state scattered across dozens of boolean flags and refs:

- **useVoice**: `isRecording`, `isMonitoring`, `audioLevel`, `lastError`, `_speechDetectedInRecording`
- **useAgent**: `isStreaming`, `voiceInitiatedTurn`, `abortedTurn`, `hasPendingPermission`, `hasPendingQuestion`, `loopRecordingActive`
- **VoiceButton.vue**: `isProcessing`, `manualRecordingActive`
- **useTTS**: `isPlaying`, queue state
- **useWebSocket**: `connectionState`

Transitions between these are implicit and spread across watchers, callbacks, and event handlers. There is no single place to see "what phase is the interaction in right now?" Logging is minimal -- only a handful of `console.error` calls in catch blocks.

Common issues that are hard to diagnose today:
- Mic doesn't pick up speech but appears active
- Mic silently switches off without user action
- Agent response arrives but TTS doesn't play
- Voice loop doesn't restart after TTS finishes
- Question/permission flow interrupts the conversation unexpectedly
- Silence detection triggers too early or too late

## Approach

### 1. Define the Full Interaction State Machine

A single state machine that tracks where we are in one conversational turn. This is the top-level state -- subsystems (mic, TTS, WebSocket) can have their own internal states, but this machine represents the user-visible interaction phase.

**States:**
| State | Description |
|-------|-------------|
| `idle` | Nothing happening. Waiting for user to initiate. |
| `mic:acquiring` | Requesting mic permissions / `getUserMedia` in progress |
| `mic:monitoring` | Mic open, passively listening for speech (interrupt detection) |
| `mic:recording` | Actively recording audio for transcription |
| `stt:transcribing` | Recording stopped, audio sent to STT backend, awaiting result |
| `agent:submitting` | Transcription received, message being sent over WebSocket |
| `agent:streaming` | Agent is producing response (text deltas, tool use, etc.) |
| `agent:awaiting-question` | Agent asked a question with options, waiting for user answer |
| `agent:awaiting-permission` | Agent requested permission, waiting for user approval/rejection |
| `tts:speaking` | TTS is synthesizing and playing back the agent response |
| `turn:completing` | Agent done, TTS draining, preparing for next turn |
| `error` | Something failed at any stage (with `errorSource` metadata) |

**Allowed transitions:**
```
idle -> mic:acquiring
mic:acquiring -> mic:monitoring | mic:recording | error | idle
mic:monitoring -> mic:recording | idle | error
mic:recording -> stt:transcribing | idle | error
stt:transcribing -> agent:submitting | idle | error
agent:submitting -> agent:streaming | error
agent:streaming -> agent:awaiting-question | agent:awaiting-permission | tts:speaking | turn:completing | error
agent:awaiting-question -> agent:streaming | idle | error
agent:awaiting-permission -> agent:streaming | idle | error
tts:speaking -> turn:completing | mic:recording (interrupt) | idle (abort) | error
turn:completing -> mic:acquiring (voice loop) | idle
error -> idle | mic:acquiring
```

**Concurrent states note:** In practice, agent streaming and TTS speaking overlap (TTS starts before the agent finishes). The state machine should represent the *primary phase* -- the thing the user is waiting on. When agent is still streaming but TTS has started, we stay in `agent:streaming`. We move to `tts:speaking` only once the agent is done and TTS is draining. The debug log captures the fine-grained concurrent events regardless.

**Implementation notes:**
- New composable `useInteractionState.ts` owns the state machine
- Single reactive `interactionState` ref with transition function
- Transition function validates allowed transitions and logs every change to the debug log
- Invalid transitions are rejected and logged as warnings (never silently swallowed)
- Existing boolean flags (`isRecording`, `isStreaming`, `isPlaying`, etc.) become computed getters derived from the state machine for backward compatibility
- Each state can carry metadata (e.g., `error` carries `errorSource` and `errorMessage`)
- `useAgent`, `useVoice`, `useTTS` call `transition()` at their key lifecycle points

### 2. Implement Structured Debug Log

A ring buffer logger that runs in the frontend, capturing timestamped events with rich context across all subsystems.

**Log entry structure:**
```ts
interface DebugLogEntry {
  timestamp: number       // Date.now()
  elapsed: number         // ms since logging started
  state: string           // current interaction state at time of logging (e.g. 'mic:recording')
  level: 'debug' | 'info' | 'warn' | 'error'
  component: string       // which component produced this entry (see components below)
  event: string           // e.g. 'state_transition', 'rms_sample', 'mic_acquired'
  data?: Record<string, unknown>  // structured context
}
```

**Ring buffer config:**
- Default capacity: 2 minutes worth of events
- Max entries: ~5000 (capped to prevent memory issues)
- Oldest entries are evicted when the buffer is full
- Tracks `recordingSince` timestamp (when logging was enabled)

**Components and what gets logged:**

| Component | Events |
|----------|--------|
| `state` | Every interaction state transition (from, to, trigger reason) |
| `mic` | `getUserMedia` calls, stream acquired, tracks ended, permissions, mic release |
| `audio` | RMS level samples (~2/sec at all times when mic is open), AudioContext state changes |
| `silence` | Silence detection start/threshold crossings, speech detected flag, auto-stop trigger with durations |
| `stt` | Transcription request sent, response received (with text length + confidence), errors |
| `ws` | WebSocket connection state changes, messages sent, reconnect attempts |
| `agent` | Message submitted, first token received (TTFT), tool_use/tool_result events, streaming done, errors |
| `question` | Question received (with options), answer submitted, rejection |
| `permission` | Permission requested (with title), response (once/always/reject) |
| `tts` | Chunk enqueued (with text), synthesis request/response, playback start/end, queue drain, abort |
| `tts-pipeline` | Chunker splits, condenser transforms, filter decisions, tool batcher accumulation/flush |
| `ui` | Button taps, abort triggered, mode switches, voice toggle, status text changes |
| `audio-feedback` | Audio feedback sounds played (handoff, success, error, question chime, etc.) |
| `loop` | Voice loop start/stop decisions, loop recording initiated, STT retry on failure |

### 3. Settings Panel UI and Download

In `SettingsPanel.vue`, add a **Debug Log** section with:
- A toggle to enable/disable debug logging (persisted via `useSettings`)
- A label showing the timestamp from when logging started (e.g. "Recording since 14:28:03")
- A download button right next to it that exports a **zip archive** containing two files

**Archive contents:**

1. `debug-log.json` -- the ring buffer with component events:
```json
{
  "exportedAt": "2026-04-17T14:30:00Z",
  "recordingSince": "2026-04-17T14:28:03Z",
  "userAgent": "...",
  "entryCount": 342,
  "entries": [ ... ]
}
```

2. `session.json` -- the conversation and session metadata from memory:
```json
{
  "sessionId": "abc-123",
  "agent": "opencode",
  "mode": "plan",
  "messageCount": 12,
  "messages": [ ... ]
}
```

**Download details:**
- Filename format: `voilot-debug-YYYY-MM-DD-HHmmss.zip`
- Use the browser's native `CompressionStream` API for zip creation (no external dependencies)
- Use `URL.createObjectURL` + anchor click pattern for the download
- When logging is toggled off, the buffer is cleared and the "recording since" timestamp resets

### 4. New Composables

| Composable | Responsibility |
|------------|---------------|
| `useInteractionState.ts` | State machine: states enum, reactive state ref, `transition()` function, metadata per state |
| `useDebugLog.ts` | Ring buffer logger: `log()` function, buffer management, export/download, enable/disable toggle, `recordingSince` tracking |

### 5. Integration Points

Files that need instrumentation with `useDebugLog` calls and `useInteractionState` transitions:

| File | Changes |
|------|---------|
| `useVoice.ts` | Transition calls for mic:acquiring/monitoring/recording/transcribing. Log mic lifecycle, RMS samples, silence detection. Remove standalone boolean state. |
| `useAgent.ts` | Transition calls for agent:submitting/streaming/awaiting-question/awaiting-permission/turn:completing. Log WS messages, tool events, question/permission flow. |
| `useTTS.ts` | Transition call for tts:speaking. Log queue operations, synthesis timing, playback events. |
| `useTTSChunker.ts` | Log chunk splits with text length. |
| `useTTSCondenser.ts` | Log condense transformations (before/after length). |
| `useTTSFilter.ts` | Log filter decisions (event type, speak/skip). |
| `useTTSToolBatcher.ts` | Log tool accumulation and flush events. |
| `useWebSocket.ts` | Log connection state changes, reconnect attempts. |
| `useAudioFeedback.ts` | Log which sound effect is played and why. |
| `VoiceButton.vue` | Log UI interactions. Remove local state (`isProcessing`, `manualRecordingActive`), derive from interaction state. |
| `SettingsPanel.vue` | Add debug log toggle, recording-since label, and download button. |

### 6. State-Component Reference Document

As a deliverable of this plan, create a persistent reference document at `frontend/docs/interaction-state-reference.md`. This document serves as a lookup for agents (or humans) analyzing debug logs. When a user uploads a debug log and describes a problem, the agent can consult this document to understand the rules and spot violations.

**The document must contain:**

#### a) Component Registry
A list of all components that participate in the interaction lifecycle, what they own, and what composable they map to:

| Component | Composable | Owns |
|-----------|-----------|------|
| `mic` | `useVoice.ts` | Mic stream, MediaRecorder, RMS analysis, silence detection |
| `stt` | `useVoice.ts` | STT HTTP request to backend |
| `agent` | `useAgent.ts` | WebSocket message submission, event handling, streaming |
| `question` | `useAgent.ts` | Question receipt, answer collection, response submission |
| `permission` | `useAgent.ts` | Permission receipt, approval/rejection |
| `tts` | `useTTS.ts` | Synthesis requests, audio playback queue |
| `tts-pipeline` | `useTTSChunker/Condenser/Filter/ToolBatcher` | Text processing before TTS |
| `ws` | `useWebSocket.ts` | WebSocket connection lifecycle |
| `ui` | `VoiceButton.vue`, `SettingsPanel.vue` | User interactions, button state |
| `audio-feedback` | `useAudioFeedback.ts` | Sound effects |
| `loop` | `useAgent.ts` | Voice loop auto-continue logic |
| `state` | `useInteractionState.ts` | State machine transitions |

#### b) State-Component Permission Matrix
A matrix showing what each component is allowed to do in each interaction state. Format:

| State | mic | stt | agent | tts | ui | ... |
|-------|-----|-----|-------|-----|----|-----|
| `idle` | may acquire | -- | -- | -- | may start recording | ... |
| `mic:recording` | must be active, logs RMS | -- | -- | -- | may stop recording | ... |
| `agent:streaming` | may monitor (for interrupt) | -- | receives events | may enqueue chunks | may abort | ... |
| ... | ... | ... | ... | ... | ... | ... |

Each cell states: `--` (inactive), `may` (allowed but optional), `must` (required), or `must not` (forbidden in this state).

#### c) Transition Rules
For each allowed transition, document:
- Which component triggers it
- What preconditions must hold
- What side effects occur (e.g., "entering `tts:speaking` starts queue drain")

#### d) Common Failure Patterns
A section listing known failure signatures in the debug log, e.g.:
- "RMS samples show values > 0 but `silence:auto_stop_triggered` fires anyway" = threshold too high
- "State stuck in `agent:streaming` with no `agent` events for > 10s" = WebSocket dropped without detection
- "No `loop:loop_started` after `tts:queue_drained`" = loop guard condition failed

This document should be kept in sync with the implementation. When the state machine or components change, the reference document must be updated.

## Open Questions
- None remaining.

## Acceptance Criteria
- Single `interactionState` ref tracks the full conversational turn lifecycle across all subsystems
- Invalid state transitions are rejected and logged as warnings
- Debug log captures events across all components: mic, STT, agent, TTS pipeline, questions, permissions, WebSocket, UI, audio feedback, and voice loop
- Every log entry includes the current interaction state
- RMS samples are logged at ~2/sec whenever the mic is open (monitoring or recording)
- User can enable/disable logging via the settings panel
- Settings panel shows "recording since" timestamp when logging is active
- User can download a zip archive containing the debug log and session data, using native `CompressionStream` API
- A state-component reference document exists at `frontend/docs/interaction-state-reference.md` for debug log analysis
- Existing functionality (record, auto-stop, monitor, interrupt, question/permission flow, TTS, voice loop, abort) works unchanged
