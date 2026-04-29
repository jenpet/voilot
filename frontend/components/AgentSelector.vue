<template>
  <div class="relative">
    <button
      class="flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full transition-colors bg-surface-700/50 text-surface-200"
      @click="toggleDropdown"
    >
      <span
        v-if="activeAgent?.color"
        class="w-2 h-2 rounded-full flex-shrink-0"
        :style="{ backgroundColor: activeAgent.color }"
      />
      <span class="truncate max-w-[120px]">{{ activeAgent?.name || agent || 'Agent' }}</span>
      <svg class="w-3 h-3 flex-shrink-0 opacity-60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    <!-- Dropdown -->
    <div
      v-if="isOpen"
      class="absolute top-full left-0 mt-1 w-64 bg-surface-800 border border-surface-600 rounded-lg shadow-xl z-50 py-1 max-h-60 overflow-y-auto"
    >
      <button
        v-for="a in agents"
        :key="a.name"
        class="w-full text-left px-3 py-2 hover:bg-surface-700 transition-colors flex items-start gap-2"
        :class="{ 'bg-surface-700/50': a.name === agent }"
        @click="selectAgent(a.name)"
      >
        <span
          v-if="a.color"
          class="w-2 h-2 rounded-full flex-shrink-0 mt-1.5"
          :style="{ backgroundColor: a.color }"
        />
        <span v-else class="w-2 h-2 rounded-full flex-shrink-0 mt-1.5 bg-surface-500" />
        <div class="min-w-0">
          <div class="text-xs font-medium text-surface-100">{{ a.name }}</div>
          <div v-if="a.description" class="text-xs text-surface-400 line-clamp-2 mt-0.5">
            {{ a.description }}
          </div>
        </div>
      </button>
      <div v-if="agents.length === 0" class="px-3 py-2 text-xs text-surface-400 italic">
        No agents available
      </div>
    </div>

    <!-- Click-outside overlay -->
    <div
      v-if="isOpen"
      class="fixed inset-0 z-40"
      @click="isOpen = false"
    />
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  agent: string
}>()

const emit = defineEmits<{
  select: [agentName: string]
}>()

const { agents } = useAgents()

const isOpen = ref(false)

const activeAgent = computed(() =>
  agents.value.find(a => a.name === props.agent)
)

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

function selectAgent(name: string) {
  isOpen.value = false
  if (name !== props.agent) {
    emit('select', name)
  }
}
</script>
