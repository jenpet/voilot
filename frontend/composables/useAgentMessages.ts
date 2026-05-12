import type { AgentEvent } from './useWebSocket';

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: number;
  type?: AgentEvent['type'];
  meta?: Record<string, unknown>;
  display?: 'default' | 'visual' | 'hidden';
}

export function useAgentMessages() {
  let idCounter = 0;
  function nextMessageId(): string {
    return `msg-${Date.now()}-${++idCounter}`;
  }

  const messages = ref<ChatMessage[]>([]);
  let currentAssistantId: string | null = null;
  const partContents = new Map<string, string>();

  /**
   * Split the current assistant text bubble if one is active.
   * Resets currentAssistantId and partContents so the next text event
   * creates a new message. Returns true if a split occurred.
   */
  function splitIfActive(): boolean {
    if (currentAssistantId) {
      currentAssistantId = null;
      partContents.clear();
      return true;
    }
    return false;
  }

  /**
   * Accumulate a text delta into the current assistant message,
   * creating a new one if needed.
   */
  function addTextDelta(partId: string, delta: string, display?: string): void {
    const existing = partContents.get(partId) || '';
    const updated = existing + delta;
    partContents.set(partId, updated);

    const fullContent = Array.from(partContents.values()).join('');
    upsertAssistantMessage(fullContent, display);
  }

  /**
   * Set full text content for a part (final snapshot from OpenCode).
   */
  function setTextContent(partId: string, content: string, display?: string): void {
    partContents.set(partId, content);

    const fullContent = Array.from(partContents.values()).join('');
    upsertAssistantMessage(fullContent, display);
  }

  /**
   * Create or update the current assistant text message.
   */
  function upsertAssistantMessage(content: string, display?: string): void {
    if (!currentAssistantId) {
      currentAssistantId = nextMessageId();
      messages.value.push({
        id: currentAssistantId,
        role: 'assistant',
        content,
        timestamp: Date.now(),
        display: display as ChatMessage['display'] || 'default',
      });
    } else {
      const msg = messages.value.find(m => m.id === currentAssistantId);
      if (msg) {
        msg.content = content;
      }
    }
  }

  /**
   * Append a meta message (tool_use, tool_result, permission_request, etc.).
   * Splits the current text bubble first if one is active.
   * Returns true if a split occurred.
   */
  function appendMeta(content: string, type: AgentEvent['type'], meta?: Record<string, unknown>): boolean {
    const didSplit = splitIfActive();
    messages.value.push({
      id: nextMessageId(),
      role: 'assistant',
      content: content || '',
      timestamp: Date.now(),
      type,
      meta,
    });
    return didSplit;
  }

  /**
   * Append a user message. Splits active assistant text first.
   */
  function addUserMessage(content: string, display?: ChatMessage['display']): void {
    splitIfActive();
    messages.value.push({
      id: nextMessageId(),
      role: 'user',
      content,
      timestamp: Date.now(),
      ...(display ? { display } : {}),
    });
  }

  /**
   * Append a system message. Splits active assistant text first.
   */
  function addSystemMessage(content: string, type?: AgentEvent['type'], meta?: Record<string, unknown>): void {
    splitIfActive();
    messages.value.push({
      id: nextMessageId(),
      role: 'system',
      content,
      timestamp: Date.now(),
      ...(type ? { type } : {}),
      ...(meta ? { meta } : {}),
    });
  }

  /**
   * Replace the entire messages array (for loading history from backend).
   */
  function setMessages(msgs: ChatMessage[]): void {
    messages.value = msgs;
    currentAssistantId = null;
    partContents.clear();
  }

  /**
   * Reset accumulation state without clearing messages.
   * Called at end-of-turn (finishStreaming).
   */
  function reset(): void {
    currentAssistantId = null;
    partContents.clear();
  }

  /**
   * Whether there is an active assistant message being accumulated.
   */
  function hasActiveAssistant(): boolean {
    return currentAssistantId !== null;
  }

  return {
    messages: readonly(messages) as Readonly<Ref<ChatMessage[]>>,
    /** Mutable access for direct manipulation (e.g. updating permission resolution) */
    mutableMessages: messages,
    addTextDelta,
    setTextContent,
    appendMeta,
    addUserMessage,
    addSystemMessage,
    setMessages,
    reset,
    hasActiveAssistant,
  };
}
