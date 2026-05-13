# Current Audio Architecture

How audio cues, TTS, and voice loop are wired today. The core problem: everything is mixed into `useAgent.ts` with 40+ scattered imperative audio calls.

```mermaid
graph TB
    subgraph "Pages / Components"
        PAGE["session/[id].vue"]
        VB["VoiceButton.vue"]
    end

    subgraph "useAgent.ts (~1175 lines)"
        direction TB
        AG_ALL["Everything mixed together:<br/>─────────────────────────<br/>WebSocket events & message handling<br/>REST API (fetchSession, fetchMessages)<br/>sendMessage / abortSession<br/>Permission / Question logic<br/>──── AUDIO (scattered) ────<br/>40+ playX() / startHum / stopHum calls<br/>TTS pipeline setup (chunker, condenser, batcher)<br/>Voice loop (startLoopRecording, auto-stop)<br/>autoVoiceArmed / voiceInitiatedTurn flags<br/>WS disconnect/reconnect audio<br/>isTTSPlaying watcher<br/>voiceEnabled watcher<br/>finishStreaming() (audio + state mixed)<br/>tryCompleteTurn()"]
    end

    subgraph "Low-level Primitives"
        AF["useAudioFeedback.ts"]
        TTS["useTTS.ts"]
        VOICE["useVoice.ts"]
        RF["useRecordingFeedback.ts"]
        CHUNK["useTTSChunker / Condenser / ToolBatcher"]
    end

    SM["useStateMachine.ts<br/>(no hooks, fire-and-forget dispatch)"]

    PAGE -- "everything" --> AG_ALL
    VB -- "startRecording, stopRecording<br/>dispatch (directly)<br/>playStartBlip (directly)" --> VOICE
    VB -- "unlockAudio (directly)" --> TTS
    VB -- "dispatch (directly)" --> SM

    AG_ALL -- "dispatch, abort" --> SM
    AG_ALL -- "40+ individual calls" --> AF
    AG_ALL -- "enqueue, stop, onQueueDrained" --> TTS
    AG_ALL -- "instantiates & uses" --> VOICE
    AG_ALL -- "instantiates" --> CHUNK
    AG_ALL -- "suppressNextStopBlip" --> RF
```

## Problems Visible

1. **`useAgent.ts` is a god-composable** — agent communication, audio cues, TTS pipeline, voice loop all in one file
2. **VoiceButton bypasses useAgent** — directly calls `useVoice`, `useStateMachine`, `useAudioFeedback`, and `useTTS` independently
3. **No listener system on the state machine** — audio cues manually placed next to each `dispatch()` call
4. **Shared state is ad-hoc** — some refs use `useState` (shared), some use `ref` (local), no ownership convention
