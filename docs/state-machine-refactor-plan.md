# Action-Gated State Machine Refactor Plan

## Goal

Replace the passive state machine validator (transitionState returns true/false but callers ignore it) with an action-gated dispatch model where callers submit named actions and the state machine decides whether to execute them based on the current state.

## Design Decisions

1. **Pure gate pattern** — `dispatch()` only checks state and transitions. Side effects remain in the calling composables. The state machine has zero dependencies on TTS, audio, voice, or agent logic.
2. **Sync dispatch** — `dispatch(action) → boolean` is always synchronous. Async side effects are the caller's responsibility.
3. **Abort is a special case** — `abort()` bypasses the action table and force-resets to `idle` from any state.

## States (12 total)

| State | Description |
|---|---|
| `idle` | Nothing active. Waiting for user input (mic tap, typed message, or voice loop restart). Resting state between turns. |
| `mic:acquiring` | Browser requesting mic access via `getUserMedia()`. May show permission prompt. Also entered when reusing a kept-open stream. |
| `mic:monitoring` | Mic open, AnalyserNode sampling RMS at ~2Hz, waiting for speech to cross threshold. For future hands-free activation. |
| `mic:recording` | MediaRecorder actively capturing audio. Stops on manual tap or silence auto-detection (~1.5s). |
| `stt:transcribing` | Audio blob sent to `/api/stt/transcribe`. Waiting for transcription response. |
| `agent:submitting` | Message sent to agent via WebSocket. Waiting for first substantive event back. Typically very brief (~tens of ms). |
| `agent:streaming` | Agent generating response. Text deltas accumulated, run through TTS condenser/chunker, enqueued to TTS. UI renders in real time. |
| `agent:awaiting-question` | Agent asked a clarifying question. UI shows input. Voice loop restarts so user can answer by voice. |
| `agent:awaiting-permission` | Agent requested permission for destructive action. UI shows approve/deny. Voice loop does NOT restart (prevents accidental approval). |
| `tts:speaking` | TTS audio playing through AudioContext. Queue processes sequentially. Persists until all queued items finish. |
| `turn:completing` | Agent done streaming AND TTS finished/finishing. Cleanup state: flush chunker, reset batcher, stop hum, play chime. Then loop restart or idle. |
| `error` | Something failed (mic denied, STT error, WS disconnect, etc.). Error metadata stored. Recoverable via retry or user action. |

## Action Table

