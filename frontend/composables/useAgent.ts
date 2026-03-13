import type { Session } from './useSession'

export interface Message {
  role: 'user' | 'assistant'
  content: string
  timestamp: number
}

interface AgentEvent {
  type: 'text' | 'code' | 'tool_use' | 'thinking' | 'error' | 'done'
  content: string
  language?: string
  meta?: Record<string, unknown>
}

export function useAgent(sessionId: string) {
  const config = useRuntimeConfig()
  const session = ref<Session | null>(null)
  const messages = ref<Message[]>([])
  const isStreaming = ref(false)

  // Fetch session details
  async function fetchSession() {
    try {
      const data = await $fetch<Session>(`${config.public.apiBaseUrl}/sessions/${sessionId}`)
      session.value = data
    } catch {
      console.error('Failed to fetch session')
    }
  }

  // Send a message via REST (SSE streaming)
  async function sendMessage(text: string) {
    // Add user message immediately
    messages.value.push({
      role: 'user',
      content: text,
      timestamp: Date.now(),
    })

    isStreaming.value = true
    let assistantContent = ''

    try {
      const response = await fetch(`${config.public.apiBaseUrl}/sessions/${sessionId}/message`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: text }),
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }

      // Add placeholder assistant message
      const assistantIndex = messages.value.length
      messages.value.push({
        role: 'assistant',
        content: '',
        timestamp: Date.now(),
      })

      // Parse SSE stream
      const reader = response.body?.getReader()
      const decoder = new TextDecoder()

      if (reader) {
        let buffer = ''
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (line.startsWith('data: ')) {
              try {
                const event: AgentEvent = JSON.parse(line.slice(6))
                if (event.type === 'text') {
                  assistantContent += event.content
                  messages.value[assistantIndex].content = assistantContent
                } else if (event.type === 'code') {
                  assistantContent += `\n\`\`\`${event.language || ''}\n${event.content}\n\`\`\`\n`
                  messages.value[assistantIndex].content = assistantContent
                } else if (event.type === 'error') {
                  assistantContent += `\n[Error: ${event.content}]\n`
                  messages.value[assistantIndex].content = assistantContent
                }
              } catch {
                // skip malformed events
              }
            }
          }
        }
      }
    } catch (err) {
      messages.value.push({
        role: 'assistant',
        content: `Failed to send message: ${err}`,
        timestamp: Date.now(),
      })
    } finally {
      isStreaming.value = false
    }
  }

  // Toggle between plan and implement mode
  async function toggleMode() {
    if (!session.value) return
    const newMode = session.value.mode === 'plan' ? 'implement' : 'plan'
    session.value.mode = newMode
    // TODO: notify backend of mode change
  }

  // Initialize
  fetchSession()

  return {
    session: readonly(session),
    messages,
    isStreaming: readonly(isStreaming),
    sendMessage,
    toggleMode,
  }
}
