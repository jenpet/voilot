export interface Session {
  id: string
  title: string
  titleOverride?: boolean
  mode: 'plan' | 'implement'
  agent?: string
  provider?: string
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
  provider?: string
  worktreePath?: string
}

export function useSession() {
  const sessions = useState<Session[]>('sessions', () => [])
  const apiBase = `${resolveBackendUrl()}/api`

  async function fetchSessions(worktreePath: string) {
    try {
      const data = await $fetch<Session[]>(`${apiBase}/sessions?worktree=${encodeURIComponent(worktreePath)}`)
      sessions.value = data || []
    } catch (err) {
      console.error('Failed to fetch sessions')
      throw err
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

  async function renameSession(id: string, title: string): Promise<boolean> {
    try {
      await $fetch(`${apiBase}/sessions/${id}/title`, {
        method: 'PATCH',
        body: { title },
      })
      const session = sessions.value.find(s => s.id === id)
      if (session) {
        session.title = title
        session.titleOverride = title !== ''
      }
      return true
    } catch {
      console.error('Failed to rename session')
      return false
    }
  }

  function clearSessions() {
    sessions.value = []
  }

  return {
    sessions: readonly(sessions),
    fetchSessions,
    clearSessions,
    createSession,
    deleteSession,
    renameSession,
  }
}
