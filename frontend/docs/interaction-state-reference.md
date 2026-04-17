# Interaction State Reference

This document describes the voilot interaction state machine, component registry, and debug logging system.

## Interaction States

| State | Description |
|---|---|
| `idle` | No active interaction. Mic closed or monitoring. |
| `mic:acquiring` | Requesting mic permission / getUserMedia in progress. |
| `mic:monitoring` | Mic open, monitoring RMS for speech detection (wake-on-speech). |
| `mic:recording` | Actively recording user speech via MediaRecorder. |
| `stt:transcribing` | Recording stopped, audio sent to STT service, awaiting transcription. |
| `agent:submitting` | Transcription received, message sent to agent via WebSocket. |
| `agent:streaming` | Agent is streaming its response (text, tool_use, etc.). |
| `agent:awaiting-question` | Agent asked a question, waiting for user's answer. |
| `agent:awaiting-permission` | Agent requested permission, waiting for user's response. |
| `tts:speaking` | TTS is playing the agent's spoken response. |
| `turn:completing` | TTS finished, cleaning up before starting next turn. |
| `error` | An error occurred; recoverable via retry or reset to idle. |

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
                                                     ▼
                                              tts:speaking
                                                     │
                                                     ▼
                                            turn:completing
                                                     │
                                                     ▼
                                                   idle (or mic:acquiring for voice loop)

  Any state ──► error ──► idle | mic:acquiring
```

## Allowed Transitions

| From | Allowed Targets |
|---|---|
| `idle` | `mic:acquiring` |
| `mic:acquiring` | `mic:monitoring`, `mic:recording`, `error`, `idle` |
| `mic:monitoring` | `mic:recording`, `idle`, `error` |
| `mic:recording` | `stt:transcribing`, `idle`, `error` |
| `stt:transcribing` | `agent:submitting`, `idle`, `error` |
| `agent:submitting` | `agent:streaming`, `error` |
| `agent:streaming` | `agent:awaiting-question`, `agent:awaiting-permission`, `tts:speaking`, `turn:completing`, `error`, `idle` |
| `agent:awaiting-question` | `agent:streaming`, `idle`, `error` |
| `agent:awaiting-permission` | `agent:streaming`, `idle`, `error` |
| `tts:speaking` | `turn:completing`, `mic:recording`, `idle`, `error` |
| `turn:completing` | `mic:acquiring`, `idle` |
| `error` | `idle`, `mic:acquiring` |

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
| `state` | `useInteractionState.ts` | State machine transitions |

## Debug Log Entry Format

```json
{
  "timestamp": 1713340800000,
  "elapsed": 5432,
  "state": "agent:streaming",
  "level": "info",
  "component": "tts",
  "event": "enqueue",
  "data": { "text": "Here is the answer...", "queueLength": 2 }
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `timestamp` | number | Unix epoch ms |
| `elapsed` | number | Ms since debug recording started |
| `state` | string | Current interaction state at log time |
| `level` | `debug` \| `info` \| `warn` \| `error` | Severity |
| `component` | string | Source component (see registry above) |
| `event` | string | Event identifier (e.g., `transition`, `enqueue`, `mic_acquired`) |
| `data` | object? | Optional structured metadata |

### Log Levels

- **debug** — Verbose operational data: RMS samples, individual deltas, audio feedback cues
- **info** — Significant lifecycle events: state transitions, message send/receive, loop start/stop
- **warn** — Recoverable issues: invalid transitions, empty STT results, connection drops
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
- `agent` / `finish_streaming` — confirms agent turn ended
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
