export interface Session {
  id: string
  title: string
  mode: 'plan' | 'implement'
  agent?: string
  model?: string
  lastUsedModel?: string
  projectId?: string
  time?: {
    created: number
    updated: number
  }
}

interface CreateSessionOptions {
  title?: string
  mode: 'plan' | 'implement'
  agent?: string
}

export function useSession() {
  const sessions = useState<Session[]>('sessions', () => [])
  const config = useRuntimeConfig()
  const apiBase = `${config.public.backendUrl}/api`

  async function fetchSessions() {
    try {
      const data = await $fetch<Session[]>(`${apiBase}/sessions`)
      sessions.value = data || []
    } catch {
      console.error('Failed to fetch sessions')
    }
  }

  async function createSession(opts: CreateSessionOptions): Promise<Session | null> {
    try {
      const session = await $fetch<Session>(`${apiBase}/sessions`, {
        method: 'POST',
        body: opts,
      })
      sessions.value.unshift(session)
      return session
    } catch {
      console.error('Failed to create session')
      return null
    }
  }

  async function deleteSession(id: string) {
    try {
      await $fetch(`${apiBase}/sessions/${id}`, {
        method: 'DELETE',
      })
      sessions.value = sessions.value.filter(s => s.id !== id)
    } catch {
      console.error('Failed to delete session')
    }
  }

  // Fetch sessions on first use
  if (sessions.value.length === 0) {
    fetchSessions()
  }

  return {
    sessions: readonly(sessions),
    fetchSessions,
    createSession,
    deleteSession,
  }
}
