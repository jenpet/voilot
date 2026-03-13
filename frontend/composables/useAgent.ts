import type { Session } from './useSession'
import type { AgentEvent } from './useWebSocket'
import { filterForTTS } from './useTTSFilter'

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
  const { enqueue: enqueueTTS, stop: stopTTS } = useTTS()
  const { mark, isActive: isTimerActive } = useRoundTripTimer()

  const session = ref<Session | null>(null)
  const messages = ref<Message[]>([])
  const isStreaming = ref(false)
  const voiceEnabled = ref(false)

  // Track the current assistant message being streamed
  let currentAssistantId: string | null = null
  // Track which partId maps to which text so far (for delta accumulation)
  const partContents = new Map<string, string>()
  // Buffer for TTS: accumulate text events, speak when done
  let ttsTextBuffer = ''

  // Fetch session details via REST
  async function fetchSession() {
    try {
      const data = await $fetch<Session>(`${apiBase}/sessions/${sessionId}`)
      session.value = data
    } catch {
      console.error('Failed to fetch session')
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
    // Feed non-text events through TTS filter immediately
    if (voiceEnabled.value && event.type !== 'text' && event.type !== 'done' && event.type !== 'status') {
      const filtered = filterForTTS(event)
      if (filtered.shouldSpeak) {
        enqueueTTS(filtered.textForTTS)
      }
    }

    switch (event.type) {
      case 'text':
        handleTextEvent(event)
        break
      case 'tool_use':
        appendAssistantMeta(event.content, 'tool_use', event.meta)
        break
      case 'tool_result':
        appendAssistantMeta(event.content, 'tool_result', event.meta)
        break
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
      // Accumulate for TTS
      ttsTextBuffer += event.delta
    } else if (event.content) {
      // Full content replacement
      partContents.set(partId, event.content)
      ttsTextBuffer = event.content
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

    // Flush accumulated text to TTS when streaming completes
    if (voiceEnabled.value && ttsTextBuffer.trim()) {
      enqueueTTS(ttsTextBuffer.trim())
    }
    ttsTextBuffer = ''
    isStreaming.value = false
    currentAssistantId = null
    partContents.clear()
  }

  // Send a message via WebSocket
  function sendMessage(text: string) {
    // Only mark agent timing if this is part of a voice round-trip
    if (isTimerActive()) {
      mark('agent_ttft', 'start')
      mark('agent_full', 'start')
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
    ttsTextBuffer = ''
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

  // Cleanup on scope dispose
  onScopeDispose(() => {
    unsubscribe()
  })

  return {
    session: readonly(session),
    messages,
    isStreaming: readonly(isStreaming),
    voiceEnabled: readonly(voiceEnabled),
    connectionState,
    sendMessage,
    abortSession,
    toggleMode,
    toggleVoice,
  }
}
