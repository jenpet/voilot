/**
 * Client-side TTS filter for agent events.
 * Mirrors the Go voice.FilterForTTS logic — decides what agent events
 * should be spoken aloud vs. skipped/summarized.
 */

import type { AgentEvent } from './useWebSocket'

export interface FilterResult {
  /** Text that should be spoken aloud */
  textForTTS: string
  /** Whether TTS should be triggered at all */
  shouldSpeak: boolean
}

/**
 * Filters an agent event for TTS output.
 *
 * Rules:
 * - text: speak as-is (trimmed)
 * - code: summarize ("Wrote N lines of language")
 * - tool_use: briefly announce ("Using toolName")
 * - tool_result: skip (too verbose)
 * - thinking: skip (internal reasoning)
 * - error: announce with prefix
 * - done: skip unless there's content
 * - status/session_*: skip
 */
export function filterForTTS(event: AgentEvent): FilterResult {
  switch (event.type) {
    case 'text': {
      const text = (event.content || '').trim()
      if (!text) {
        return { textForTTS: '', shouldSpeak: false }
      }
      return { textForTTS: text, shouldSpeak: true }
    }

    case 'code': {
      const lang = event.language || 'code'
      const lines = (event.content || '').split('\n').length
      return {
        textForTTS: `Wrote ${lines} lines of ${lang}.`,
        shouldSpeak: true,
      }
    }

    case 'tool_use': {
      const toolName = (event.meta?.tool as string) || ''
      if (toolName) {
        return { textForTTS: `Using ${toolName}.`, shouldSpeak: true }
      }
      return { textForTTS: '', shouldSpeak: false }
    }

    case 'tool_result':
      // Too verbose to read tool results aloud
      return { textForTTS: '', shouldSpeak: false }

    case 'thinking':
      // Skip internal reasoning
      return { textForTTS: '', shouldSpeak: false }

    case 'error':
      return {
        textForTTS: `Error: ${event.content || 'Unknown error'}`,
        shouldSpeak: true,
      }

    case 'question_request': {
      // Speak the question text and enumerate options
      const question = (event.content || '').trim()
      if (!question) {
        return { textForTTS: '', shouldSpeak: false }
      }
      const options = (event.meta?.options as Array<{ label: string }>) || []
      if (options.length > 0) {
        const labels = options.map(o => o.label)
        const last = labels.pop()
        const optList = labels.length > 0 ? `${labels.join(', ')}, or ${last}` : last
        return { textForTTS: `${question}. ${optList}.`, shouldSpeak: true }
      }
      return { textForTTS: question, shouldSpeak: true }
    }

    case 'done':
      if (event.content) {
        return { textForTTS: event.content, shouldSpeak: true }
      }
      return { textForTTS: '', shouldSpeak: false }

    default:
      return { textForTTS: '', shouldSpeak: false }
  }
}
