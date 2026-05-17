import type { Session } from './useSession'
import type { AgentEvent } from './useWebSocket'
import type { InjectionKey } from 'vue'
import { useTTSChunker } from './useTTSChunker'
import { useTTSCondenser } from './useTTSCondenser'
import { useTTSToolBatcher } from './useTTSToolBatcher'
import { suppressNextStopBlip } from './useRecordingFeedback'
import { useDebugLog } from './useDebugLog'
import {
  dispatch,
  abort,
  getState,
} from './useStateMachine'
import {
  stopWorkingHum,
  playErrorTone,
  playSTTFailureTone,
  notifyToolActivity,
  cancelWatchdog,
  setTTSEnqueue,
  stopAll as stopAllAudioFeedback,
} from './useAudioFeedback'
import { useAudioOrchestrator } from './useAudioOrchestrator'
import { useAgentMessages, type ChatMessage } from './useAgentMessages'

// Re-export ChatMessage as Message for backwards compatibility
export type Message = ChatMessage

// Injection key for respondToPermission — allows ChatMessage to call it without prop drilling.
export const RespondToPermissionKey: InjectionKey<
  (permissionId: string, response: 'once' | 'always' | 'reject') => void
> = Symbol('respondToPermission')

// Injection key for respondToQuestion — allows ChatMessage to select an option.
export const RespondToQuestionKey: InjectionKey<
  (questionId: string, questionIndex: number, selectedLabels: string[]) => void
> = Symbol('respondToQuestion')

// Injection key for rejectQuestion — allows ChatMessage to dismiss a question.
export const RejectQuestionKey: InjectionKey<
  (questionId: string) => void
> = Symbol('rejectQuestion')

// Injection key for the currently active (first unanswered) question identifier.
// Format: "questionId:questionIndex" or null if no question is pending.
export const ActiveQuestionKey: InjectionKey<Ref<string | null>> = Symbol('activeQuestion')

