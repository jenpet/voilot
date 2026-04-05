import type { Session } from './useSession'
import type { AgentEvent } from './useWebSocket'
import type { InjectionKey } from 'vue'
import { useTTSChunker } from './useTTSChunker'
import { useTTSCondenser } from './useTTSCondenser'
import { useTTSToolBatcher } from './useTTSToolBatcher'
import { suppressNextStopBlip } from './useRecordingFeedback'
import {
  playHandoff,
  startWorkingHum,
  stopWorkingHum,
  playSuccessChime,
  playQuestionChime,
  playPermissionChime,
  playErrorTone,
  playCancelTone,
  playLoopListeningTick,
  playSTTFailureTone,
  playWarningTone,
  playReconnectChime,
  playModeSignature,
  notifyToolActivity,
  startWatchdog,
  cancelWatchdog,
  setTTSEnqueue,
} from './useAudioFeedback'

export interface Message {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp: number
  type?: AgentEvent['type']
  meta?: Record<string, unknown>
}

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

let messageIdCounter = 0
function nextMessageId(): string {
  return `msg-${Date.now()}-${++messageIdCounter}`
}

export function useAgent(sessionId: string) {
  const config = useRuntimeConfig()
  const apiBase = `${config.public.backendUrl}/api`
  const { send, subscribe, connectionState } = useWebSocket()
  const { enqueue: enqueueTTS, stop: stopTTS, isPlaying: isTTSPlaying } = useTTS()

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
    setOnAutoStop,
  } = useVoice()

  const session = ref<Session | null>(null)
  const messages = ref<Message[]>([])
  const isStreaming = ref(false)
  const voiceEnabled = ref(true)

  // Track whether the current turn was aborted by the user.
  // Prevents finishStreaming() from playing a success chime after abort.
  let abortedTurn = false

  // Track whether the current conversation turn was started by voice input.
  // When false (text-typed input), the mic auto-loop does NOT re-activate.
  const voiceInitiatedTurn = ref(false)

  // Derived: true when any permission_request message is unresolved
  const hasPendingPermission = computed(() =>
    messages.value.some(m => m.type === 'permission_request' && m.meta?.resolved !== true),
  )
  // Derived: true when any question_request message is unresolved
  const hasPendingQuestion = computed(() =>
    messages.value.some(m => m.type === 'question_request' && m.meta?.resolved !== true),
  )
  // Track partial answers for multi-question batches.
  // Key: questionId, Value: { totalQuestions, answers: Map<index, string[]> }
  const pendingQuestionAnswers = new Map<string, { totalQuestions: number; answers: Map<number, string[]> }>()
  // Track the current assistant message being streamed
  let currentAssistantId: string | null = null
  // Track which partId maps to which text so far (for delta accumulation)
  const partContents = new Map<string, string>()
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
  function startLoopRecording() {
    if (!voiceEnabled.value || !voiceInitiatedTurn.value || isRecording.value || isTTSPlaying.value || loopStartPending) {
      return
    }
    // Don't auto-record while waiting for user to respond to a permission prompt
    if (hasPendingPermission.value) return
    // When a question is pending, allow recording (user answers by voice).
    // Otherwise, block if the agent is still streaming.
    if (!hasPendingQuestion.value && isStreaming.value) return
    loopStartPending = true
    loopRecordingActive.value = true
    // Subtle tick so the user knows the mic is hot (Phase 3)
    playLoopListeningTick()
    startRecording() // reuses existing mic stream via ensureMicAndAnalyser()
  }

  // Handle auto-stop from silence detection — only for loop-triggered recordings.
  // Manual VoiceButton recordings are handled by VoiceButton's own auto-stop handler.
  setOnAutoStop(async () => {
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
      playSTTFailureTone()
      startLoopRecording()
    }
  })

  // When TTS finishes and the agent is done streaming, start recording
  // for the user's next turn. Also triggers when TTS finishes announcing
  // a question (isStreaming is still true but hasPendingQuestion allows it).
  watch(isTTSPlaying, (playing) => {
    if (!playing && (!isStreaming.value || hasPendingQuestion.value)) {
      startLoopRecording()
    }
  })

  // Watch voice toggle — clean up when voice is turned off
  watch(() => voiceEnabled.value, (enabled) => {
    if (!enabled) {
      stopMonitoring()
    }
  })

  // ─── WebSocket disconnect/reconnect audio (Phase 5a) ────────────
  watch(connectionState, (newState, oldState) => {
    if (!voiceEnabled.value) return
    if (newState === 'disconnected' && oldState === 'connected') {
      stopWorkingHum().then(() => {
        playWarningTone()
        enqueueTTS('Connection lost.')
      })
    } else if (newState === 'connected' && oldState !== 'connected') {
      playReconnectChime()
      enqueueTTS('Reconnected.')
    }
  })

  // Fetch session details via REST
  async function fetchSession() {
    try {
      const data = await $fetch<Session>(`${apiBase}/sessions/${sessionId}`)
      session.value = data
    } catch {
      console.error('Failed to fetch session')
    }
  }

  // Fetch existing message history from the backend
  async function fetchMessages() {
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
        if (messages.value.length === 0) {
          messages.value = data.map(m => ({
            id: m.id,
            role: m.role,
            content: m.content,
            timestamp: m.timestamp,
            type: m.type as AgentEvent['type'] | undefined,
            meta: m.meta,
          }))
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

    // Feed non-text events through the tool batcher — it batches consecutive
    // tool_use events into a single TTS summary and passes other events
    // (error, code, etc.) through filterForTTS as before.
    // Exclude permission events, question events, control events, and
    // error events during aborted turns from the batcher.
    if (voiceEnabled.value
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
        // Show thinking indicator but don't add to message content
        isStreaming.value = true
        break
      case 'error': {
        // Abort errors (e.g. MessageAbortedError) are expected after the
        // user hits stop — don't play error tones or speak the raw error.
        const isAbortError = abortedTurn
          || (event.content && event.content.includes('Aborted'));
        if (!isAbortError) {
          stopWorkingHum().then(() => playErrorTone())
          appendSystemMessage(`Error: ${event.content}`)
        }
        break
      }
      case 'done':
        finishStreaming()
        break
      case 'status':
        if (event.content === 'busy') {
          isStreaming.value = true
        } else if (event.content === 'idle') {
          finishStreaming()
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
        } else if (session.value && event.meta?.mode) {
          session.value.mode = event.meta.mode as 'plan' | 'implement'
          // Audio cue for mode switch (Phase 5b)
          if (voiceEnabled.value) {
            playModeSignature()
          }
        } else if (session.value && event.content) {
          session.value.title = event.content
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

    // Flush any pending tool-use batch before speaking text
    if (voiceEnabled.value) {
      ttsToolBatcher.flush()
    }

    // Use delta-based streaming if available
    const partId = event.partId || 'default'

    // Mark first text token arrival (agent TTFT) — only during voice round-trips
    if (!currentAssistantId && isTimerActive()) {
      mark('agent_ttft', 'end')
    }

    if (event.delta) {
      // Accumulate delta into part content
      const existing = partContents.get(partId) || ''
      const updated = existing + event.delta
      partContents.set(partId, updated)
      // Feed delta to TTS chunker — it will enqueue sentence-sized chunks as they complete
      if (voiceEnabled.value) {
        ttsChunker.push(event.delta)
      }
    } else if (event.content) {
      // Full content replacement (final snapshot from OpenCode)
      partContents.set(partId, event.content)
      // Don't re-send to TTS — deltas already covered this content
    }

    // Rebuild the full assistant message from all parts
    const fullContent = Array.from(partContents.values()).join('')

    if (!currentAssistantId) {
      // Create new assistant message
      currentAssistantId = nextMessageId()
      messages.value.push({
        id: currentAssistantId,
        role: 'assistant',
        content: fullContent,
        timestamp: Date.now(),
      })
    } else {
      // Update existing assistant message
      const msg = messages.value.find(m => m.id === currentAssistantId)
      if (msg) {
        msg.content = fullContent
      }
    }
  }

  function appendAssistantMeta(content: string, type: AgentEvent['type'], meta?: Record<string, unknown>) {
    messages.value.push({
      id: nextMessageId(),
      role: 'assistant',
      content: content || '',
      timestamp: Date.now(),
      type,
      meta,
    })
  }

  function appendSystemMessage(content: string) {
    messages.value.push({
      id: nextMessageId(),
      role: 'system',
      content,
      timestamp: Date.now(),
    })
  }

  function finishStreaming() {
    // Mark agent completion — only during voice round-trips
    if (isTimerActive()) {
      mark('agent_full', 'end')
    }

    // Stop working hum and play success chime when agent finishes.
    // Skip the chime if this turn was aborted — abortSession() already
    // played the cancel tone, so a success chime would be confusing.
    if (voiceEnabled.value) {
      if (abortedTurn) {
        stopWorkingHum()
      } else {
        stopWorkingHum().then(() => playSuccessChime())
      }
    }
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
    currentAssistantId = null
    partContents.clear()
    toolStartTimes.clear()

    // If voice is enabled and TTS has nothing left to play, start recording
    // for the user's next turn immediately.
    // (If TTS is still playing, the isTTSPlaying watcher will start recording
    // when playback finishes.)
    startLoopRecording()
  }

  // Send a message via WebSocket
  function sendMessage(text: string, options?: { origin?: 'voice' | 'text' }) {
    abortedTurn = false
    // If there's a pending question, intercept the input as a custom answer
    if (hasPendingQuestion.value && tryAnswerPendingQuestion(text)) {
      // Preserve voice-initiated state so the mic loop continues for
      // subsequent questions in a batch (the early return below would
      // otherwise skip the voiceInitiatedTurn assignment).
      if (options?.origin === 'voice') {
        voiceInitiatedTurn.value = true
      }
      // Show the custom answer as a user message in the chat
      messages.value.push({
        id: nextMessageId(),
        role: 'user',
        content: text,
        timestamp: Date.now(),
      })
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

    // Play handoff tone + start working hum so the user knows
    // the agent received the message and is working.
    // Suppress stop blip for voice-originated messages to prevent
    // double-beep (stop blip + handoff tone overlap).
    if (voiceEnabled.value) {
      if (options?.origin === 'voice') {
        suppressNextStopBlip()
      }
      playHandoff()
      startWorkingHum()
      startWatchdog()
    }

    // Add user message immediately
    messages.value.push({
      id: nextMessageId(),
      role: 'user',
      content: text,
      timestamp: Date.now(),
    })

    const sent = send({
      type: 'message',
      sessionId,
      content: text,
    })

    if (!sent) {
      appendSystemMessage('Failed to send message: not connected to backend')
    }
  }

  // Abort the current session
  function abortSession() {
    abortedTurn = true
    stopTTS() // Stop any ongoing TTS playback
    stopMonitoring() // Stop mic monitoring if active
    ttsChunker.reset()
    ttsCondenser.reset()
    ttsToolBatcher.reset()

    // Audio feedback: stop hum, play cancel tone, announce "Stopped."
    if (voiceEnabled.value) {
      stopWorkingHum().then(() => {
        playCancelTone()
        enqueueTTS('Stopped.')
      })
    }

    send({
      type: 'abort',
      sessionId,
    })
  }

  // Toggle voice mode (TTS for agent responses)
  function toggleVoice() {
    voiceEnabled.value = !voiceEnabled.value
    if (!voiceEnabled.value) {
      stopTTS()
      stopMonitoring()
    }
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

    // Add permission request as a special message in the chat
    messages.value.push({
      id: nextMessageId(),
      role: 'system',
      content: title,
      timestamp: Date.now(),
      type: 'permission_request',
      meta: {
        permissionId,
        permissionType: event.meta?.permissionType,
        pattern: event.meta?.pattern,
        title,
        resolved: false,
      },
    })

    // Announce via TTS with permission chime
    if (voiceEnabled.value) {
      ttsToolBatcher.flush()
      stopWorkingHum().then(() => {
        playPermissionChime()
        enqueueTTS(`Permission needed: ${title}`)
      })
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

    // Break the current assistant message so that any text the agent sends
    // after the question is answered appears as a new chat bubble.
    currentAssistantId = null
    partContents.clear()

    // Initialize answer tracking for this question batch
    if (questionId && !pendingQuestionAnswers.has(questionId)) {
      pendingQuestionAnswers.set(questionId, {
        totalQuestions,
        answers: new Map(),
      })
    }

    // Add question as a special message in the chat
    messages.value.push({
      id: nextMessageId(),
      role: 'system',
      content: event.content || header || 'Question',
      timestamp: Date.now(),
      type: 'question_request',
      meta: {
        questionId,
        questionIndex,
        totalQuestions,
        header,
        options,
        multiple,
        resolved: false,
      },
    })

    // TTS: Flush any buffered text from the agent's preamble so it
    // speaks in correct order before the question announcement.
    // For multi-question batches, announce overview + first question only.
    // Subsequent questions are announced after the previous one is answered
    // (see respondToQuestion).
    if (voiceEnabled.value && questionIndex === 0) {
      ttsToolBatcher.flush()
      ttsChunker.flush()
      stopWorkingHum().then(() => {
        playQuestionChime()
        if (totalQuestions > 1) {
          enqueueTTS(`I have ${totalQuestions} questions for you.`)
        }
        announceQuestion(event.content || header, options)
      })
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
  }

  function rejectQuestion(questionId: string) {
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

  // Initialize
  fetchSession()
  fetchMessages()

  // Cleanup on scope dispose
  onScopeDispose(() => {
    unsubscribe()
    stopMonitoring()
  })

  return {
    session: readonly(session),
    messages,
    isStreaming: readonly(isStreaming),
    hasPendingPermission,
    hasPendingQuestion,
    isTTSPlaying,
    isRecording,
    isMonitoring,
    voiceEnabled: readonly(voiceEnabled),
    connectionState,
    sendMessage,
    abortSession,
    stopTTS,
    setAgent,
    setModel,
    toggleVoice,
    respondToPermission,
    respondToQuestion,
    rejectQuestion,
  }
}
