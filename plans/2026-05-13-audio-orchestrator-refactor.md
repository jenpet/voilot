# Plan: Audio Orchestrator Refactor

**Status:** approved
**Created:** 2026-05-13
**Author:** user + agent

## Goal

Replace the scattered audio cue logic in `useAgent.ts` (~40 imperative calls across 1175 lines) with a declarative, state-machine-driven audio orchestrator. Fix a class of bugs caused by ad-hoc audio triggers: hum restart ping-pong, iOS TTS not playing, voice loop not restarting, success chime on invisible turns, and `onQueueDrained` callback being permanently destroyed.

## Context

### Current Problems

1. **`useAgent.ts` is a god-composable** — 1175 lines mixing agent communication, audio cues, TTS pipeline, voice loop, and reactive watchers
2. **Audio cues are imperative and scattered** — 40+ `playX()` / `startWorkingHum()` / `stopWorkingHum()` calls placed manually next to `dispatch()` calls, leading to missed/duplicate triggers
3. **VoiceButton bypasses useAgent** — directly calls `useVoice`, `useStateMachine`, `useAudioFeedback`, and `useTTS`, creating multiple independent state manipulators
4. **No listener system on the state machine** — `dispatch()` returns a boolean but nobody reacts to transitions; audio cues are manually placed
5. **Shared state is ad-hoc** — some refs use `useState` (shared), some use `ref` (local), no convention about ownership
6. **iOS Safari audio unlock** — `unlockAudio()` only called on VoiceButton tap or voice toggle, so TTS fails silently on first auto-voice greeting

### Bugs Fixed by This Refactor

| Bug | Root Cause | Fix |
|-----|-----------|-----|
| `onQueueDrained` permanently destroyed after abort | `stop()` in `useTTS.ts` nulled the callback | Already fixed — don't null callback in `stop()` |
| `autoVoiceArmed` never consumed for `session_busy_on_load` | Check in `handleTextEvent` requires `idle` state, but `session_busy_on_load` already entered `agent_turn` | Consume on any `→ agent_turn` transition in SM hook |
| No working hum during auto-voice initial response | `startWorkingHum()` only called in `sendMessage()` and `session_busy_on_load`, not in `auto_voice_initial` | Start hum on every `→ agent_turn` transition |
| Hum restart ping-pong during text streaming | Fix 3 added `startWorkingHum()` on every SSE event while `autoVoiceArmed` stayed true | One-shot hum start via transition hook, not per-event |
| Hum overlapping TTS playback | Hum and TTS use independent audio paths, nobody stops hum when TTS starts | Watcher: `isTTSPlaying` → stop hum |
| Success chime for invisible initial turns | `finishStreaming()` always plays chime regardless of `suppressInitialTools` | Check `suppressInitialTools` in `→ idle` hook |
| iOS TTS not playing first message | AudioContext not unlocked (no prior user gesture) | Early `unlockAudio()` in `app.vue` on first tap |
| Two chimes on session start | Handoff tone fires on `sendMessage`, not tied to specific triggers | Handoff only on `send_message` trigger |

## Architecture

See `docs/diagrams/audio-orchestrator-current.md` for the current architecture.
See `docs/diagrams/audio-orchestrator-future.md` for the target architecture.
See `docs/diagrams/audio-orchestrator-sequence.md` for the voice turn sequence diagram.

## Design Decisions

### 1. State Machine Transition Hooks (Option A)

Add `onTransition(cb)` listener API to the existing state machine. Audio cues are mapped declaratively to transitions rather than placed imperatively next to `dispatch()` calls.

**Decided against**: Separate audio state machine (over-engineered), centralized handler function (still imperative).

### 2. Audio Orchestrator as Subsystem of useAgent (Variant B)

The orchestrator owns audio cues, TTS pipeline, AND the voice loop (mic lifecycle, auto-stop, silence detection). `useAgent` passes `sendMessage` as a callback. VoiceButton calls `orchestrator.toggleRecording()`.

**Decided against**: Keeping voice loop in `useAgent` (Variant A) — would leave voice state split across two composables and not fully solve the VoiceButton bypass problem.

### 3. Shared Reactive State via `useSessionState`

Refs that cross composable boundaries live in a central `useSessionState` composable. Each ref is documented with its owner. Internal flags (`agentDone`, `loopStartPending`, `abortedTurn`) stay private in the owning composable.

**Design rule**: Shared state goes in `useSessionState`. Derived state (computed refs like `hasPendingQuestion`) and read-only dependencies (like `connectionState`) are passed as constructor args. Internal flags stay private. Each shared ref has a documented owner — only the owner writes, everyone else reads.

### 4. VoiceButton Becomes a Thin UI Trigger

VoiceButton calls `orchestrator.toggleRecording()` instead of directly calling `useVoice`, `useStateMachine`, and `useAudioFeedback`. The early `unlockAudio()` in `app.vue` removes the iOS user-gesture constraint for downstream audio operations.

### 5. Early iOS Audio Unlock

One-shot `click`/`touchstart` listener on `document` in `app.vue` calls `unlockAudio()` on the very first user interaction. By the time the user navigates to a session page, the AudioContext is already running.

