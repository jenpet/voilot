import type { Session } from './useSession'
import type { AgentEvent } from './useWebSocket'
import { useTTSChunker } from './useTTSChunker'
import { useTTSCondenser } from './useTTSCondenser'
import { useTTSToolBatcher } from './useTTSToolBatcher'
import { playThinkingJingle } from './useRecordingFeedback'

export interface Message {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp: number
  type?: AgentEvent['type']
  meta?: Record<string, unknown>
}

let messageIdCounter = 0
function nextMessageId(): string {
  return `msg-${Date.now()}-${++messageIdCounter}`
}

export function useAgent(sessionId: string) {
  const config = useRuntimeConfig()
  const apiBase = `${config.public.backendUrl}/api`
  const { send, subscribe, connectionState } = useWebSocket()
  const { enqueue: enqueueTTS, stop: stopTTS, isPlaying: isTTSPlaying } = useTTS()
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
  // Called when the agent's turn is fully over (streaming done + TTS finished).
  function startLoopRecording() {
    if (!voiceEnabled.value || isRecording.value || isStreaming.value || isTTSPlaying.value) return
    loopRecordingActive.value = true
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

    if (text) {
      sendMessage(text)
    } else {
      // No speech detected (empty/too short) — start recording again
      startLoopRecording()
    }
  })

  // When TTS finishes and the agent is done streaming, start recording
  // for the user's next turn.
  watch(isTTSPlaying, (playing) => {
    if (!playing && !isStreaming.value) {
      startLoopRecording()
    }
  })

  // Watch voice toggle — clean up when voice is turned off
  watch(() => voiceEnabled.value, (enabled) => {
    if (!enabled) {
      stopMonitoring()
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
    // Feed non-text events through the tool batcher — it batches consecutive
    // tool_use events into a single TTS summary and passes other events
    // (error, code, etc.) through filterForTTS as before.
    if (voiceEnabled.value && event.type !== 'text' && event.type !== 'done' && event.type !== 'status') {
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
      case 'error':
        appendSystemMessage(`Error: ${event.content}`)
        break
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
        // Update session title or mode if changed
        if (session.value && event.meta?.mode) {
          session.value.mode = event.meta.mode as 'plan' | 'implement'
        } else if (session.value && event.content) {
          session.value.title = event.content
        }
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

    // Flush any remaining buffered text to TTS
    if (voiceEnabled.value) {
      ttsToolBatcher.flush()
      ttsChunker.flush()
    }
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
  function sendMessage(text: string) {
    // Only mark agent timing if this is part of a voice round-trip
    if (isTimerActive()) {
      mark('agent_ttft', 'start')
      mark('agent_full', 'start')
    }

    // Play subtle "thinking" jingle so the user knows the agent is working
    if (voiceEnabled.value) {
      playThinkingJingle()
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
    stopTTS() // Stop any ongoing TTS playback
    stopMonitoring() // Stop mic monitoring if active
    ttsChunker.reset()
    ttsCondenser.reset()
    ttsToolBatcher.reset()
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

  // Toggle between plan and implement mode
  function toggleMode() {
    if (!session.value) return
    const newMode = session.value.mode === 'plan' ? 'implement' : 'plan'
    const sent = send({
      type: 'set_mode',
      sessionId,
      content: newMode,
    })
    if (sent) {
      session.value.mode = newMode
    } else {
      appendSystemMessage('Failed to change mode: not connected to backend')
    }
  }

  function handleCommand(command: string) {
    switch (command) {
      case 'switch_plan':
        if (session.value && session.value.mode !== 'plan') {
          send({ type: 'set_mode', sessionId, content: 'plan' })
          session.value.mode = 'plan'
          appendSystemMessage('Switched to Planning mode')
        }
        break
      case 'switch_implement':
        if (session.value && session.value.mode !== 'implement') {
          send({ type: 'set_mode', sessionId, content: 'implement' })
          session.value.mode = 'implement'
          appendSystemMessage('Switched to Implement mode')
        }
        break
      case 'stop':
        abortSession()
        break
    }
  }

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
    isTTSPlaying,
    isRecording,
    isMonitoring,
    voiceEnabled: readonly(voiceEnabled),
    connectionState,
    sendMessage,
    abortSession,
    stopTTS,
    toggleMode,
    toggleVoice,
  }
}