| Action | From states | To state | What triggers it | What the caller does on success |
|---|---|---|---|---|
| `acquire_mic` | `idle`, `turn:completing`, `error` | `mic:acquiring` | User taps mic; voice loop restart; retry after error | Calls `getUserMedia()` or reuses kept-open stream. Sets up AudioContext + AnalyserNode. |
| `start_monitoring` | `mic:acquiring` | `mic:monitoring` | After mic acquisition when wake-on-speech desired | Starts RMS polling loop (~2Hz). Transitions to recording on speech detection. |
| `start_recording` | `mic:acquiring`, `mic:monitoring` | `mic:recording` | After mic acquisition (button/loop); speech detected in monitoring | Creates MediaRecorder, starts it. Sets up silence detection. Plays recording feedback. |
| `stop_recording` | `mic:recording` | `stt:transcribing` | Silence auto-stop; user taps mic while recording | Stops MediaRecorder. Sends audio blob to STT endpoint. Returns transcribed text. |
| `submit_message` | `idle`, `stt:transcribing` | `agent:submitting` | STT returns text; user sends typed message; user answers question | Sends WebSocket message. Adds user message to UI. Sets voiceInitiatedTurn. Resets per-turn state. |
| `start_streaming` | `agent:submitting` | `agent:streaming` | First substantive WebSocket event for the turn | Creates assistant message in UI. Starts working hum audio feedback. |
| `finish_streaming` | `agent:streaming`, `tts:speaking` | `turn:completing` | OpenCode sends `status: idle` or `done` event | Stops hum, plays chime. Flushes TTS chunker. Resets batcher/condenser. Sets isStreaming=false. Attempts voice loop restart. |
| `start_tts` | `agent:streaming`, `turn:completing` | `tts:speaking` | First TTS AudioBufferSourceNode.start() call | (Transition only — playback already initiated by useTTS.processQueue.) |
| `drain_tts` | `tts:speaking` | `turn:completing` | Last TTS queue item finishes, queue empty | (Transition only — processQueue already stopped.) |
| `complete_turn` | `turn:completing` | `idle` | Voice loop didn't start; TTS fully drained and no loop | Returns system to resting state. |
| `enter_question` | `agent:streaming` | `agent:awaiting-question` | Agent sends `question_request` event | Adds question message to UI. Plays question chime. Speaks question via TTS. |
| `resolve_question` | `agent:awaiting-question` | `agent:streaming` | Agent sends `question_replied` event | Marks question resolved. Agent resumes streaming. |
| `enter_permission` | `agent:streaming` | `agent:awaiting-permission` | Agent sends `permission_request` event | Adds permission message to UI. Plays permission chime. |
| `resolve_permission` | `agent:awaiting-permission` | `agent:streaming` | Agent sends `permission_replied` event | Marks permission resolved. Agent resumes streaming. |
| `error` | any except `idle` | `error` | Mic denied; getUserMedia failed; STT failed; WS send failed; MediaRecorder failed | Stores error metadata. Plays error tone. |
| `recover` | `error` | `idle` | User dismisses error; automatic retry path | Clears error metadata. |
| `abort` | **any (special case)** | `idle` | User taps stop/abort button | Force-resets. Sends abort to backend. Stops TTS. Stops hum. Plays cancel tone. |

## New File: `frontend/composables/useStateMachine.ts`

```ts
// Action table as a Record<string, { from: State[], to: State }>
// dispatch(action: string): boolean
//   - Checks current state against action.from
//   - If valid: transitions state, logs, returns true
//   - If invalid: logs warning, returns false
// abort(trigger: string): void
//   - Force-resets to idle from any state
// getState(): State
// state: Readonly<Ref<State>>
```

No imports from useAgent, useVoice, useTTS, useAudioFeedback, or any other composable. Only imports Vue's `ref`/`readonly` and the debug log for logging.

## Migration: File-by-File Changes

### `useAgent.ts`

**Event handler dispatch (lines 376, 378-383):**
```
// Before:
case 'done': finishStreaming(); break;
case 'status': if (event.content === 'idle') finishStreaming(); break;

// After:
case 'done':
case 'status':
  if (event.type === 'done' || event.content === 'idle') {
    if (dispatch('finish_streaming')) {
      doFinishStreamingSideEffects();
    }
  }
  break;
```
The duplicate call is now structurally impossible — the first dispatch moves state to `turn:completing`, the second dispatch is rejected because `turn:completing` is not in `finish_streaming.from`.

**Delete `streamingFinished` guard** — no longer needed.

**`finishStreaming()` refactored to `doFinishStreamingSideEffects()`:**
- Remove `transitionState('turn:completing', ...)` call (dispatch already did it)
- Remove the synchronous idle fallback — replace with `dispatch('complete_turn')` gated on `!isTTSPlaying.value`

**`startLoopRecording()`:**
- Replace `transitionState('mic:acquiring', ...)` with `dispatch('acquire_mic')`

**`sendMessage()`:**
- Replace `transitionState('agent:submitting', ...)` with `dispatch('submit_message')`
- Replace `transitionState('error', ...)` with `dispatch('error')`

**`handleAgentEvent()` first-event transition (line 314):**
- Replace `transitionState('agent:streaming', ...)` with `dispatch('start_streaming')`

**Question/permission handlers:**
- `dispatch('enter_question')`, `dispatch('resolve_question')`
- `dispatch('enter_permission')`, `dispatch('resolve_permission')`

