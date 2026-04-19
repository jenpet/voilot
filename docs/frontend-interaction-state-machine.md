# Interaction State Reference

This document describes the voilot interaction state machine, its action-gated dispatch model, component registry, and debug logging system.

## Architecture: Action-Gated Dispatch

The state machine tracks **turn lifecycle** only — 4 states. Audio/media concerns (mic, TTS, STT) are tracked by reactive booleans in their respective composables (`useVoice`, `useTTS`), not as state machine states.

State transitions are driven by **named actions** via `dispatch(action, trigger) → boolean`. The state machine checks whether the action is valid in the current state and transitions if so. Side effects remain in the calling composables — the state machine is a pure gate with no dependencies.

- **`dispatch(action, trigger)`** — returns `true` if the action was accepted (state transitions), `false` if rejected (state unchanged, warning logged).
- **`abort(trigger)`** — force-resets to `idle` from any state. Bypasses the action table. Used for user abort/cleanup.
- **`getState()`** — returns the current state value synchronously.

Implementation: `frontend/composables/useStateMachine.ts`

## Interaction States (4)

| State | Description |
|---|---|
| `idle` | Nothing active. Waiting for user input (mic tap, typed message, or voice loop restart). |
| `user_turn` | User is providing input — mic recording, STT transcribing, or typing. |
| `agent_turn` | Agent is processing — streaming response, TTS playing. Ends when both agent streaming and TTS playback are complete. |
| `error` | Something failed (mic denied, STT error, WS disconnect, etc.). Recoverable via `recover` action. |

## Action Table (5 actions)

| Action | From states | To state | What triggers it |
|---|---|---|---|
| `start_user_turn` | `idle` | `user_turn` | User taps mic button; voice loop auto-restart |
| `start_agent_turn` | `user_turn`, `idle` | `agent_turn` | STT result sent to agent; typed message sent; session busy on page load |
| `complete_turn` | `agent_turn` | `idle` | Agent done streaming AND TTS queue drained (or no TTS) |
| `error` | any except `idle` | `error` | Mic denied; STT failed; WS send failed |
| `recover` | `error` | `idle` | User dismisses error; automatic retry path |

**`abort`** (special case): any state → `idle`. Bypasses the action table entirely.

## State Transition Diagram

```
              start_user_turn              start_agent_turn
    idle ──────────────────► user_turn ──────────────────► agent_turn
      ▲                                                       │
      │                    start_agent_turn                    │
      ├◄──────────────────────────────────────────────────────┘
      │                   (idle → agent_turn for busy-on-load) complete_turn
      │
      │  recover
    error ◄──── any state (except idle) via error action

    Any state ──► idle via abort()
```

### Key Design Decisions

1. **`start_agent_turn` from `idle`**: Allows text-only messages (no mic/user_turn phase) and busy-on-load recovery (page reload while agent is processing).
2. **Turn completion is dual-gated**: `tryCompleteTurn()` in `useAgent.ts` only dispatches `complete_turn` when both `agentDone` flag is true AND `onQueueDrained` callback fires. This prevents premature turn completion.
3. **Voice loop restart**: After `complete_turn` returns to `idle`, `useAgent.ts` calls `startLoopRecording()` which dispatches `start_user_turn` — re-entering the cycle.

## Audio/Media State (Orthogonal)

These are **not** state machine states. They are reactive booleans in their respective composables:

| Signal | Composable | Description |
|---|---|---|
| `isRecording` | `useVoice` | MediaRecorder actively capturing audio |
| `isTTSPlaying` | `useTTS` | TTS audio currently playing |
| `voiceEnabled` | `useVoice` | Mic stream is open and available |
| `isStreaming` | `useAgent` | Agent is sending response events |

This separation enables future **voice barge-in** (mic can be open during agent_turn) and eliminates the class of bugs caused by conflating turn lifecycle with audio state.

## Component Registry

Debug log entries include a `component` field identifying the source subsystem:

