/**
 * Composable to fetch and cache the list of available agents from the backend.
 * Agents are scoped to a specific worktree+provider instance and shared across
 * components via useState. Call fetchAgents(worktree, provider) to refresh.
 */

export interface AgentInfo {
  name: string
  description?: string
  color?: string
}

export function useAgents() {
  const apiBase = `${resolveBackendUrl()}/api`
  const agents = useState<AgentInfo[]>('agents', () => [])
  const loading = useState<boolean>('agents-loading', () => false)

  async function fetchAgents(worktree: string, provider: string) {
    if (loading.value) return
    loading.value = true
    try {
      const params = new URLSearchParams({ worktree, provider })
      const data = await $fetch<AgentInfo[]>(`${apiBase}/agents?${params}`)
      agents.value = data || []
    } catch {
      console.error('Failed to fetch agents')
    } finally {
      loading.value = false
    }
  }

  return {
    agents: readonly(agents),
    loading: readonly(loading),
    fetchAgents,
  }
}
