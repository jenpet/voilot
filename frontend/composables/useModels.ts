/**
 * Composable to fetch and cache the list of selectable models from the backend.
 * Models are scoped to a specific worktree+provider instance and shared across
 * components via useState. Call fetchModels(worktree, provider) to refresh.
 */

export interface ModelInfo {
  id: string
  name?: string
  providerId?: string
  providerName?: string
}

interface ModelCatalog {
  models: ModelInfo[]
  defaultModel?: string
}

export function useModels() {
  const apiBase = `${resolveBackendUrl()}/api`
  const models = useState<ModelInfo[]>('models', () => [])
  const defaultModel = useState<string>('default-model', () => '')
  const loading = useState<boolean>('models-loading', () => false)

  async function fetchModels(worktree: string, provider: string) {
    if (loading.value) return
    loading.value = true
    try {
      const params = new URLSearchParams({ worktree, provider })
      const data = await $fetch<ModelCatalog>(`${apiBase}/models?${params}`)
      models.value = data?.models || []
      defaultModel.value = data?.defaultModel || ''
    } catch {
      console.error('Failed to fetch models')
    } finally {
      loading.value = false
    }
  }

  return {
    models: readonly(models),
    defaultModel: readonly(defaultModel),
    loading: readonly(loading),
    fetchModels,
  }
}
