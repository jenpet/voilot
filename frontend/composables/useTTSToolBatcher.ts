/**
 * Batches consecutive tool-use (and tool-result) agent events into a
 * single TTS summary instead of announcing each one individually.
 *
 * Problem: when the agent runs 5 bash commands in a row, the old code
 * announced "Using bash. Using bash. Using bash. …" — noisy and unhelpful.
 *
 * Solution: collect tool_use and tool_result events silently. When the
 * batch is flushed (by a text event, done event, or debounce timeout),
 * emit one concise summary:
 *   - Single tool:  "Used bash."
 *   - Multiple same: "Used bash 3 times."
 *   - Mixed tools:   "Used 3 times bash and 2 times edit."
 *
 * Non-tool events that pass filterForTTS (e.g. error, code) flush the
 * pending batch first, then are spoken immediately.
 */

import type { AgentEvent } from './useWebSocket';
import { filterForTTS } from './useTTSFilter';

/** Debounce window in ms — enough to catch a burst of parallel tool calls */
const BATCH_WINDOW_MS = 1500;

export interface TTSToolBatcher {
  /** Feed a non-text agent event. Handles batching internally. */
  push: (event: AgentEvent) => void;
  /** Flush any pending batch immediately (call on text/done events). */
  flush: () => void;
  /** Discard pending batch without speaking (call on abort). */
  reset: () => void;
}

export function useTTSToolBatcher(
  enqueueTTS: (text: string) => void,
): TTSToolBatcher {
  // Accumulated tool names in the current batch, e.g. ["bash", "bash", "edit"]
  let pendingTools: string[] = [];
  let timer: ReturnType<typeof setTimeout> | null = null;

  function flushBatch() {
    clearTimer();
    if (pendingTools.length === 0) return;

    const summary = buildSummary(pendingTools);
    pendingTools = [];
    if (summary) {
      enqueueTTS(summary);
    }
  }

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function startTimer() {
    clearTimer();
    timer = setTimeout(flushBatch, BATCH_WINDOW_MS);
  }

  function push(event: AgentEvent) {
    // Silently absorb tool_use events into the batch
    if (event.type === 'tool_use') {
      const toolName = (event.meta?.tool as string) || '';
      if (toolName) {
        pendingTools.push(toolName);
        // Restart the debounce window — more tools may arrive
        startTimer();
      }
      return;
    }

    // Silently absorb tool_result events (they follow tool_use and are too verbose)
    if (event.type === 'tool_result') {
      // Keep the debounce alive if we have a pending batch
      if (pendingTools.length > 0) {
        startTimer();
      }
      return;
    }

    // Any other non-tool event — flush pending batch first, then handle normally
    flushBatch();

    const filtered = filterForTTS(event);
    if (filtered.shouldSpeak) {
      enqueueTTS(filtered.textForTTS);
    }
  }

  function reset() {
    clearTimer();
    pendingTools = [];
  }

  return { push, flush: flushBatch, reset };
}

/**
 * Build a concise TTS summary from a list of tool names.
 *
 * Examples:
 *   ["bash"]                            -> "Used bash."
 *   ["bash", "bash", "bash"]            -> "Used bash 3 times."
 *   ["bash", "bash", "edit"]            -> "Used 2 times bash and 1 time edit."
 *   ["bash", "edit", "grep", "read"]    -> "Used 1 time bash, 1 time edit, 1 time grep, and 1 time read."
 */
function buildSummary(tools: string[]): string {
  if (tools.length === 0) return '';

  // Single tool invocation
  if (tools.length === 1) {
    return `Used ${tools[0]}.`;
  }

  // Count occurrences
  const counts = new Map<string, number>();
  for (const t of tools) {
    counts.set(t, (counts.get(t) || 0) + 1);
  }

  // All the same tool
  if (counts.size === 1) {
    const [name, count] = [...counts.entries()][0];
    return `Used ${name} ${count} times.`;
  }

  // Format each entry as "N time(s) name"
  const parts: string[] = [];
  for (const [name, count] of counts.entries()) {
    parts.push(`${count} ${count === 1 ? 'time' : 'times'} ${name}`);
  }

  // Two distinct tools — compact
  if (parts.length === 2) {
    return `Used ${parts[0]} and ${parts[1]}.`;
  }

  // Three or more — Oxford comma
  const last = parts.pop()!;
  return `Used ${parts.join(', ')}, and ${last}.`;
}
