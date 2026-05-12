import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ref, readonly, type Ref } from 'vue';

// Provide Nuxt auto-imports as globals
vi.stubGlobal('ref', ref);
vi.stubGlobal('readonly', readonly);

import { useAgentMessages, type ChatMessage } from '../useAgentMessages';

describe('useAgentMessages', () => {
  let builder: ReturnType<typeof useAgentMessages>;

  beforeEach(() => {
    builder = useAgentMessages();
  });

  // ── Text delta accumulation ──────────────────────────────────────

  describe('addTextDelta', () => {
    it('accumulates deltas into one assistant message', () => {
      builder.addTextDelta('p1', 'Hello ');
      builder.addTextDelta('p1', 'world');

      expect(builder.messages.value).toHaveLength(1);
      expect(builder.messages.value[0].role).toBe('assistant');
      expect(builder.messages.value[0].content).toBe('Hello world');
    });

    it('joins multiple partIds into one message', () => {
      builder.addTextDelta('p1', 'Part one. ');
      builder.addTextDelta('p2', 'Part two.');

      expect(builder.messages.value).toHaveLength(1);
      expect(builder.messages.value[0].content).toBe('Part one. Part two.');
    });

    it('sets display on the created message', () => {
      builder.addTextDelta('p1', 'hi', 'visual');

      expect(builder.messages.value[0].display).toBe('visual');
    });
  });

  // ── setTextContent ───────────────────────────────────────────────

  describe('setTextContent', () => {
    it('replaces part content with full snapshot', () => {
      builder.addTextDelta('p1', 'partial');
      builder.setTextContent('p1', 'full content');

      expect(builder.messages.value).toHaveLength(1);
      expect(builder.messages.value[0].content).toBe('full content');
    });
  });

  // ── appendMeta (tool split) ──────────────────────────────────────

  describe('appendMeta', () => {
    it('splits active text bubble and returns true', () => {
      builder.addTextDelta('p1', 'Let me check');
      const didSplit = builder.appendMeta('Using tool: bash', 'tool_use', { tool: 'bash' });

      expect(didSplit).toBe(true);
      expect(builder.messages.value).toHaveLength(2);
      expect(builder.messages.value[0].content).toBe('Let me check');
      expect(builder.messages.value[0].role).toBe('assistant');
      expect(builder.messages.value[1].content).toBe('Using tool: bash');
      expect(builder.messages.value[1].type).toBe('tool_use');
    });

    it('returns false when no active text', () => {
      const didSplit = builder.appendMeta('Using tool: bash', 'tool_use');

      expect(didSplit).toBe(false);
      expect(builder.messages.value).toHaveLength(1);
    });

    it('creates new text bubble after tool events', () => {
      builder.addTextDelta('p1', 'Let me check');
      builder.appendMeta('Using bash', 'tool_use');
      builder.appendMeta('Output: ok', 'tool_result');
      builder.addTextDelta('p2', 'I found something');

      expect(builder.messages.value).toHaveLength(4);
      expect(builder.messages.value[0].content).toBe('Let me check');
      expect(builder.messages.value[0].role).toBe('assistant');
      expect(builder.messages.value[1].type).toBe('tool_use');
      expect(builder.messages.value[2].type).toBe('tool_result');
      expect(builder.messages.value[3].content).toBe('I found something');
      expect(builder.messages.value[3].role).toBe('assistant');
      // They must be different messages
      expect(builder.messages.value[0].id).not.toBe(builder.messages.value[3].id);
    });
  });

  // ── Multiple tool groups ─────────────────────────────────────────

  describe('multiple tool groups', () => {
    it('produces correct splits for text → tools → text → tools → text', () => {
      builder.addTextDelta('p1', 'First text');
      builder.appendMeta('tool 1', 'tool_use');
      builder.appendMeta('result 1', 'tool_result');
      builder.addTextDelta('p2', 'Second text');
      builder.appendMeta('tool 2', 'tool_use');
      builder.appendMeta('result 2', 'tool_result');
      builder.addTextDelta('p3', 'Third text');

      expect(builder.messages.value).toHaveLength(7);
      // text, tool_use, tool_result, text, tool_use, tool_result, text
      expect(builder.messages.value[0].content).toBe('First text');
      expect(builder.messages.value[1].type).toBe('tool_use');
      expect(builder.messages.value[2].type).toBe('tool_result');
      expect(builder.messages.value[3].content).toBe('Second text');
      expect(builder.messages.value[4].type).toBe('tool_use');
      expect(builder.messages.value[5].type).toBe('tool_result');
      expect(builder.messages.value[6].content).toBe('Third text');

      // All three text bubbles are distinct
      const textIds = [0, 3, 6].map(i => builder.messages.value[i].id);
      expect(new Set(textIds).size).toBe(3);
    });
  });

  // ── User messages ────────────────────────────────────────────────

  describe('addUserMessage', () => {
    it('appends a user message', () => {
      builder.addUserMessage('Hello agent');

      expect(builder.messages.value).toHaveLength(1);
      expect(builder.messages.value[0].role).toBe('user');
      expect(builder.messages.value[0].content).toBe('Hello agent');
    });

    it('splits active assistant text', () => {
      builder.addTextDelta('p1', 'Thinking...');
      builder.addUserMessage('Interrupt');

      expect(builder.messages.value).toHaveLength(2);
      expect(builder.messages.value[0].role).toBe('assistant');
      expect(builder.messages.value[0].content).toBe('Thinking...');
      expect(builder.messages.value[1].role).toBe('user');
    });
  });

  // ── System messages ──────────────────────────────────────────────

  describe('addSystemMessage', () => {
    it('appends a system message with optional type and meta', () => {
      builder.addSystemMessage('Permission needed', 'permission_request', { permissionId: 'abc' });

      expect(builder.messages.value).toHaveLength(1);
      expect(builder.messages.value[0].role).toBe('system');
      expect(builder.messages.value[0].type).toBe('permission_request');
      expect(builder.messages.value[0].meta?.permissionId).toBe('abc');
    });

    it('splits active assistant text', () => {
      builder.addTextDelta('p1', 'Before');
      builder.addSystemMessage('Error occurred');

      expect(builder.messages.value).toHaveLength(2);
      expect(builder.messages.value[0].content).toBe('Before');
      expect(builder.messages.value[1].role).toBe('system');
    });
  });

  // ── setMessages ──────────────────────────────────────────────────

  describe('setMessages', () => {
    it('replaces the entire messages array', () => {
      builder.addTextDelta('p1', 'live message');
      const history: ChatMessage[] = [
        { id: 'h1', role: 'user', content: 'old msg', timestamp: 1000 },
        { id: 'h2', role: 'assistant', content: 'old reply', timestamp: 1001 },
      ];
      builder.setMessages(history);

      expect(builder.messages.value).toHaveLength(2);
      expect(builder.messages.value[0].id).toBe('h1');
      expect(builder.messages.value[1].id).toBe('h2');
    });

    it('resets accumulation state so next text creates a new message', () => {
      builder.addTextDelta('p1', 'accumulating');
      builder.setMessages([]);
      builder.addTextDelta('p2', 'fresh');

      expect(builder.messages.value).toHaveLength(1);
      expect(builder.messages.value[0].content).toBe('fresh');
    });
  });

  // ── reset ────────────────────────────────────────────────────────

  describe('reset', () => {
    it('clears accumulation state but preserves messages', () => {
      builder.addTextDelta('p1', 'First turn');
      builder.reset();
      builder.addTextDelta('p2', 'Second turn');

      expect(builder.messages.value).toHaveLength(2);
      expect(builder.messages.value[0].content).toBe('First turn');
      expect(builder.messages.value[1].content).toBe('Second turn');
      expect(builder.messages.value[0].id).not.toBe(builder.messages.value[1].id);
    });
  });

  // ── hasActiveAssistant ───────────────────────────────────────────

  describe('hasActiveAssistant', () => {
    it('returns false initially', () => {
      expect(builder.hasActiveAssistant()).toBe(false);
    });

    it('returns true after addTextDelta', () => {
      builder.addTextDelta('p1', 'text');
      expect(builder.hasActiveAssistant()).toBe(true);
    });

    it('returns false after appendMeta splits', () => {
      builder.addTextDelta('p1', 'text');
      builder.appendMeta('tool', 'tool_use');
      expect(builder.hasActiveAssistant()).toBe(false);
    });

    it('returns false after reset', () => {
      builder.addTextDelta('p1', 'text');
      builder.reset();
      expect(builder.hasActiveAssistant()).toBe(false);
    });
  });
});