export function useAgent(sessionId: string) {
  const apiBase = `${resolveBackendUrl()}/api`
  const { send, subscribe, connectionState } = useWebSocket()
  const { enqueue: enqueueTTS, onQueueDrained, stop: stopTTS, isPlaying: isTTSPlaying } = useTTS()
  const { log } = useDebugLog()

  // Wire TTS into the audio feedback manager so it can speak
  // check-in messages and error announcements.
  setTTSEnqueue(enqueueTTS)
  const { mark, isActive: isTimerActive } = useRoundTripTimer()
  const {
    isRecording,
    isMonitoring,
    stopMonitoring,
    startRecording,
    stopRecording,
    forceStopRecording,
    setOnAutoStop,
  } = useVoice()

  const session = ref<Session | null>(null)
  const msgBuilder = useAgentMessages()
  const messages = msgBuilder.mutableMessages
  const isStreaming = ref(false)
  const isLoading = ref(true)
  const voiceEnabled = ref(true)

  // Derived: true when any permission_request message is unresolved
  const hasPendingPermission = computed(() =>
    messages.value.some(m => m.type === 'permission_request' && m.meta?.resolved !== true),
  )
  // Derived: true when any question_request message is unresolved
  const hasPendingQuestion = computed(() =>
    messages.value.some(m => m.type === 'question_request' && m.meta?.resolved !== true),
  )

  // ── Audio orchestrator: handles transition-driven audio cues ────
  const orchestrator = useAudioOrchestrator({
    sessionId,
    sendMessage: (text: string) => sendMessage(text),
    hasPendingQuestion,
    hasPendingPermission,
    connectionState,
    voiceEnabled,
    tts: { enqueue: enqueueTTS, stop: stopTTS, isPlaying: isTTSPlaying },
  })

  // Track whether the current turn was aborted by the user.
  // Prevents finishStreaming() from playing a success chime after abort.
  let abortedTurn = false

  // Track whether the agent has finished streaming for this turn.
  // Used by the TTS queue drain callback to know when to complete the turn.
  let agentDone = false

  // Track whether the current conversation turn was started by voice input.
  // When false (text-typed input), the mic auto-loop does NOT re-activate.
  const voiceInitiatedTurn = ref(false)

  // Auto-voice for new sessions: when enabled, treat the initial agent
  // response as voice-initiated so the mic auto-starts after the greeting.
  const { autoVoiceNewSessions } = useSettings()
  let autoVoiceArmed = autoVoiceNewSessions.value

  // Track partial answers for multi-question batches.
  // Key: questionId, Value: { totalQuestions, answers: Map<index, string[]> }
  const pendingQuestionAnswers = new Map<string, { totalQuestions: number; answers: Map<number, string[]> }>()
  // Track tool_use start times by partId so we can compute duration when tool_result arrives
  const toolStartTimes = new Map<string, number>()
  // Incremental TTS chunker: streams sentence-sized chunks to TTS as they arrive
  // The condenser strips code blocks, markdown formatting, etc. before speaking
  const ttsCondenser = useTTSCondenser()
  const ttsChunker = useTTSChunker(enqueueTTS, (text) => ttsCondenser.condense(text))
  // Batch consecutive tool-use events into a single TTS summary
  // (avoids "Using bash. Using bash. Using bash." when the agent runs many commands)
  const ttsToolBatcher = useTTSToolBatcher(enqueueTTS)

  // Shared flag so useRecordingFeedback knows whether to suppress blips
  // during auto-loop recordings (as opposed to manual VoiceButton taps).
  const loopRecordingActive = useState<boolean>('voice-loop-active', () => false)

  // Synchronous guard to prevent duplicate startLoopRecording calls.
  // startRecording() is async — between the call and isRecording.value=true,
  // a second caller can slip through the isRecording.value check.
  let loopStartPending = false

  // ─── Conversational Voice Loop ──────────────────────────────────
  //
  // Turn-based conversation — no interruption, no monitoring:
  //   1. User taps mic (VoiceButton) → records → silence auto-stops → STT → send
  //   2. Agent streams full response, TTS plays it all
  //   3. TTS finishes → immediately start recording (mic is already open)
  //   4. Silence auto-stops → STT → send → back to step 2
  //
  // The mic stream stays open across turns (keepMicOpen=true) so there's
  // no need to re-acquire it, which would fail on iOS Safari outside a
  // user gesture.

  // Start recording for the next conversational turn.
  // Called when the agent's turn is fully over (streaming done + TTS finished),
  // OR when a question is pending and the user needs to answer by voice.
  function startLoopRecording(): boolean {
    if (!voiceEnabled.value || !voiceInitiatedTurn.value || isRecording.value || isTTSPlaying.value || loopStartPending) {
      log('debug', 'loop', 'loop_guard_blocked', {
        voiceEnabled: voiceEnabled.value,
        voiceInitiatedTurn: voiceInitiatedTurn.value,
        isRecording: isRecording.value,
        isTTSPlaying: isTTSPlaying.value,
        loopStartPending,
      })
      return false
    }
    // Don't auto-record while waiting for user to respond to a permission prompt
    if (hasPendingPermission.value) return false
    // When a question is pending, allow recording (user answers by voice).
    // Otherwise, block if the agent is still streaming.
    if (!hasPendingQuestion.value && isStreaming.value) return false

    const accepted = dispatch('start_user_turn', 'loop_recording_start')
    if (!accepted) {
      log('warn', 'loop', 'loop_dispatch_rejected', { state: getState() })
      return false
    }

    loopStartPending = true
    loopRecordingActive.value = true
    log('info', 'loop', 'loop_started')
    // Loop listening tick is played by the orchestrator via onTransition
    startRecording() // reuses existing mic stream via ensureMicAndAnalyser()
    return true
  }

  // Handle auto-stop from silence detection — only for loop-triggered recordings.
  // Manual VoiceButton recordings are handled by VoiceButton's own auto-stop handler.
  const unsubAutoStop = setOnAutoStop(async () => {
    if (!isRecording.value || !loopRecordingActive.value) return

    // Stop recording, transcribe, and send.
    // Keep mic open so we can start recording again after the next agent turn.
    const text = await stopRecording(voiceEnabled.value)
    loopRecordingActive.value = false
    loopStartPending = false

    if (text) {
      sendMessage(text, { origin: 'voice' })
    } else {
      // No speech detected (empty/too short) — play failure tone, then retry
      log('warn', 'loop', 'stt_empty_retry')
      abort('stt_empty')
      playSTTFailureTone()
      startLoopRecording()
    }
  })

  // When TTS finishes and the agent is done, complete the turn and
  // optionally restart the voice loop.
  // Also handles awaiting_input: after question TTS finishes, start mic.
  onQueueDrained(() => {
    const currentState = getState()

    // In awaiting_input with a pending question: start mic for voice answer
    if (currentState === 'awaiting_input' && hasPendingQuestion.value
        && voiceInitiatedTurn.value && voiceEnabled.value) {
      startLoopRecording() // awaiting_input → user_turn
      return
    }

    if (!agentDone) return // Agent still streaming — wait for finishStreaming
    tryCompleteTurn()
  })

  // Watch voice toggle — clean up when voice is turned off
  watch(() => voiceEnabled.value, (enabled) => {
    if (!enabled) {
      stopMonitoring()
    }
  })

  // ─── WebSocket disconnect/reconnect ──────────────────────────────
  // Audio cues (warning tone, reconnect chime, TTS announcements) are
  // handled by the orchestrator's connection state watcher.
  // Here we only re-fetch session/messages on reconnect.
  let hadConnection = false
  watch(connectionState, (newState, oldState) => {
    if (newState === 'connected' && !hadConnection) {
      hadConnection = true
      return
    }
    if (newState === 'connected' && oldState !== 'connected' && hadConnection) {
      log('info', 'ws', 'reconnected_agent')
      fetchSession()
      fetchMessages({ force: true })
    }
  })

  // Fetch session details via REST
  async function fetchSession() {
    try {
      const data = await $fetch<Session & { busy?: boolean }>(`${apiBase}/sessions/${sessionId}`)
      const isBusy = data.busy === true
      // Remove the busy field before storing — it's transient, not part of Session
      delete (data as unknown as Record<string, unknown>).busy
      session.value = data

      // If the backend reports the session as busy, sync the frontend state.
      // This handles page reload / WebSocket reconnect where the idle event
      // was missed — the frontend enters agent:streaming and waits for the
      // real done/idle event from SSE.
      if (isBusy) {
        log('info', 'agent', 'session_busy_on_load', { sessionId })
        isStreaming.value = true
        dispatch('start_agent_turn', 'session_busy_on_load')
        // Working hum is started by the orchestrator via onTransition
      }
    } catch {
      console.error('Failed to fetch session')
    }
  }

  // Fetch existing message history from the backend
  async function fetchMessages(options?: { force?: boolean }) {
    try {
      interface HistoryMessage {
        id: string
        role: 'user' | 'assistant'
        content: string
        timestamp: number
        type?: string
        meta?: Record<string, unknown>
      }
      const data = await $fetch<HistoryMessage[]>(`${apiBase}/sessions/${sessionId}/messages`)
      if (data && data.length > 0) {
        // Only load history if we don't already have messages (avoid duplicates on reconnect)
        // Unless force is set (e.g., after backend restart)
        if (messages.value.length === 0 || options?.force) {
          msgBuilder.setMessages(data
            .filter(m => !m.meta?.hidden)
            .map(m => ({
              id: m.id,
              role: m.role,
              content: m.content,
              timestamp: m.timestamp,
              type: m.type as AgentEvent['type'] | undefined,
              meta: m.meta,
            })))

          // Speak the welcome message on first session load
          if (shouldSpeakWelcome(voiceEnabled.value, data)) {
            const spoken = ttsCondenser.condense(data[0].content)
            if (spoken) {
              enqueueTTS(spoken)
            }
          }
        }
      }
    } catch {
      console.error('Failed to fetch message history')
    }
  }

  // Handle incoming WebSocket messages
  const unsubscribe = subscribe((msg) => {
    // Only handle events for this session
    if (msg.sessionId && msg.sessionId !== sessionId) return

    switch (msg.type) {
      case 'event':
        if (msg.event) {
          handleAgentEvent(msg.event)
        }
        break
      case 'command':
        handleCommand(msg.content || '')
        break
      case 'error':
        appendSystemMessage(`Error: ${msg.content || 'Unknown error'}`)
        break
    }
  })

  function handleAgentEvent(event: AgentEvent) {
    // Cancel watchdog on any agent event — confirms the agent is alive
    cancelWatchdog()
    log('debug', 'agent', 'event_received', { type: event.type, partId: event.partId, hasDelta: !!event.delta })

    // Skip events for messages marked as hidden or visual-only
    const eventDisplay = event.display || 'default'

    // Feed non-text events through the tool batcher — it batches consecutive
    // tool_use events into a single TTS summary and passes other events
    // (error, code, etc.) through filterForTTS as before.
    // Exclude permission events, question events, control events, and
    // error events during aborted turns from the batcher.
    if (voiceEnabled.value
      && eventDisplay === 'default'
      && event.type !== 'text'
      && event.type !== 'done'
      && event.type !== 'status'
      && event.type !== 'permission_request'
      && event.type !== 'permission_replied'
      && event.type !== 'question_request'
      && event.type !== 'question_replied'
      && !(event.type === 'error' && abortedTurn)) {
      ttsToolBatcher.push(event)
    }

    switch (event.type) {
      case 'text':
        handleTextEvent(event)
        break
      case 'tool_use':
        // Record start time for duration tracking
        if (event.partId) {
          toolStartTimes.set(event.partId, Date.now())
        }
        // Notify audio feedback that tools are active (for spoken check-in context)
        notifyToolActivity()
        appendAssistantMeta(event.content, 'tool_use', event.meta)
        break
      case 'tool_result': {
        // Compute duration from matching tool_use event
        const resultMeta = { ...event.meta }
        if (event.partId && toolStartTimes.has(event.partId)) {
          const startTime = toolStartTimes.get(event.partId)!
          resultMeta.durationMs = Date.now() - startTime
          toolStartTimes.delete(event.partId)
        }
        appendAssistantMeta(event.content, 'tool_result', resultMeta)
        break
      }
      case 'thinking':
        // Reasoning content is filtered out by the backend and never
        // forwarded to the frontend.  This case is kept as a no-op
        // safety net in case future backend changes re-introduce it.
        break
      case 'error': {
        // Abort errors (e.g. MessageAbortedError) are expected after the
        // user hits stop — don't play error tones or speak the raw error.
        const isAbortError = abortedTurn
          || (event.content && event.content.includes('Aborted'));
        log(isAbortError ? 'debug' : 'error', 'agent', 'error_event', { content: event.content, isAbortError })
        if (!isAbortError) {
          stopWorkingHum().then(() => playErrorTone())
          appendSystemMessage(`Error: ${event.content}`)
        }
        break
      }
      case 'done':
      case 'status':
        if (event.type === 'done' || event.content === 'idle') {
          finishStreaming()
        } else if (event.content === 'busy') {
          isStreaming.value = true
        }
        break
      case 'session_updated':
        // Update session agent, model, title, or mode if changed
        if (session.value && event.meta?.agent) {
          session.value.agent = event.meta.agent as string
        } else if (session.value && event.meta?.model !== undefined) {
          session.value.model = event.meta.model as string
        } else if (session.value && event.meta?.lastUsedModel) {
          session.value.lastUsedModel = event.meta.lastUsedModel as string
        } else if (session.value && event.content) {
          // Only apply OpenCode's auto-title if no manual override exists
          if (!session.value.titleOverride) {
            session.value.title = event.content
          }
        }
        break
      case 'permission_request':
        handlePermissionRequest(event)
        break
      case 'permission_replied':
        handlePermissionReplied(event)
        break
      case 'question_request':
        handleQuestionRequest(event)
        break
      case 'question_replied':
        handleQuestionReplied(event)
        break
    }
  }

  function handleTextEvent(event: AgentEvent) {
    isStreaming.value = true

    // Auto-voice for new sessions: on the first streaming event, if we have
    // auto-voice enabled, treat this as a voice-initiated turn so the mic
    // auto-starts after the greeting TTS finishes. The state may already be
    // agent_turn (from session_busy_on_load or initial_tools), so only
    // dispatch start_agent_turn when still in idle.
    if (autoVoiceArmed) {
      autoVoiceArmed = false
      voiceInitiatedTurn.value = true
      if (getState() === 'idle') {
        dispatch('start_agent_turn', 'auto_voice_initial')
      }
    }

    // Flush any pending tool-use batch before speaking text
    if (voiceEnabled.value) {
      ttsToolBatcher.flush()
    }

    // Use delta-based streaming if available
    const partId = event.partId || 'default'

    // Mark first text token arrival (agent TTFT) — only during voice round-trips
    if (!msgBuilder.hasActiveAssistant() && isTimerActive()) {
      mark('agent_ttft', 'end')
      log('info', 'agent', 'first_text_token', { partId })
    }

    if (event.delta) {
      // Accumulate delta into the message builder
      msgBuilder.addTextDelta(partId, event.delta, event.display)
      // Feed delta to TTS chunker — it will enqueue sentence-sized chunks as they complete
      if (voiceEnabled.value && (event.display || 'default') === 'default') {
        ttsChunker.push(event.delta)
      }
    } else if (event.content) {
      // Full content replacement (final snapshot from OpenCode)
      msgBuilder.setTextContent(partId, event.content, event.display)
      // Don't re-send to TTS — deltas already covered this content
    }
  }

  function appendAssistantMeta(content: string, type: AgentEvent['type'], meta?: Record<string, unknown>) {
    const didSplit = msgBuilder.appendMeta(content, type, meta)
    // If a text bubble was split, flush the TTS chunker so the pre-tool
    // text is spoken before any tool summary.
    if (didSplit && voiceEnabled.value) {
      ttsChunker.flush()
    }
  }

  function appendSystemMessage(content: string) {
    msgBuilder.addSystemMessage(content)
  }

  function finishStreaming() {
    // Guard against duplicate done/idle events
    if (agentDone) {
      log('debug', 'agent', 'finish_streaming_duplicate')
      return
    }
    agentDone = true
    log('info', 'agent', 'finish_streaming', { abortedTurn })

    // Mark agent completion — only during voice round-trips
    if (isTimerActive()) {
      mark('agent_full', 'end')
    }

    // Audio cues (stop hum, success chime) are fired by the orchestrator
    // when dispatch('complete_turn') transitions agent_turn → idle.
    abortedTurn = false

    // Flush any remaining buffered text to TTS.
    // Tool batcher is flushed silently at end-of-turn — tool summaries are
    // only spoken when followed by agent text (see useTTSToolBatcher).
    // Then reset the batcher so the next turn starts fresh.
    if (voiceEnabled.value) {
      ttsToolBatcher.flushSilent()
      ttsChunker.flush()
    }
    ttsToolBatcher.reset()
    ttsCondenser.reset()
    isStreaming.value = false
    msgBuilder.reset()
    toolStartTimes.clear()

    // If TTS has nothing left to play, complete the turn immediately.
    // Otherwise the onQueueDrained callback will handle it.
    if (!isTTSPlaying.value) {
      tryCompleteTurn()
    }
  }

  // Complete the turn: dispatch state transition, optionally restart voice loop.
  function tryCompleteTurn() {
    dispatch('complete_turn', 'turn_complete') // agent_turn → idle
    agentDone = false
    startLoopRecording() // idle → user_turn (if guards pass)
  }

  // Send a message via WebSocket
  function sendMessage(text: string, options?: { origin?: 'voice' | 'text' }) {
    log('info', 'agent', 'send_message', { origin: options?.origin, length: text.length, hasPendingQuestion: hasPendingQuestion.value })
    abortedTurn = false
    autoVoiceArmed = false // User sent a message — no longer first response
    // If there's a pending question, intercept the input as a custom answer
    if (hasPendingQuestion.value && tryAnswerPendingQuestion(text)) {
      // Preserve voice-initiated state so the mic loop continues for
      // subsequent questions in a batch (the early return below would
      // otherwise skip the voiceInitiatedTurn assignment).
      if (options?.origin === 'voice') {
        voiceInitiatedTurn.value = true
      }
      // Show the custom answer as a user message in the chat
      msgBuilder.addUserMessage(text)
      return
    }

    // Track whether this turn was started by voice so the mic auto-loop
    // only re-activates after voice-initiated turns (not after typing).
    voiceInitiatedTurn.value = options?.origin === 'voice'

    // Only mark agent timing if this is part of a voice round-trip
    if (isTimerActive()) {
      mark('agent_ttft', 'start')
      mark('agent_full', 'start')
    }

    // Suppress stop blip for voice-originated messages to prevent
    // double-beep (stop blip + handoff tone overlap).
    if (voiceEnabled.value && options?.origin === 'voice') {
      suppressNextStopBlip()
    }
    // Audio cues (handoff, hum, watchdog) are fired by the orchestrator
    // when dispatch('start_agent_turn', 'send_message') transitions.

    // Add user message immediately
    msgBuilder.addUserMessage(text)

    // Transition to agent_turn — the state machine tracks that
    // we've handed the message to the agent and are waiting for a response.
    dispatch('start_agent_turn', 'send_message')
    agentDone = false

    const sent = send({
      type: 'message',
      sessionId,
      content: text,
    })

    if (!sent) {
      log('error', 'agent', 'send_message_failed')
      dispatch('error', 'send_message_failed')
      appendSystemMessage('Failed to send message: not connected to backend')
    }
  }

  // Abort the current session (guarded against repeated calls)
  let abortSentAt = 0
  function abortSession() {
    const now = Date.now()
    if (now - abortSentAt < 2000) return // Debounce repeated taps
    abortSentAt = now
    log('info', 'agent', 'abort_session')
    abortedTurn = true
    agentDone = false
    abort('abort_session')
    // Audio feedback (stop hum, cancel tone, TTS "Stopped.") is handled
    // by the orchestrator's abort transition handler.
    // stopTTS is also called by the orchestrator — it stops current playback
    // then enqueues "Stopped." announcement.
    stopMonitoring() // Stop mic monitoring if active
    ttsChunker.reset()
    ttsCondenser.reset()
    ttsToolBatcher.reset()

    send({
      type: 'abort',
      sessionId,
    })
  }

  // Stop TTS playback and silence the voice pipeline (monitoring + recording)
  // so residual mic input doesn't get transcribed and looped back.
  function stopPlayback() {
    log('info', 'agent', 'stop_playback')
    stopTTS()
    forceStopRecording()
    stopMonitoring()
  }

  // Toggle voice mode (TTS for agent responses)
  function toggleVoice() {
    voiceEnabled.value = !voiceEnabled.value
    if (!voiceEnabled.value) {
      // TTS stop is handled by orchestrator's voiceEnabled watcher
      stopMonitoring()
    }
  }

  // Update the session title locally (after a successful backend PATCH).
  function setTitle(title: string, override: boolean) {
    if (!session.value) return
    session.value.title = title
    session.value.titleOverride = override
  }

  // Switch the active agent for this session
  function setAgent(agentName: string) {
    if (!session.value) return
    if (session.value.agent === agentName) return
    const sent = send({
      type: 'set_agent',
      sessionId,
      content: agentName,
    })
    if (sent) {
      session.value.agent = agentName
    } else {
      appendSystemMessage('Failed to switch agent: not connected to backend')
    }
  }

  // Switch the active model override for this session
  function setModel(modelID: string) {
    if (!session.value) return
    const current = session.value.model || ''
    if (current === modelID) return
    const sent = send({
      type: 'set_model',
      sessionId,
      content: modelID,
    })
    if (sent) {
      session.value.model = modelID
    } else {
      appendSystemMessage('Failed to switch model: not connected to backend')
    }
  }

  function handleCommand(command: string) {
    switch (command) {
      case 'stop':
        abortSession()
        break
    }
  }

  // ─── Permission Handling ──────────────────────────────────────────

  function handlePermissionRequest(event: AgentEvent) {
    const permissionId = event.meta?.permissionId as string | undefined
    const title = event.meta?.title as string || event.content || 'Permission needed'
    log('info', 'permission', 'request_received', { permissionId, title })

    // Add permission request as a special message in the chat
    msgBuilder.addSystemMessage(title, 'permission_request', {
      permissionId,
      permissionType: event.meta?.permissionType,
      pattern: event.meta?.pattern,
      title,
      resolved: false,
    })

    // Announce via TTS (chime + hum stop handled by orchestrator on await_input transition)
    if (voiceEnabled.value) {
      ttsToolBatcher.flush()
      enqueueTTS(`Permission needed: ${title}`)
    }

    // Transition to awaiting_input — user must tap approve/reject
    if (getState() === 'agent_turn') {
      dispatch('await_input', 'permission_request')
    }
  }

  function handlePermissionReplied(event: AgentEvent) {
    const permissionId = event.meta?.permissionId as string | undefined
    const response = event.meta?.response as string || event.content

    if (!permissionId) return

    // Find the matching permission_request message and mark it resolved
    const msg = messages.value.find(
      m => m.type === 'permission_request' && m.meta?.permissionId === permissionId,
    )
    if (msg && msg.meta) {
      msg.meta.resolved = true
      msg.meta.resolvedResponse = response
    }

    // Brief TTS announcement
    if (voiceEnabled.value) {
      const label = response === 'reject' ? 'Permission denied' : 'Permission approved'
      enqueueTTS(label)
    }
  }

  // Respond to a pending permission prompt via WebSocket
  function respondToPermission(permissionId: string, response: 'once' | 'always' | 'reject') {
    log('info', 'permission', 'respond', { permissionId, response })
    const sent = send({
      type: 'permission_response',
      sessionId,
      permissionId,
      response,
      remember: response === 'always',
    })

    if (sent) {
      // Optimistically mark the message as resolved
      const msg = messages.value.find(
        m => m.type === 'permission_request' && m.meta?.permissionId === permissionId,
      )
      if (msg && msg.meta) {
        msg.meta.resolved = true
        msg.meta.resolvedResponse = response
      }
      // Transition back to agent_turn — agent resumes after permission response
      if (getState() === 'awaiting_input') {
        dispatch('answer_input', 'permission_response')
      }
    } else {
      appendSystemMessage('Failed to respond to permission: not connected to backend')
    }
  }

  // Provide respondToPermission so ChatMessage can access it via inject()
  provide(RespondToPermissionKey, respondToPermission)

  // ─── Question Handling ────────────────────────────────────────────

  /**
   * Speak a question and its options via TTS.
   * Reused both on initial arrival (first question) and after answering
   * the previous question in a multi-question batch.
   */
  function announceQuestion(questionText: string, options: Array<{ label: string; description: string }>) {
    let ttsText = questionText
    if (options.length > 0) {
      const optLabels = options.map(o => o.label)
      const lastLabel = optLabels.pop()
      ttsText += `. ${optLabels.length > 0 ? optLabels.join(', ') + ', or ' : ''}${lastLabel}.`
    }
    enqueueTTS(ttsText)
  }

  function handleQuestionRequest(event: AgentEvent) {
    const questionId = event.meta?.questionId as string | undefined
    const questionIndex = event.meta?.questionIndex as number ?? 0
    const totalQuestions = event.meta?.totalQuestions as number ?? 1
    const header = event.meta?.header as string || ''
    const options = event.meta?.options as Array<{ label: string; description: string }> || []
    const multiple = event.meta?.multiple as boolean || false
    log('info', 'question', 'request_received', { questionId, questionIndex, totalQuestions, optionCount: options.length, multiple })

    // Break the current assistant message so that any text the agent sends
    // after the question is answered appears as a new chat bubble.
    // (addSystemMessage splits automatically)

    // Initialize answer tracking for this question batch
    if (questionId && !pendingQuestionAnswers.has(questionId)) {
      pendingQuestionAnswers.set(questionId, {
        totalQuestions,
        answers: new Map(),
      })
    }

    // Add question as a special message in the chat
    msgBuilder.addSystemMessage(event.content || header || 'Question', 'question_request', {
      questionId,
      questionIndex,
      totalQuestions,
      header,
      options,
      multiple,
      resolved: false,
    })

    // TTS: Flush any buffered text from the agent's preamble so it
    // speaks in correct order before the question announcement.
    // TTS announcement (chime + hum stop handled by orchestrator on await_input transition)
    if (voiceEnabled.value && questionIndex === 0) {
      ttsToolBatcher.flush()
      ttsChunker.flush()
      if (totalQuestions > 1) {
        enqueueTTS(`I have ${totalQuestions} questions for you.`)
      }
      announceQuestion(event.content || header, options)
    }

    // Transition to awaiting_input so the voice loop can restart
    // after TTS finishes announcing the question.
    // Guard: only dispatch from agent_turn (multi-question bursts
    // arrive in rapid succession — only the first triggers the transition).
    if (getState() === 'agent_turn') {
      dispatch('await_input', 'question_request')
    }
  }

  function handleQuestionReplied(event: AgentEvent) {
    const questionId = event.meta?.questionId as string | undefined
    const rejected = event.meta?.rejected as boolean || false

    if (!questionId) return

    // Mark all question messages for this batch as resolved
    messages.value
      .filter(m => m.type === 'question_request' && m.meta?.questionId === questionId)
      .forEach((msg) => {
        if (msg.meta) {
          msg.meta.resolved = true
          msg.meta.rejected = rejected
          if (!rejected && event.meta?.answers) {
            const answers = event.meta.answers as string[][]
            const idx = msg.meta.questionIndex as number
            if (answers[idx]) {
              msg.meta.selectedLabels = answers[idx]
            }
          }
        }
      })

    // Clean up answer tracking
    pendingQuestionAnswers.delete(questionId)
  }

  /**
   * Respond to a single question within a batch.
   * Tracks partial answers; sends the full answers array to the backend
   * only when all questions in the batch have been answered.
   */
  function respondToQuestion(questionId: string, questionIndex: number, selectedLabels: string[]) {
    log('info', 'question', 'respond', { questionId, questionIndex, selectedLabels })
    const batch = pendingQuestionAnswers.get(questionId)
    if (!batch) return

    // Store this answer
    batch.answers.set(questionIndex, selectedLabels)

    // Optimistically mark this question as resolved
    const msg = messages.value.find(
      m => m.type === 'question_request'
        && m.meta?.questionId === questionId
        && m.meta?.questionIndex === questionIndex,
    )
    if (msg && msg.meta) {
      msg.meta.resolved = true
      msg.meta.selectedLabels = selectedLabels
    }

    // Check if all questions in the batch have been answered
    if (batch.answers.size >= batch.totalQuestions) {
      // Assemble the answers array in order
      const answers: string[][] = []
      for (let i = 0; i < batch.totalQuestions; i++) {
        answers.push(batch.answers.get(i) || [])
      }

      const sent = send({
        type: 'question_response',
        sessionId,
        questionId,
        answers,
      })

      if (!sent) {
        appendSystemMessage('Failed to respond to question: not connected to backend')
      }

      pendingQuestionAnswers.delete(questionId)
    } else if (voiceEnabled.value) {
      // More questions remain — announce the next unanswered one via TTS
      const nextMsg = messages.value.find(
        m => m.type === 'question_request'
          && m.meta?.questionId === questionId
          && m.meta?.resolved !== true,
      )
      if (nextMsg && nextMsg.meta) {
        const opts = (nextMsg.meta.options as Array<{ label: string; description: string }>) || []
        announceQuestion(nextMsg.content, opts)
      }
    }

    // State transitions: move back to agent_turn after answering
    const currentState = getState()
    if (currentState === 'awaiting_input') {
      dispatch('answer_input', 'question_answered')
    } else if (currentState === 'user_turn') {
      dispatch('start_agent_turn', 'question_answered')
    }
    // If more questions remain, go back to awaiting_input for next question
    if (hasPendingQuestion.value && getState() === 'agent_turn') {
      dispatch('await_input', 'next_question')
    }
  }

  function rejectQuestion(questionId: string) {
    log('info', 'question', 'reject', { questionId })
    const sent = send({
      type: 'question_reject',
      sessionId,
      questionId,
    })

    if (sent) {
      // Optimistically mark all questions in this batch as rejected
      messages.value
        .filter(m => m.type === 'question_request' && m.meta?.questionId === questionId)
        .forEach((msg) => {
          if (msg.meta) {
            msg.meta.resolved = true
            msg.meta.rejected = true
          }
        })
      pendingQuestionAnswers.delete(questionId)

      // Transition back to agent_turn — agent resumes after rejection
      if (getState() === 'awaiting_input') {
        dispatch('answer_input', 'question_rejected')
      } else if (getState() === 'user_turn') {
        dispatch('start_agent_turn', 'question_rejected')
      }
    } else {
      appendSystemMessage('Failed to reject question: not connected to backend')
    }
  }

  /**
   * Handle a custom answer submitted via the chat input (text or voice).
   * Returns true if the input was consumed as a question answer, false if
   * it should be sent as a regular agent message.
   */
  function tryAnswerPendingQuestion(text: string): boolean {
    // Find the first unanswered question
    const pendingMsg = messages.value.find(
      m => m.type === 'question_request' && m.meta?.resolved !== true,
    )
    if (!pendingMsg || !pendingMsg.meta) return false

    const questionId = pendingMsg.meta.questionId as string
    const questionIndex = pendingMsg.meta.questionIndex as number

    // Use the text as a custom answer (single selection)
    respondToQuestion(questionId, questionIndex, [text.trim()])
    return true
  }

  provide(RespondToQuestionKey, respondToQuestion)
  provide(RejectQuestionKey, rejectQuestion)

  // The currently active question: first unanswered question_request message.
  // ChatMessage uses this to decide whether to show interactive buttons or a
  // dimmed "waiting" state for not-yet-active questions in a multi-question batch.
  const activeQuestion = computed<string | null>(() => {
    const pending = messages.value.find(
      m => m.type === 'question_request' && m.meta?.resolved !== true,
    )
    if (!pending || !pending.meta) return null
    return `${pending.meta.questionId}:${pending.meta.questionIndex}`
  })
  provide(ActiveQuestionKey, activeQuestion)

  // ─── Session Leave Cleanup ────────────────────────────────────────
  //
  // Full teardown of all resources when leaving a session page.
  // Stops TTS, mic, hum, timers, and resets the state machine.
  // Optionally sends an abort to the backend.

  function cleanup(options?: { abortBackend?: boolean }) {
    log('info', 'agent', 'cleanup', { abortBackend: options?.abortBackend })

    // Stop TTS playback and clear queue
    stopTTS()

    // Stop any active recording (discard audio)
    forceStopRecording()

    // Stop mic monitoring and release mic stream
    stopMonitoring()

    // Stop all audio feedback (hum, watchdog, check-in timers)
    stopAllAudioFeedback()

    // Reset TTS pipeline state
    ttsChunker.reset()
    ttsCondenser.reset()
    ttsToolBatcher.reset()

    // Reset voice loop state
    loopRecordingActive.value = false
    loopStartPending = false
    voiceInitiatedTurn.value = false

    // Reset streaming state
    isStreaming.value = false
    agentDone = false
    abortedTurn = false
    msgBuilder.reset()
    toolStartTimes.clear()

    // Clean up orchestrator BEFORE abort to prevent it from playing
    // cancel tone / "Stopped." during session leave cleanup.
    orchestrator.cleanup()

    // Force state machine back to idle
    abort('session_leave')

    // Optionally abort the agent on the backend
    if (options?.abortBackend) {
      send({
        type: 'abort',
        sessionId,
      })
    }

    // Remove auto-stop handler
    unsubAutoStop()

    // Unsubscribe from WebSocket messages
    unsubscribe()
  }

  // Initialize — isLoading stays true until both session metadata and message
  // history are loaded. If fetchSession detects busy, isStreaming is set before
  // isLoading clears, so the loader stays continuous.
  Promise.all([fetchSession(), fetchMessages()]).finally(() => {
    isLoading.value = false
  })

  // Cleanup on scope dispose (component unmount without explicit cleanup)
  onScopeDispose(() => {
    cleanup()
  })

  return {
    session: readonly(session),
    messages,
    isStreaming: readonly(isStreaming),
    isLoading: readonly(isLoading),
    hasPendingPermission,
    hasPendingQuestion,
    isTTSPlaying,
    isRecording,
    isMonitoring,
    voiceEnabled: readonly(voiceEnabled),
    loopRecordingActive: readonly(loopRecordingActive),
    connectionState,
    sendMessage,
    abortSession,
    stopPlayback,
    setTitle,
    setAgent,
    setModel,
    toggleVoice,
    respondToPermission,
    respondToQuestion,
    rejectQuestion,
    cleanup,
  }
}

/**
 * Determines whether the welcome message should be spoken via TTS.
 * Only speaks on the very first load of a fresh session (no user messages yet).
 */
export function shouldSpeakWelcome(
  voiceEnabled: boolean,
  messages: { id: string; role: string }[],
): boolean {
  return voiceEnabled
    && messages.length > 0
    && !!messages[0].id?.startsWith('welcome-')
    && !messages.some(m => m.role === 'user');
}
