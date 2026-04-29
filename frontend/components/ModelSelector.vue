<template>
  <div class="relative">
    <button
      class="flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full transition-colors"
      :class="model ? 'bg-bg-secondary/50 text-text-primary' : 'bg-bg-secondary/70 text-text-primary'"
      @click="toggleDropdown"
    >
      <span class="truncate max-w-[200px] hidden sm:inline">{{ activeLabel }}</span>
      <span class="truncate max-w-[100px] sm:hidden">{{ compactLabel }}</span>
      <svg class="w-3 h-3 flex-shrink-0 opacity-60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    <div
      v-if="isOpen"
      class="absolute top-full left-0 mt-1 w-80 bg-bg-primary border border-bg-elevated rounded-lg shadow-xl z-50 py-1 max-h-72 overflow-y-auto"
    >
      <button
        class="w-full text-left px-3 py-2 hover:bg-bg-secondary transition-colors"
        :class="{ 'bg-bg-secondary/50': !model }"
        @click="selectModel('')"
      >
        <div class="text-xs font-medium text-text-primary">Default (OpenCode)</div>
        <div v-if="defaultLabel" class="text-xs text-text-muted mt-0.5">{{ defaultLabel }}</div>
      </button>

      <button
        v-for="m in models"
        :key="m.id"
        class="w-full text-left px-3 py-2 hover:bg-bg-secondary transition-colors"
        :class="{ 'bg-bg-secondary/50': m.id === model }"
        @click="selectModel(m.id)"
      >
        <div class="text-xs font-medium text-text-primary">{{ m.name || modelNameFromId(m.id) }}</div>
        <div class="text-xs text-text-muted mt-0.5">{{ formatProviderAndId(m) }}</div>
      </button>

      <div v-if="models.length === 0" class="px-3 py-2 text-xs text-text-muted italic">
        No models available
      </div>
    </div>

    <div
      v-if="isOpen"
      class="fixed inset-0 z-40"
      @click="isOpen = false"
    />
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  model: string
  lastUsedModel?: string
}>()

const emit = defineEmits<{
  select: [modelId: string]
}>()

const { models, defaultModel } = useModels()

const isOpen = ref(false)

const modelMap = computed(() => {
  const map = new Map<string, { id: string; name?: string; providerId?: string; providerName?: string }>()
  for (const m of models.value) {
    map.set(m.id, m)
  }
  return map
})

function modelNameFromId(id: string): string {
  const idx = id.indexOf('/')
  return idx >= 0 ? id.slice(idx + 1) : id
}

function providerFromId(id: string): string {
  const idx = id.indexOf('/')
  return idx >= 0 ? id.slice(0, idx) : ''
}

function formatProviderAndId(m: { id: string; providerId?: string; providerName?: string }): string {
  const providerName = m.providerName || m.providerId || providerFromId(m.id)
  return providerName ? `${providerName} - ${m.id}` : m.id
}

const activeLabel = computed(() => {
  if (!props.model) {
    if (defaultLabel.value) {
      return `Default (${defaultLabel.value})`
    }
    if (lastUsedLabel.value) {
      return `Default (Last used: ${lastUsedLabel.value})`
    }
    return 'Default (OpenCode)'
  }
  const selected = modelMap.value.get(props.model)
  const modelName = selected?.name || modelNameFromId(props.model)
  const provider = selected?.providerName || selected?.providerId || providerFromId(props.model)
  return provider ? `${provider} - ${modelName}` : modelName
})

const compactLabel = computed(() => {
  const resolvedId = props.model || defaultModel.value || ''
  if (!resolvedId) return 'Default'
  const selected = modelMap.value.get(resolvedId)
  return selected?.name || modelNameFromId(resolvedId)
})

const defaultLabel = computed(() => {
  const id = defaultModel.value
  if (!id) return ''
  const selected = modelMap.value.get(id)
  const modelName = selected?.name || modelNameFromId(id)
  const provider = selected?.providerName || selected?.providerId || providerFromId(id)
  return provider ? `${provider} - ${modelName}` : modelName
})

const lastUsedLabel = computed(() => {
  const id = props.lastUsedModel || ''
  if (!id) return ''
  const selected = modelMap.value.get(id)
  const modelName = selected?.name || modelNameFromId(id)
  const provider = selected?.providerName || selected?.providerId || providerFromId(id)
  return provider ? `${provider} - ${modelName}` : modelName
})

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

function selectModel(id: string) {
  isOpen.value = false
  if (id !== props.model) {
    emit('select', id)
  }
}
</script>