**`abortSession()`:**
- Replace `resetState('abort_session')` with `abort('abort_session')`

**`isTTSPlaying` watcher:**
- After `startLoopRecording()`, call `dispatch('complete_turn')` if state is still `turn:completing`

**Auto-stop handler (STT empty):**
- Replace `transitionState('idle', 'stt_empty')` with `dispatch('recover')` or `dispatch('complete_turn')`

### `useVoice.ts`

| Current call | Replacement |
|---|---|
| `transitionState('mic:acquiring', ...)` | `dispatch('acquire_mic')` |
| `transitionState('mic:monitoring', ...)` | `dispatch('start_monitoring')` |
| `transitionState('mic:recording', ...)` | `dispatch('start_recording')` |
| `transitionState('stt:transcribing', ...)` | `dispatch('stop_recording')` |
| `transitionState('error', ...)` | `dispatch('error')` |

### `useTTS.ts`

| Current call | Replacement |
|---|---|
| `transitionState('tts:speaking', ...)` (line 206) | `dispatch('start_tts')` |
| `transitionState('turn:completing', ...)` (line 269) | `dispatch('drain_tts')` |

### `VoiceButton.vue`

| Current call | Replacement |
|---|---|
| `resetState('no_transcription')` | `abort('no_transcription')` or `dispatch('recover')` depending on current state |

### `useInteractionState.ts`

After all callers are migrated:
- Remove `transitionState()`, `resetState()`, `getInteractionState()` exports
- Either delete the file or keep it as a re-export of `useStateMachine` for the `useInteractionState()` composable (computed flags like `isRecording`, `isStreaming`, etc.)

## Tests: `__tests__/useStateMachine.test.ts`

### Exhaustive action/state matrix
For every action in the table, test:
- Each `from` state → dispatch succeeds, state transitions to `to`
- Every other state → dispatch fails, state unchanged

### Scenario tests
1. **Happy path voice round-trip**: `acquire_mic` → `start_recording` → `stop_recording` → `submit_message` → `start_streaming` → `start_tts` → `finish_streaming` → `complete_turn`
2. **Duplicate finish_streaming**: dispatch succeeds once, second dispatch from `turn:completing` rejected
3. **TTS starts during streaming, finishes after**: `start_tts` from `agent:streaming`, `finish_streaming` from `tts:speaking`, `drain_tts` rejected (already in `turn:completing`)
4. **TTS drain drives turn completion**: `drain_tts` → `complete_turn`
5. **Abort from every state**: verify abort always succeeds
6. **Question interrupt**: `enter_question` from `agent:streaming`, `resolve_question` back to `agent:streaming`
7. **Permission interrupt**: same pattern
8. **Error and recovery**: `error` from various states, `recover` back to `idle`
9. **Error from idle rejected**: `dispatch('error')` from `idle` returns false

### Port existing tests
Migrate `useInteractionState.test.ts` assertions to use `dispatch()` instead of `transitionState()`.

## Documentation: `docs/frontend-interaction-state-machine.md`

After implementation, update with:
- State descriptions table (from this plan)
- Action table with triggers and side effects
- Updated state diagram with action names as edge labels
- Note that `dispatch()` is the only public API for state changes
- Updated debug log section (action rejections are logged as warnings)

## Execution Order

1. Create `useStateMachine.ts` — action table + dispatch + abort + getState
2. Write `useStateMachine.test.ts` — exhaustive matrix + scenarios
3. Migrate `useAgent.ts` — largest consumer, delete streamingFinished guard
4. Migrate `useVoice.ts`
5. Migrate `useTTS.ts`
6. Migrate `VoiceButton.vue`
7. Clean up `useInteractionState.ts` — thin wrapper or delete
8. Delete old `useInteractionState.test.ts` (replaced by new tests)
9. Run full test suite
10. Update `docs/frontend-interaction-state-machine.md`
