<template>
  <div
    class="min-w-0"
    :class="messageClasses"
  >
    <!-- Agent name shown on agent switch -->
    <p
      v-if="showAgentName"
      class="text-[10px] text-text-muted mb-1"
    >
      {{ agentName }}
    </p>
    <!-- Permission request -->
    <div v-if="message.type === 'permission_request'" class="min-w-0">
      <!-- Header row: icon + title -->
      <div class="flex items-start gap-2">
        <span v-if="isPermissionResolved" class="text-sm mt-0.5 flex-shrink-0">
          <span v-if="permissionResponse === 'reject'" class="text-accent-warn">&#x2717;</span>
          <span v-else class="text-accent">&#x2713;</span>
        </span>
        <span v-else class="text-sm mt-0.5 flex-shrink-0 text-accent-secondary">&#x26A0;</span>
        <div class="flex-1 min-w-0">
          <p class="text-xs font-medium" :class="isPermissionResolved ? 'text-text-primary' : 'text-accent-secondary'">
            {{ permissionTitle }}
          </p>
          <p v-if="permissionPattern" class="text-xs text-text-muted mt-0.5 truncate font-mono">
            {{ permissionPattern }}
          </p>
        </div>
      </div>

      <!-- Resolution label -->
      <p v-if="isPermissionResolved" class="text-xs mt-2" :class="permissionResponse === 'reject' ? 'text-accent-warn/80' : 'text-accent/80'">
        {{ permissionResponseLabel }}
      </p>

      <!-- Action buttons (only when pending) -->
      <div v-else class="flex items-center gap-2 mt-3">
        <button
          class="px-3 py-1.5 text-xs bg-accent/20 text-accent hover:bg-accent/30 active:bg-accent/40 transition-colors"
          @click="respond('once')"
        >
          Allow Once
        </button>
        <button
          class="px-3 py-1.5 text-xs bg-accent/20 text-accent hover:bg-accent/30 active:bg-accent/40 transition-colors"
          @click="respond('always')"
        >
          Allow Always
        </button>
        <button
          class="px-3 py-1.5 text-xs bg-accent-warn/20 text-accent-warn hover:bg-accent-warn/30 active:bg-accent-warn/40 transition-colors"
          @click="respond('reject')"
        >
          Deny
        </button>
      </div>
    </div>

    <!-- Question request -->
    <div v-else-if="message.type === 'question_request'" class="min-w-0">
      <!-- Header row: icon + question -->
      <div class="flex items-start gap-2">
        <span v-if="isQuestionResolved" class="text-sm mt-0.5 flex-shrink-0">
          <span v-if="isQuestionRejected" class="text-accent-warn">&#x2717;</span>
          <span v-else class="text-accent">&#x2713;</span>
        </span>
        <span v-else class="text-sm mt-0.5 flex-shrink-0 text-accent">?</span>
        <div class="flex-1 min-w-0">
          <p v-if="questionHeader" class="text-[10px] font-semibold uppercase tracking-wider mb-0.5"
            :class="isQuestionResolved ? 'text-text-muted' : 'text-accent/70'">
            {{ questionHeader }}
          </p>
          <p class="text-xs font-medium" :class="isQuestionResolved ? 'text-text-primary' : 'text-accent'">
            {{ message.content }}
          </p>
        </div>
      </div>

      <!-- Resolved: show selected answer or rejection -->
      <div v-if="isQuestionResolved" class="mt-2">
        <p v-if="isQuestionRejected" class="text-xs text-accent-warn/80">
          Dismissed
        </p>
        <p v-else class="text-xs text-accent/80">
          {{ questionSelectedLabel }}
        </p>
      </div>

      <!-- Pending: option buttons (only for the active question) -->
      <div v-else-if="isActiveQuestion" class="mt-3">
        <div class="flex flex-wrap gap-2">
          <button
            v-for="option in questionOptions"
            :key="option.label"
            class="px-3 py-1.5 text-xs bg-accent/15 text-accent hover:bg-accent/25 active:bg-accent/35 transition-colors"
            :title="option.description"
            @click="selectOption(option.label)"
          >
            {{ option.label }}
          </button>
        </div>
        <!-- Dismiss button -->
        <button
          class="mt-2 px-2 py-1 text-[10px] text-text-muted hover:text-accent-warn hover:bg-accent-warn/10 transition-colors"
          @click="dismissQuestion"
        >
          Dismiss
        </button>
      </div>

      <!-- Waiting: not yet active in multi-question batch -->
      <div v-else class="mt-2">
        <p class="text-[10px] text-text-muted italic">
          Waiting for previous question...
        </p>
      </div>
    </div>

    <!-- Tool use / tool result -->
    <div v-else-if="message.type === 'tool_use' || message.type === 'tool_result'" class="flex items-start gap-2">
      <span class="text-xs mt-0.5 flex-shrink-0">
        {{ message.type === 'tool_use' ? '⚙' : '✓' }}
      </span>
      <div class="min-w-0">
        <p class="text-xs font-medium text-text-primary">
          {{ toolTitle }}
        </p>
        <p v-if="message.content && message.type === 'tool_result'" class="text-xs text-text-muted mt-1 truncate">
          {{ message.content.slice(0, 200) }}
        </p>
      </div>
    </div>

    <!-- System message -->
    <div v-else-if="message.role === 'system'" class="text-xs text-text-muted italic">
      {{ message.content }}
    </div>

    <!-- Regular text message -->
    <div v-else>
      <!-- Assistant: render markdown -->
      <div
        v-if="message.role === 'assistant'"
        class="text-sm break-words prose-chat"
        v-html="renderedContent"
      />
      <!-- User: plain text -->
      <p v-else class="text-sm whitespace-pre-wrap break-words">{{ message.content }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Message } from '~/composables/useAgent'
