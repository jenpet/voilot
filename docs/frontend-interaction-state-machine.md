# Interaction State Reference

This document describes the voilot interaction state machine, its action-gated dispatch model, component registry, and debug logging system.

## Architecture: Action-Gated Dispatch

State transitions are driven by **named actions** via `dispatch(action, trigger) → boolean`. The state machine checks whether the action is valid in the current state and transitions if so. Side effects (TTS playback, mic acquisition, audio feedback, etc.) remain in the calling composables — the state machine is a pure gate with no dependencies.

- **`dispatch(action, trigger)`** — returns `true` if the action was accepted (state transitions), `false` if rejected (state unchanged, warning logged).
- **`abort(trigger)`** — force-resets to `idle` from any state. Bypasses the action table. Used for user abort/cleanup.
- **`getState()`** — returns the current state value synchronously.

Implementation: `frontend/composables/useStateMachine.ts`

## Interaction States (12)

| State | Description |
|---|---|
| `idle` | Nothing active. Waiting for user input (mic tap, typed message, or voice loop restart). Resting state between turns. |
| `mic:acquiring` | Browser requesting mic access via `getUserMedia()`. May show permission prompt. Also entered when reusing a kept-open stream. |
| `mic:monitoring` | Mic open, AnalyserNode sampling RMS at ~2Hz, waiting for speech to cross threshold. For future hands-free activation. |
| `mic:recording` | MediaRecorder actively capturing audio. Stops on manual tap or silence auto-detection (~1.5s). |
| `stt:transcribing` | Audio blob sent to STT service. Waiting for transcription response. |
| `agent:submitting` | Message sent to agent via WebSocket. Waiting for first substantive event back. Typically very brief (~tens of ms). |
| `agent:streaming` | Agent generating response. Text deltas accumulated, run through TTS condenser/chunker, enqueued to TTS. UI renders in real time. |
| `agent:awaiting-question` | Agent asked a clarifying question. UI shows input. Voice loop restarts so user can answer by voice. |
| `agent:awaiting-permission` | Agent requested permission for destructive action. UI shows approve/deny. Voice loop does NOT restart (prevents accidental approval). |
| `tts:speaking` | TTS audio playing through AudioContext. Queue processes sequentially. Persists until all queued items finish. |
| `turn:completing` | Agent done streaming AND TTS finished/finishing. Cleanup state: flush chunker, reset batcher, stop hum, play chime. Then loop restart or idle. |
| `error` | Something failed (mic denied, STT error, WS disconnect, etc.). Recoverable via retry or user action. |

## Action Table (16 actions)

| Action | From states | To state | What triggers it |
|---|---|---|---|
| `acquire_mic` | `idle`, `turn:completing`, `error` | `mic:acquiring` | User taps mic; voice loop restart; retry after error |
| `start_monitoring` | `mic:acquiring` | `mic:monitoring` | After mic acquisition when wake-on-speech desired |
| `start_recording` | `mic:acquiring`, `mic:monitoring` | `mic:recording` | After mic acquisition (button/loop); speech detected in monitoring |
| `stop_recording` | `mic:recording` | `stt:transcribing` | Silence auto-stop; user taps mic while recording |
| `submit_message` | `idle`, `stt:transcribing` | `agent:submitting` | STT returns text; user sends typed message |
| `start_streaming` | `agent:submitting` | `agent:streaming` | First substantive WebSocket event for the turn |
| `finish_streaming` | `agent:streaming`, `tts:speaking` | `turn:completing` | OpenCode sends `status: idle` or `done` event |
| `start_tts` | `agent:streaming`, `turn:completing` | `tts:speaking` | First TTS AudioBufferSourceNode.start() call |
| `drain_tts` | `tts:speaking` | `turn:completing` | Last TTS queue item finishes, queue empty |
| `complete_turn` | `turn:completing` | `idle` | Voice loop didn't start; TTS fully drained and no loop |
| `enter_question` | `agent:streaming` | `agent:awaiting-question` | Agent sends `question_request` event |
| `resolve_question` | `agent:awaiting-question` | `agent:streaming` | Agent sends `question_replied` event |
| `enter_permission` | `agent:streaming` | `agent:awaiting-permission` | Agent sends `permission_request` event |
| `resolve_permission` | `agent:awaiting-permission` | `agent:streaming` | Agent sends `permission_replied` event |
| `error` | any except `idle` | `error` | Mic denied; getUserMedia failed; STT failed; WS send failed |
| `recover` | `error` | `idle` | User dismisses error; automatic retry path |

**`abort`** (special case): any state → `idle`. Bypasses the action table entirely.

## State Transition Diagram

```
idle ──► mic:acquiring ──► mic:monitoring ──► mic:recording
                │                                    │
                └──► mic:recording                   ▼
                                            stt:transcribing
                                                     │
                                                     ▼
                                            agent:submitting
                                                     │
                                                     ▼
                                            agent:streaming ──► agent:awaiting-question
                                                     │                    │
                                                     │          ◄────────┘
                                                     │
                                                     ├──► agent:awaiting-permission
                                                     │                    │
                                                     │          ◄────────┘
                                                     │
                                              ┌──────┤
                                              ▼      ▼
                                        tts:speaking  turn:completing
                                              │            │
                                              └──► turn:completing
                                                     │
                                                     ▼
                                                   idle (or mic:acquiring for voice loop)

  Any state ──► error ──► idle (via recover)
  Any state ──► idle (via abort)
```

### Key: Duplicate finish_streaming Protection

OpenCode sends both `status: idle` AND `done` events at end-of-turn. Both trigger `dispatch('finish_streaming')`. The first succeeds (moving state to `turn:completing`), the second is automatically rejected because `turn:completing` is not in `finish_streaming`'s `from` list. This is the structural fix for the voice loop breakage bug.

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
  "state": "agent:streaming",
  "level": "info",
  "component": "state",
  "event": "action_dispatched",
  "data": { "action": "finish_streaming", "from": "agent:streaming", "to": "turn:completing", "trigger": "finish_streaming" }
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

### Voice loop not restarting

Look for:
- `loop` / `loop_started` — should fire after TTS finishes and agent is done
- `state` / `action_dispatched` with `action: finish_streaming` — confirms agent turn ended
- `state` / `action_rejected` with `action: finish_streaming` — duplicate end-of-turn event correctly rejected
- `tts` / `queue_drained` — confirms TTS finished all items
- Check `state` field — should be `idle` or `turn:completing` when loop tries to restart

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
