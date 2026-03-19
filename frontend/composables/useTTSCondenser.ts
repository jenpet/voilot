/**
 * TTS text condenser — transforms raw agent text into concise,
 * speakable output for TTS.
 *
 * Handles:
 * - Fenced code blocks (```...```) → summarized as "Wrote N lines of code"
 * - Inline code backticks → stripped, content spoken
 * - Markdown formatting (bold, italic, headers) → stripped
 * - Bullet/numbered lists → spoken naturally (capped at a few items)
 * - Consecutive blank lines → collapsed
 *
 * This is a stateful transformer because code blocks can span
 * multiple text chunks from the chunker.
 */

export interface TTSCondenser {
  /** Condense a text chunk for TTS. May return empty string to skip. */
  condense(text: string): string
  /** Reset state (call when starting a new response). */
  reset(): void
}

export function useTTSCondenser(): TTSCondenser {
  // Track whether we're currently inside a fenced code block
  let inCodeBlock = false
  // Accumulate code block lines for summarization
  let codeBlockLines = 0
  // Track which language the code block is in
  let codeBlockLang = ''

  const FENCE_OPEN = /^```(\w*)/
  const FENCE_CLOSE = /^```\s*$/

  function condense(text: string): string {
    const lines = text.split('\n')
    const output: string[] = []

    for (const line of lines) {
      // Check for code fence boundaries
      if (!inCodeBlock && FENCE_OPEN.test(line.trim())) {
        inCodeBlock = true
        codeBlockLines = 0
        const match = line.trim().match(FENCE_OPEN)
        codeBlockLang = match?.[1] || 'code'
        continue
      }

      if (inCodeBlock) {
        if (FENCE_CLOSE.test(line.trim())) {
          // End of code block — emit summary
          inCodeBlock = false
          const lang = codeBlockLang || 'code'
          if (codeBlockLines > 0) {
            output.push(`Wrote ${codeBlockLines} lines of ${lang}.`)
          }
          codeBlockLang = ''
          codeBlockLines = 0
        } else {
          // Inside code block — just count lines
          codeBlockLines++
        }
        continue
      }

      // Outside code blocks — clean up markdown for speech
      let cleaned = line

      // Strip markdown headers (## Title → Title)
      cleaned = cleaned.replace(/^#{1,6}\s+/, '')

      // Strip bold/italic markers
      cleaned = cleaned.replace(/\*{1,3}([^*]+)\*{1,3}/g, '$1')
      cleaned = cleaned.replace(/_{1,3}([^_]+)_{1,3}/g, '$1')

      // Strip inline code backticks but keep content
      cleaned = cleaned.replace(/`([^`]+)`/g, '$1')

      // Convert bullet points to natural speech
      cleaned = cleaned.replace(/^\s*[-*+]\s+/, '')
      // Convert numbered lists
      cleaned = cleaned.replace(/^\s*\d+[.)]\s+/, '')

      // Strip markdown links: [text](url) → text
      cleaned = cleaned.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')

      // Strip horizontal rules
      if (/^\s*[-*_]{3,}\s*$/.test(cleaned)) {
        continue
      }

      // Collapse excessive whitespace
      cleaned = cleaned.replace(/\s+/g, ' ').trim()

      if (cleaned) {
        output.push(cleaned)
      }
    }

    // If we're still inside a code block (spans into next chunk),
    // don't output anything — it'll be summarized when the block closes
    return output.join(' ').trim()
  }

  function reset() {
    inCodeBlock = false
    codeBlockLines = 0
    codeBlockLang = ''
  }

  return { condense, reset }
}