import { RespondToPermissionKey, RespondToQuestionKey, RejectQuestionKey, ActiveQuestionKey } from '~/composables/useAgent'
import { renderMarkdown } from '~/composables/useMarkdown'

const props = defineProps<{
  message: Message
  agentName?: string
  previousAgentName?: string
}>()

// Show agent name only on assistant text messages when agent differs from previous
const showAgentName = computed(() => {
  if (props.message.role !== 'assistant') return false
  if (props.message.type && props.message.type !== 'text') return false
  if (!props.agentName) return false
  return props.agentName !== props.previousAgentName
})

// ─── Markdown rendering (assistant messages only) ──────────────────
const renderedContent = computed(() => renderMarkdown(props.message.content))

const respondToPermission = inject(RespondToPermissionKey, null)
const respondToQuestion = inject(RespondToQuestionKey, null)
const rejectQuestion = inject(RejectQuestionKey, null)
const activeQuestion = inject(ActiveQuestionKey, ref(null))

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

// ─── Question helpers ──────────────────────────────────────────────

const isQuestionResolved = computed(() => props.message.meta?.resolved === true)
const isQuestionRejected = computed(() => props.message.meta?.rejected === true)

const questionHeader = computed(() => props.message.meta?.header as string || '')

const questionOptions = computed(() => {
  return (props.message.meta?.options as Array<{ label: string; description: string }>) || []
})

const questionSelectedLabel = computed(() => {
  const labels = props.message.meta?.selectedLabels as string[] | undefined
  if (!labels || labels.length === 0) return 'Answered'
  return labels.join(', ')
})

// Is this the currently active (first unanswered) question in the batch?
// Only the active question shows interactive buttons; others are dimmed.
const isActiveQuestion = computed(() => {
  if (props.message.type !== 'question_request') return false
  const qid = props.message.meta?.questionId
  const idx = props.message.meta?.questionIndex
  if (qid == null || idx == null) return false
  return activeQuestion.value === `${qid}:${idx}`
})

function selectOption(label: string) {
  const questionId = props.message.meta?.questionId as string | undefined
  const questionIndex = props.message.meta?.questionIndex as number | undefined
  if (questionId == null || questionIndex == null || !respondToQuestion) return
  // Single selection: send as a single-item array
  respondToQuestion(questionId, questionIndex, [label])
}

function dismissQuestion() {
  const questionId = props.message.meta?.questionId as string | undefined
  if (!questionId || !rejectQuestion) return
  rejectQuestion(questionId)
}

// ─── Message styling ───────────────────────────────────────────────

const messageClasses = computed(() => {
  if (props.message.type === 'permission_request') {
    const base = 'px-4 py-3'
    if (isPermissionResolved.value) {
      return permissionResponse.value === 'reject'
        ? `${base} bg-accent-warn/10 border border-accent-warn/30 text-text-primary`
        : `${base} bg-accent/10 border border-accent/30 text-text-primary`
    }
    return `${base} bg-accent-secondary/10 border border-accent-secondary/30 text-text-primary`
  }
  if (props.message.type === 'question_request') {
    const base = 'px-4 py-3'
    if (isQuestionResolved.value) {
      return isQuestionRejected.value
        ? `${base} bg-accent-warn/10 border border-accent-warn/30 text-text-primary`
        : `${base} bg-accent/10 border border-accent/30 text-text-primary`
    }
    return `${base} bg-accent/10 border border-accent/30 text-text-primary`
  }
  if (props.message.role === 'user') {
    return 'px-4 py-3 border-l-2 border-accent bg-accent/10 text-text-primary'
  }
  if (props.message.role === 'system') {
    return 'px-4 py-3 mx-auto bg-bg-primary/50 text-text-muted text-center'
  }
  if (props.message.type === 'tool_use' || props.message.type === 'tool_result') {
    return 'px-4 py-3 bg-bg-primary/60 border border-bg-elevated/50 text-text-primary'
  }
  // Assistant text: no box styling
  return 'py-1 text-text-primary'
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