| Component | File(s) | Description |
|---|---|---|
| `mic` | `useVoice.ts` | Mic stream acquisition, release, AudioContext lifecycle |
| `stt` | `useVoice.ts` | Speech-to-text requests and responses |
| `agent` | `useAgent.ts` | Agent message handling, event processing, send/abort |
| `question` | `useAgent.ts` | Question request/response handling |
| `permission` | `useAgent.ts` | Permission request/response handling |
| `tts` | `useTTS.ts` | TTS synthesis, playback, queue management |
| `tts-pipeline` | `useTTSChunker.ts`, `useTTSToolBatcher.ts` | Text chunking and tool batching for TTS |
| `ws` | `useWebSocket.ts`, `useAgent.ts` | WebSocket connection lifecycle |
| `ui` | `VoiceButton.vue` | User interface interactions |
| `audio-feedback` | `useAudioFeedback.ts`, `useRecordingFeedback.ts` | Audio cues (blips, chimes, hum) |
| `loop` | `useAgent.ts` | Voice conversation loop control |
| `state` | `useStateMachine.ts` | Action dispatch, transitions, rejections |

## Debug Log Entry Format

```json
{
  "timestamp": 1713340800000,
  "elapsed": 5432,
  "state": "agent_turn",
  "level": "info",
  "component": "state",
  "event": "action_dispatched",
  "data": { "action": "complete_turn", "from": "agent_turn", "to": "idle", "trigger": "turn_complete" }
}
```

### State Machine Log Events

| Event | Level | Description |
|---|---|---|
| `action_dispatched` | info | Action accepted, state transitioned |
| `action_rejected` | warn | Action denied (current state not in `from` list) |
| `abort` | info | Force-reset to idle via `abort()` |

### Fields

| Field | Type | Description |
|---|---|---|
| `timestamp` | number | Unix epoch ms |
| `elapsed` | number | Ms since debug recording started |
| `state` | string | Current interaction state at log time |
| `level` | `debug` \| `info` \| `warn` \| `error` | Severity |
| `component` | string | Source component (see registry above) |
| `event` | string | Event identifier (e.g., `action_dispatched`, `enqueue`, `mic_acquired`) |
| `data` | object? | Optional structured metadata |

### Log Levels

- **debug** — Verbose operational data: RMS samples, individual deltas, audio feedback cues
- **info** — Significant lifecycle events: state transitions, message send/receive, loop start/stop
- **warn** — Recoverable issues: rejected actions, empty STT results, connection drops
- **error** — Failures: mic denied, STT errors, TTS errors, send failures

## Common Failure Patterns

### Voice loop not restarting

Look for:
- `loop` / `loop_started` — should fire after TTS finishes and agent is done
- `state` / `action_dispatched` with `action: complete_turn` — confirms turn ended
- `tts` / `queue_drained` — confirms TTS finished all items
- `agent` / `finish_streaming` — confirms agent done streaming
- Check that `agentDone` flag and `onQueueDrained` callback both fired — turn completion requires both

### Mic not picking up sound

Look for:
- `mic` / `stream_acquired` — confirms mic was opened
- `mic` / `rms_sample` — should appear at ~2/sec while mic is open; values near 0 indicate silence
- `mic` / `speech_detected` — should fire when user starts speaking
- Missing `rms_sample` entries → mic stream may have been released or never acquired

### Silent TTS (agent responds but nothing plays)

Look for:
- `tts` / `enqueue` — text was queued for synthesis
- `tts` / `synth_start` → `synth_complete` — HTTP request to TTS service succeeded
- `tts` / `playback_started` — AudioBufferSourceNode.start() was called
- `tts` / `playback_error` — decode or playback failure
- Missing `enqueue` entries → text may be filtered out by condenser/chunker

### WebSocket disconnects

Look for:
- `ws` / `disconnected` — connection dropped
- `ws` / `reconnect_scheduled` — reconnect timer started
- `ws` / `connected` — reconnection succeeded
- `agent` / `send_message_failed` — message couldn't be sent (ws down)

## Debug Log Download

The settings panel (gear icon in session header) provides:

1. **Toggle** — enable/disable debug log recording
2. **Recording since** — timestamp when logging started
3. **Entry count** — number of buffered entries (max 5000, ring buffer)
4. **Download** — produces `voilot-debug-YYYY-MM-DD-HHmmss.zip` containing:
   - `debug-log.json` — all log entries with metadata
   - `session.json` — browser environment info (userAgent, viewport, connection type)

The zip is created using the native `CompressionStream` API (DEFLATE). Falls back to plain JSON if `CompressionStream` is unavailable.
