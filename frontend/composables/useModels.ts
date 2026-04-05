/**
 * Composable to fetch and cache the list of selectable models from the backend.
 * Models are fetched once and shared across components via useState.
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
  const config = useRuntimeConfig()
  const apiBase = `${config.public.backendUrl}/api`
  const models = useState<ModelInfo[]>('models', () => [])
  const defaultModel = useState<string>('default-model', () => '')
  const loading = useState<boolean>('models-loading', () => false)

  async function fetchModels() {
    if (loading.value) return
    loading.value = true
    try {
      const data = await $fetch<ModelCatalog>(`${apiBase}/models`)
      models.value = data?.models || []
      defaultModel.value = data?.defaultModel || ''
    } catch {
      console.error('Failed to fetch models')
    } finally {
      loading.value = false
    }
  }

  if (models.value.length === 0 && !loading.value) {
    fetchModels()
  }

  return {
    models: readonly(models),
    defaultModel: readonly(defaultModel),
    loading: readonly(loading),
    fetchModels,
  }
}
