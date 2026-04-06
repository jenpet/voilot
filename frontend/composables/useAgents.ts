/**
 * Composable to fetch and cache the list of available agents from the backend.
 * Agents are fetched once and shared across all components via useState.
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

  async function fetchAgents() {
    if (loading.value) return
    loading.value = true
    try {
      const data = await $fetch<AgentInfo[]>(`${apiBase}/agents`)
      agents.value = data || []
    } catch {
      console.error('Failed to fetch agents')
    } finally {
      loading.value = false
    }
  }

  // Fetch on first use if empty
  if (agents.value.length === 0) {
    fetchAgents()
  }

  return {
    agents: readonly(agents),
    loading: readonly(loading),
    fetchAgents,
  }
}
