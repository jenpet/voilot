<template>
  <ListCard
    :timestamp="session.time?.updated"
    deletable
    @delete="$emit('delete')"
  >
    <h3
      v-if="!editing"
      class="font-medium truncate cursor-pointer hover:text-accent transition-colors"
      @click.stop="startEdit"
    >{{ session.title || 'Untitled Session' }}</h3>
    <input
      v-else
      ref="editInputRef"
      v-model="editText"
      class="font-medium bg-bg-primary border border-accent rounded px-1.5 py-0.5 outline-none w-full text-sm"
      maxlength="80"
      @click.stop
      @keydown.enter.prevent="saveEdit"
      @keydown.escape.prevent="cancelEdit"
      @blur="saveEdit"
    />
    <div class="flex items-center gap-2 mt-1 min-w-0">
      <span
        v-if="session.provider"
        class="inline-flex items-center px-2 py-0.5 text-xs rounded-full flex-shrink-0 bg-bg-elevated text-text-muted"
      >
        {{ session.provider }}
      </span>
      <span
        class="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full flex-shrink-0"
        :style="agentBadgeStyle"
      >
        <span
          v-if="agentInfo?.color"
          class="w-1.5 h-1.5 rounded-full flex-shrink-0"
          :style="{ backgroundColor: agentInfo.color }"
        />
        {{ session.agent || 'planitect' }}
      </span>
      <span class="text-xs text-text-muted truncate">{{ session.id }}</span>
    </div>
  </ListCard>
</template>

<script setup lang="ts">
import type { Session } from '~/composables/useSession'

const props = defineProps<{
  session: Session
}>()

defineEmits<{
  delete: []
}>()

const { renameSession } = useSession()
const { agents } = useAgents()

// Inline title editing
const editing = ref(false)
const editText = ref('')
const editInputRef = ref<HTMLInputElement>()

function startEdit() {
  editText.value = props.session.title || ''
  editing.value = true
  nextTick(() => editInputRef.value?.select())
}

async function saveEdit() {
  if (!editing.value) return
  editing.value = false
  const newTitle = editText.value.trim()
  const oldTitle = props.session.title || ''
  if (newTitle === oldTitle) return
  await renameSession(props.session.id, newTitle)
}

function cancelEdit() {
  editing.value = false
}

const agentInfo = computed(() =>
  agents.value.find(a => a.name === (props.session.agent || 'planitect'))
)

const agentBadgeStyle = computed(() => {
  const color = agentInfo.value?.color
  if (color) {
    return {
      backgroundColor: `${color}20`,
      color: color,
    }
  }
  return {
    backgroundColor: 'var(--bg-elevated)',
    color: 'var(--text-primary)',
  }
})
</script>