### 6. SM Abort Notifications

Both `dispatch()` and `abort()` notify the same `onTransition` listeners. Abort passes `action: 'abort'` so the orchestrator can distinguish it from normal transitions. No separate `onAbort` API.

## Shared State Inventory (`useSessionState`)

| Ref | Type | Owner (writes) | Readers | Why shared |
|-----|------|---------------|---------|------------|
| `isStreaming` | `boolean` | useAgent | orchestrator, session page | Orchestrator reads for voice loop guards; page reads for isBusy |
| `voiceEnabled` | `boolean` | orchestrator | useAgent (re-export), session page, VoiceButton | Controls all audio/voice behavior across multiple components |
| `voiceInitiatedTurn` | `boolean` | orchestrator | useAgent (sendMessage origin tracking) | useAgent needs to know if current turn was voice-initiated |
| `suppressInitialTools` | `boolean` | useAgent | orchestrator | Orchestrator reads for tool batcher gating and success chime gating |
| `loopRecordingActive` | `boolean` | orchestrator | useRecordingFeedback (stop blip suppression) | Recording feedback needs to know if loop is active to suppress blips |

## SM Transition → Audio Mapping

| Transition | Trigger | Audio Action |
|-----------|---------|-------------|
| `* → agent_turn` | `send_message` | handoff tone, start hum, start watchdog |
| `* → agent_turn` | `session_busy_on_load` | start hum |
| `* → agent_turn` | `auto_voice_*` | start hum |
| `* → agent_turn` | `permission_response`, `question_*` | (nothing — returning from input) |
| `* → agent_turn` | any (if `autoVoiceArmed`) | consume flag → set `voiceInitiatedTurn = true` |
| `agent_turn → idle` | normal | success chime (unless `suppressInitialTools`) |
| `agent_turn → idle` | abort | (nothing — cancel tone played during abort) |
| `agent_turn → awaiting_input` | `permission_request` | stop hum, permission chime |
| `agent_turn → awaiting_input` | `question_request` | stop hum, question chime |
| `idle → user_turn` | `loop_recording_start` | loop listening tick |
| abort from any state | `abort_session` | stop hum, cancel tone, TTS "Stopped." |

**Reactive rules (not transitions):**
- `watch(isTTSPlaying)`: true → stop hum
- `watch(isTTSPlaying)`: false + `agentDone` → `tryCompleteTurn()`
- `watch(voiceEnabled)`: off → stop monitoring, stop TTS
- `watch(connectionState)`: disconnect → warning tone + "Connection lost."; reconnect → chime + "Reconnected."

## Orchestrator Interface

### Constructor Args (non-shared read-only deps)

```typescript
interface AudioOrchestratorOptions {
  sessionId: string
  sendMessage: (text: string) => void
  hasPendingQuestion: ComputedRef<boolean>
  hasPendingPermission: ComputedRef<boolean>
  connectionState: Ref<string>
}
```

### Public API

```typescript
interface AudioOrchestrator {
  // Refs (for UI / parent composable)
  isTTSPlaying: Readonly<Ref<boolean>>
  isRecording: Readonly<Ref<boolean>>
  isMonitoring: Readonly<Ref<boolean>>

  // TTS pipeline
  feedTextDelta(delta: string, partId: string, display?: string): void
  feedToolEvent(event: AgentEvent): void
  flushToolBatcher(): void
  notifyStreamingDone(context: { abortedTurn: boolean }): void

  // Voice control
  toggleRecording(): Promise<string | null>  // for VoiceButton
  stopManualRecording(): Promise<string | null>
  toggleVoice(): void
  stopPlayback(): void
  handleAbort(): void

  // Announcements
  announcePermission(title: string): void
  announceQuestion(text: string): void
  announceAnswer(label: string): void
  notifyToolActivity(): void

  // Lifecycle
  cleanup(): void
}
```

## Files Changed

| File | Change | Lines |
|------|--------|-------|
| `composables/useSessionState.ts` | **New** — shared reactive state | ~40 |
| `composables/useAudioOrchestrator.ts` | **New** — audio/voice/TTS orchestrator | ~350-400 |
| `composables/useStateMachine.ts` | Add `onTransition()` listener API | ~25 added |
| `composables/useAgent.ts` | Remove audio/voice/TTS, delegate to orchestrator | ~200 removed, ~50 added |
| `app.vue` | Early `unlockAudio()` on first interaction | ~10 added |
| `components/VoiceButton.vue` | Thin wrapper calling orchestrator | ~20 changed |
| `pages/session/[id].vue` | Use orchestrator methods from useAgent return | ~15 changed |
| `composables/useRecordingFeedback.ts` | Read `loopRecordingActive` from `useSessionState` | ~5 changed |

### Files NOT Changed

- `useAudioFeedback.ts` — stays as low-level audio primitive library
- `useTTS.ts` — no changes (except the already-applied `onQueueDrained` fix)
- `useTTSChunker.ts`, `useTTSCondenser.ts`, `useTTSToolBatcher.ts` — no changes, just instantiated by orchestrator
- `useVoice.ts` — no changes, instantiated by orchestrator
