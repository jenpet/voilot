# State Machine Rewrite: 12-State → 4-State Turn Lifecycle

## Motivation

The 12-state interaction state machine conflates two orthogonal concerns: **turn lifecycle** (whose turn is it?) and **audio/media state** (what's playing/recording?). This coupling is the root cause of every voice loop bug encountered so far:

- Check-in TTS during `agent:streaming` triggers `start_tts`/`drain_tts`, corrupting state
- `turn:completing` creates complex coupling between "agent done," "TTS done," and "loop restart"
- `mic:acquiring`/`mic:monitoring`/`mic:recording` as states makes barge-in impossible (mic must be independent of turn)

## New Architecture

### Turn State (4 states, state machine)

```
idle → user_turn → agent_turn → idle
  ↑                               |
  +-------------------------------+ (voice loop)
```

| State | Description |
|---|---|
| `idle` | No active turn. Waiting for user input or voice loop restart. |
| `user_turn` | User is composing: mic acquisition, recording, transcribing, or typing. |
| `agent_turn` | Message submitted through agent finishing. Covers streaming, questions, permissions. |
| `error` | Something failed. Recoverable via `recover` → `idle`. |

### Action Table (5 actions + abort)

| Action | From | To | Trigger |
|---|---|---|---|
| `start_user_turn` | `idle` | `user_turn` | Mic tap, voice loop restart |
| `start_agent_turn` | `user_turn`, `idle` | `agent_turn` | Message submitted (voice or typed) |
| `complete_turn` | `agent_turn` | `idle` | Agent done + TTS drained |
| `error` | any except `idle` | `error` | Mic denied, STT failed, WS error |
| `recover` | `error` | `idle` | User dismisses error |
| **`abort`** | any → `idle` | (special) | User abort, cleanup |

`start_agent_turn` accepts `idle` so typed messages (no mic involved) skip `user_turn`.

### Audio/Media State (reactive booleans, no machine)

These existing refs replace the removed states:

| Concern | Ref | Source | Replaces state |
|---|---|---|---|
| TTS playing | `isPlaying` | `useTTS.ts` | `tts:speaking` |
| Mic recording | `isRecording` | `useVoice.ts` | `mic:recording` |
| Mic monitoring | `isMonitoring` | `useVoice.ts` | `mic:monitoring` |
| Working hum | `isHumActive()` | `useAudioFeedback.ts` | (part of `agent:streaming`) |

### Voice Loop (reactive watcher)

```ts
watch([turnState, isTTSPlaying], ([turn, playing]) => {
  if (turn === 'idle' && !playing && voiceInitiated) {
    startNextVoiceTurn()
  }
})
```

Replaces the complex `turn:completing → acquire_mic → start_recording` chain.

### TTS Queue Drain (callback, not watcher)

`useTTS` exposes `onQueueDrained(cb)`. `useAgent` registers a callback that dispatches `complete_turn` when agent is also done. This avoids false triggers from brief gaps between queue items.

## File-by-File Changes

### 1. `useStateMachine.ts` — Rewrite

- 4 states, 5 actions, same `dispatch()`/`abort()`/`getState()` API
- Remove all computed flags (unused externally)
- Keep `_log()`, `_forceStateForTesting()`

### 2. `useVoice.ts` — Remove dispatches

- Delete all `dispatch()` calls (acquire_mic, start_monitoring, start_recording, stop_recording, error)
- Replace `getState() === 'mic:monitoring'` check with local `isMonitoring` ref
- Keep all `_log()` calls for debug visibility

### 3. `useTTS.ts` — Remove dispatches, add callback

- Delete `dispatch('start_tts')` and `dispatch('drain_tts')`
- Delete `isCheckIn` flag, `enqueueCheckIn()`, `_realTTSDispatched` (no longer needed)
- Add `onQueueDrained(cb)` — called when queue empties after processing
- Keep `isPlaying` ref

### 4. `useAudioFeedback.ts` — Remove check-in bypass

- Delete `_enqueueCheckInTTS`, `setTTSCheckInEnqueue()`
- All TTS enqueues use regular `_enqueueTTS` (safe — TTS no longer drives state)

### 5. `useAgent.ts` — Simplify orchestration

- `sendMessage()`: `dispatch('start_agent_turn')`
- Voice transcription path: already in `user_turn`, dispatch `start_agent_turn`
- First agent event: no dispatch needed (already in `agent_turn`)
- `finishStreaming()`: set `agentDone` flag. If TTS not playing, `dispatch('complete_turn')`. Otherwise let `onQueueDrained` callback handle it.
- Voice loop: watcher on `[turnState, isTTSPlaying]`
- Remove all check-in TTS plumbing
- Question/permission: local refs only, no dispatch

### 6. `VoiceButton.vue` — Simplify

- Remove `abort` import from useStateMachine
- Start path: dispatch `start_user_turn`
- No-transcription path: `abort()` (same as before)

### 7. `useInteractionState.ts` — Delete (dead code)

### 8. `useStateMachine.test.ts` — Rewrite for 4-state model

### 9. `docs/frontend-interaction-state-machine.md` — Update

## Migration Order

1. useStateMachine.ts (new states/actions)
2. useVoice.ts (remove old dispatches)
3. useTTS.ts (remove old dispatches, add onQueueDrained)
4. useAudioFeedback.ts (remove check-in bypass)
5. useAgent.ts (new dispatch calls, voice loop watcher)
6. VoiceButton.vue (new dispatch calls)
7. Delete useInteractionState.ts
8. useStateMachine.test.ts
9. docs/frontend-interaction-state-machine.md
10. Build verification
