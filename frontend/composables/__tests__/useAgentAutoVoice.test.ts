/**
 * Tests for the auto-voice arming logic in useAgent's handleTextEvent.
 *
 * The auto-voice feature arms on new sessions when autoVoiceNewSessions is
 * enabled. When armed, the first text event should set voiceInitiatedTurn=true
 * so the mic auto-starts after TTS finishes, regardless of whether the state
 * is idle or already agent_turn (from session_busy_on_load / initial_tools).
 *
 * This test extracts the core decision logic to validate it in isolation
 * without needing the full useAgent dependency tree.
 */

import { describe, it, expect, beforeEach } from 'vitest';

// ── Extracted auto-voice arm logic ──────────────────────────────────
// Mirrors the logic in useAgent.ts handleTextEvent (lines ~469-480).

interface AutoVoiceContext {
  autoVoiceArmed: boolean;
  voiceInitiatedTurn: boolean;
  currentState: string;
  dispatchCalls: Array<{ action: string; trigger: string }>;
}

function simulateAutoVoiceOnText(ctx: AutoVoiceContext): AutoVoiceContext {
  if (ctx.autoVoiceArmed) {
    ctx.autoVoiceArmed = false;
    ctx.voiceInitiatedTurn = true;
    if (ctx.currentState === 'idle') {
      ctx.dispatchCalls.push({ action: 'start_agent_turn', trigger: 'auto_voice_initial' });
    }
  }
  return ctx;
}

describe('auto-voice arming', () => {
  describe('text arrives while idle (no prior busy signal)', () => {
    it('sets voiceInitiatedTurn and dispatches start_agent_turn', () => {
      const ctx = simulateAutoVoiceOnText({
        autoVoiceArmed: true,
        voiceInitiatedTurn: false,
        currentState: 'idle',
        dispatchCalls: [],
      });

      expect(ctx.voiceInitiatedTurn).toBe(true);
      expect(ctx.autoVoiceArmed).toBe(false);
      expect(ctx.dispatchCalls).toEqual([
        { action: 'start_agent_turn', trigger: 'auto_voice_initial' },
      ]);
    });
  });

  describe('text arrives while already in agent_turn (session_busy_on_load)', () => {
    it('sets voiceInitiatedTurn without dispatching (already in agent_turn)', () => {
      const ctx = simulateAutoVoiceOnText({
        autoVoiceArmed: true,
        voiceInitiatedTurn: false,
        currentState: 'agent_turn',
        dispatchCalls: [],
      });

      expect(ctx.voiceInitiatedTurn).toBe(true);
      expect(ctx.autoVoiceArmed).toBe(false);
      // No dispatch — already in agent_turn
      expect(ctx.dispatchCalls).toEqual([]);
    });
  });

  describe('not armed (user already sent a message)', () => {
    it('does not modify voiceInitiatedTurn', () => {
      const ctx = simulateAutoVoiceOnText({
        autoVoiceArmed: false,
        voiceInitiatedTurn: false,
        currentState: 'idle',
        dispatchCalls: [],
      });

      expect(ctx.voiceInitiatedTurn).toBe(false);
      expect(ctx.dispatchCalls).toEqual([]);
    });
  });

  describe('arm is consumed on first text event only', () => {
    it('second text event does not re-arm', () => {
      const ctx: AutoVoiceContext = {
        autoVoiceArmed: true,
        voiceInitiatedTurn: false,
        currentState: 'agent_turn',
        dispatchCalls: [],
      };

      // First text event consumes the arm
      simulateAutoVoiceOnText(ctx);
      expect(ctx.autoVoiceArmed).toBe(false);
      expect(ctx.voiceInitiatedTurn).toBe(true);

      // Reset voiceInitiatedTurn to test second call
      ctx.voiceInitiatedTurn = false;
      simulateAutoVoiceOnText(ctx);
      expect(ctx.voiceInitiatedTurn).toBe(false); // Not re-armed
    });
  });
});
