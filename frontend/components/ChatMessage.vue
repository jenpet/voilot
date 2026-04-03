<template>
  <div
    class="rounded-xl px-4 py-3 max-w-[85%]"
    :class="messageClasses"
  >
    <!-- Permission request -->
    <div v-if="message.type === 'permission_request'" class="min-w-0">
      <!-- Header row: icon + title -->
      <div class="flex items-start gap-2">
        <span v-if="isPermissionResolved" class="text-sm mt-0.5 flex-shrink-0">
          <span v-if="permissionResponse === 'reject'" class="text-red-400">&#x2717;</span>
          <span v-else class="text-green-400">&#x2713;</span>
        </span>
        <span v-else class="text-sm mt-0.5 flex-shrink-0 text-amber-400">&#x26A0;</span>
        <div class="flex-1 min-w-0">
          <p class="text-xs font-medium" :class="isPermissionResolved ? 'text-surface-300' : 'text-amber-200'">
            {{ permissionTitle }}
          </p>
          <p v-if="permissionPattern" class="text-xs text-surface-400 mt-0.5 truncate font-mono">
            {{ permissionPattern }}
          </p>
        </div>
      </div>

      <!-- Resolution label -->
      <p v-if="isPermissionResolved" class="text-xs mt-2" :class="permissionResponse === 'reject' ? 'text-red-400/80' : 'text-green-400/80'">
        {{ permissionResponseLabel }}
      </p>

      <!-- Action buttons (only when pending) -->
      <div v-else class="flex items-center gap-2 mt-3">
        <button
          class="px-3 py-1.5 text-xs rounded-lg bg-green-600/30 text-green-300 hover:bg-green-600/50 active:bg-green-600/70 transition-colors"
          @click="respond('once')"
        >
          Allow Once
        </button>
        <button
          class="px-3 py-1.5 text-xs rounded-lg bg-blue-600/30 text-blue-300 hover:bg-blue-600/50 active:bg-blue-600/70 transition-colors"
          @click="respond('always')"
        >
          Allow Always
        </button>
        <button
          class="px-3 py-1.5 text-xs rounded-lg bg-red-600/30 text-red-300 hover:bg-red-600/50 active:bg-red-600/70 transition-colors"
          @click="respond('reject')"
        >
          Deny
        </button>
      </div>
    </div>

    <!-- Tool use / tool result -->
    <div v-else-if="message.type === 'tool_use' || message.type === 'tool_result'" class="flex items-start gap-2">
      <span class="text-xs mt-0.5 flex-shrink-0">
        {{ message.type === 'tool_use' ? '⚙' : '✓' }}
      </span>
      <div class="min-w-0">
        <p class="text-xs font-medium text-surface-300">
          {{ toolTitle }}
        </p>
        <p v-if="message.content && message.type === 'tool_result'" class="text-xs text-surface-400 mt-1 truncate">
          {{ message.content.slice(0, 200) }}
        </p>
      </div>
    </div>

    <!-- System message -->
    <div v-else-if="message.role === 'system'" class="text-xs text-surface-400 italic">
      {{ message.content }}
    </div>

    <!-- Regular text message -->
    <div v-else>
      <p class="text-sm whitespace-pre-wrap break-words">{{ message.content }}</p>
      <span class="block mt-1 text-xs text-surface-500">
        {{ message.role === 'user' ? 'You' : 'Agent' }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Message } from '~/composables/useAgent'
import { RespondToPermissionKey } from '~/composables/useAgent'

const props = defineProps<{
  message: Message
}>()

const respondToPermission = inject(RespondToPermissionKey, null)

// ─── Permission helpers ────────────────────────────────────────────

const isPermissionResolved = computed(() => props.message.meta?.resolved === true)
const permissionResponse = computed(() => props.message.meta?.resolvedResponse as string | undefined)

const permissionTitle = computed(() => {
  return (props.message.meta?.title as string) || props.message.content || 'Permission needed'
})

const permissionPattern = computed(() => {
  const pattern = props.message.meta?.pattern
  if (!pattern) return ''
  return Array.isArray(pattern) ? pattern.join(', ') : String(pattern)
})

const permissionResponseLabel = computed(() => {
  switch (permissionResponse.value) {
    case 'once': return 'Allowed once'
    case 'always': return 'Allowed always'
    case 'reject': return 'Denied'
    default: return 'Resolved'
  }
})

function respond(response: 'once' | 'always' | 'reject') {
  const permissionId = props.message.meta?.permissionId as string | undefined
  if (!permissionId || !respondToPermission) return
  respondToPermission(permissionId, response)
}

// ─── Message styling ───────────────────────────────────────────────

const messageClasses = computed(() => {
  if (props.message.type === 'permission_request') {
    if (isPermissionResolved.value) {
      return permissionResponse.value === 'reject'
        ? 'mr-auto bg-red-900/20 border border-red-800/30 text-surface-200'
        : 'mr-auto bg-green-900/20 border border-green-800/30 text-surface-200'
    }
    return 'mr-auto bg-amber-900/20 border border-amber-700/40 text-surface-200'
  }
  if (props.message.role === 'user') {
    return 'ml-auto bg-blue-600/20 text-blue-100'
  }
  if (props.message.role === 'system') {
    return 'mx-auto bg-surface-800/50 text-surface-400 max-w-full text-center'
  }
  if (props.message.type === 'tool_use' || props.message.type === 'tool_result') {
    return 'mr-auto bg-surface-800/60 border border-surface-700/50 text-surface-300'
  }
  return 'mr-auto bg-surface-800 text-surface-100'
})

const toolTitle = computed(() => {
  const tool = props.message.meta?.tool as string | undefined
  const status = props.message.meta?.status as string | undefined
  const title = props.message.meta?.title as string | undefined
  if (title) return title
  if (tool) return `${tool}${status ? ` (${status})` : ''}`
  return props.message.content
})
</script>
