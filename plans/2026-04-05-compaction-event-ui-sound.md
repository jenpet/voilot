# Plan: First-Class Compaction Event UI + Sound

**Status:** draft
**Created:** 2026-04-05
**Author:** user + planitect

## Goal
Make compaction a first-class interaction in Voilot so users see a single compact UI item instead of a large assistant text block, avoid verbose TTS output during compaction, and get clear lightweight feedback while compaction is in progress.

## Context
OpenCode emits compaction via `message.part.updated` / `message.part.delta` with `type: "compaction"`. Today, backend fallback logic forwards unknown part types as regular `text` events, which causes the frontend to render large assistant messages and route compaction content into TTS. Existing frontend audio feedback already generates tones programmatically (no static audio assets), so compaction feedback should follow that pattern.

## Approach
1. Introduce a dedicated compaction event type end-to-end.
2. Detect and preserve compaction in backend SSE parsing.
3. Prevent verbose compaction text from being spoken.
4. Add compaction lifecycle behavior in frontend agent flow.
5. Render compaction as one collapsible UI item.
6. Add subtle looping background sound during compaction.
7. Validate behavior end-to-end.

## Open Questions
- Should collapsed compaction items default to always-collapsed or be responsive by screen size?
- Should compaction start/done announcements be suppressed when TTS is disabled?

## Acceptance Criteria
- Backend emits `type: "compaction"` for compaction parts (including deltas).
- Frontend treats compaction as a dedicated message type.
- Chat shows a single collapsible compaction item.
- Compaction summary body is not spoken verbatim by TTS.
- TTS says "Compaction started" and "Compaction done".
- A subtle looping gentle pulse plays only while compaction is active.
