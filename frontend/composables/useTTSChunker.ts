/**
 * Incremental TTS chunker — breaks streaming text into sentence-sized
 * chunks and enqueues them to TTS as they complete, rather than waiting
 * for the entire response.
 *
 * This dramatically reduces voice latency: TTS starts speaking after
 * the first sentence (~1-3s) instead of after the full response (30s+).
 *
 * Usage:
 *   const chunker = useTTSChunker(enqueueTTS)
 *   // On each text delta:
 *   chunker.push(delta)
 *   // When the agent finishes:
 *   chunker.flush()
 *   // On abort/interrupt:
 *   chunker.reset()
 */

import { useDebugLog } from './useDebugLog';
import type { DebugLogLevel } from './useDebugLog';

function _log(level: DebugLogLevel, event: string, data?: Record<string, unknown>) {
  try {
    const { log } = useDebugLog();
    log(level, 'tts-pipeline', event, data);
  } catch {
    // Composable not available outside setup — ignore
  }
}

// Sentence-ending punctuation followed by whitespace or end-of-string.
// Also splits on newline boundaries (paragraph breaks).
const SENTENCE_BOUNDARY = /(?<=[.!?:;])\s+|(?<=\n)\s*/

// Minimum chunk size in characters — avoids sending tiny fragments
// that produce awkward TTS with gaps. Kokoro handles ~5-50 words well.
const MIN_CHUNK_LENGTH = 40

// Maximum chunk size — if no sentence boundary is found within this
// many characters, force a split at the last whitespace to keep TTS
// latency bounded.
const MAX_CHUNK_LENGTH = 400

export interface TTSChunker {
  /** Feed a text delta into the chunker. May enqueue 0 or more TTS items. */
  push(delta: string): void
  /** Flush any remaining buffered text to TTS (call on "done" event). */
  flush(): void
  /** Discard all buffered text without speaking (call on abort/interrupt). */
  reset(): void
}

/**
 * Creates a TTS chunker that splits streaming text into speakable
 * sentence-sized chunks.
 *
 * @param enqueue - function to enqueue text for TTS synthesis
 * @param condense - optional function to condense/filter text before TTS
 */
export function useTTSChunker(
  enqueue: (text: string) => void,
  condense?: (text: string) => string,
): TTSChunker {
  let buffer = ''

  function emitChunk(text: string) {
    const cleaned = condense ? condense(text) : text
    const trimmed = cleaned.trim()
    if (trimmed) {
      _log('debug', 'chunker_emit', { length: trimmed.length, text: trimmed.substring(0, 60) })
      enqueue(trimmed)
    }
  }

  function push(delta: string) {
    buffer += delta

    // Try to extract complete sentences from the buffer
    while (buffer.length >= MIN_CHUNK_LENGTH) {
      // Look for a sentence boundary after MIN_CHUNK_LENGTH
      const searchFrom = MIN_CHUNK_LENGTH
      const searchRegion = buffer.slice(searchFrom)
      const match = searchRegion.match(SENTENCE_BOUNDARY)

      if (match && match.index !== undefined) {
        // Found a sentence boundary — split there
        const splitAt = searchFrom + match.index
        const chunk = buffer.slice(0, splitAt)
        buffer = buffer.slice(splitAt + (match[0]?.length || 0))
        emitChunk(chunk)
        continue
      }

      // No sentence boundary found — if buffer exceeds MAX_CHUNK_LENGTH,
      // force a split at the last whitespace to avoid unbounded buffering
      if (buffer.length > MAX_CHUNK_LENGTH) {
        const lastSpace = buffer.lastIndexOf(' ', MAX_CHUNK_LENGTH)
        if (lastSpace > MIN_CHUNK_LENGTH) {
          const chunk = buffer.slice(0, lastSpace)
          buffer = buffer.slice(lastSpace + 1)
          emitChunk(chunk)
          continue
        }
        // No good split point — emit the whole max chunk
        const chunk = buffer.slice(0, MAX_CHUNK_LENGTH)
        buffer = buffer.slice(MAX_CHUNK_LENGTH)
        emitChunk(chunk)
        continue
      }

      // Buffer is between MIN and MAX but no boundary yet — wait for more data
      break
    }
  }

  function flush() {
    if (buffer.trim()) {
      _log('debug', 'chunker_flush', { length: buffer.trim().length })
      emitChunk(buffer)
    }
    buffer = ''
  }

  function reset() {
    buffer = ''
  }

  return { push, flush, reset }
}
