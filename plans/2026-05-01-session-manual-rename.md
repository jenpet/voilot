# Plan: Manual Session Rename

**Status:** draft
**Created:** 2026-05-01
**Author:** jenpet + planitect

## Goal

Allow users to manually rename sessions so they can actually identify them in the session list. OpenCode's auto-titling via `small_model` is unreliable (requires provider-specific config and may not be available), so voilot needs its own title override that persists across restarts.

## Context

- OpenCode sets session titles to "New session - \<timestamp\>" by default. Auto-titling requires a `small_model` config entry pointing to a cheap model, which may not be configured.
- Voilot already displays `session.title` with an "Untitled Session" fallback in both `SessionCard.vue` and `session/[id].vue`.
- The `sessionmap` package (`backend/internal/sessionmap/sessionmap.go`) already provides file-backed JSON persistence mapping `sessionID -> worktreePath`. This is the natural place to co-locate title overrides.
- The frontend already handles `session.updated` SSE events that carry title changes from OpenCode (`useAgent.ts` line 427).

## Approach

### 1. Evolve sessionmap entry format

Change `sessionmap.Map` entries from `map[string]string` (sessionID -> worktreePath) to `map[string]Entry` where:

```go
type Entry struct {
    WorktreePath string `json:"worktreePath,omitempty"`
    Title        string `json:"title,omitempty"`
}
```

**Migration:** On load, if a JSON value is a plain string (not an object), treat it as `Entry{WorktreePath: value}`. This preserves backward compatibility with existing session map files. Implement by attempting struct unmarshal first, falling back to plain string parsing per entry.

Update all call sites that use `Get()`, `Set()`, etc. to work with the new `Entry` type.

### 2. Add REST endpoint for title update

`PATCH /api/sessions/:id/title` with body `{"title": "string"}`.

- Validate title length (max 80 characters), return 400 if exceeded.
- Store title in the session map entry via a new `SetTitle(sessionID, title)` method.
- Return the updated session object.

### 3. Manual title takes priority over OpenCode

In the backend, when returning sessions (list or get), check if the session map has a non-empty title for that session ID. If so, use it instead of whatever OpenCode returned.

In the frontend, when handling `session.updated` SSE events with a title change: skip the update if the session has a manual title override. The frontend can track this with a simple flag or by comparing against a known override. Simplest approach: the backend always returns the correct title (manual wins), so the frontend just needs to re-fetch or trust the backend-provided title over SSE updates.

### 4. Frontend: inline editable title

**Session page header** (`pages/session/[id].vue`, line 14):
- Replace the static `<h1>` with an inline editable text field.
- On tap/click, switch to an `<input>` pre-filled with the current title.
- Save on Enter or blur. Cancel on Escape (revert to previous value).
- `PATCH /api/sessions/:id/title` on save.
- 80 character maxlength on the input.

**Session card** (`components/SessionCard.vue`, line 7):
- Same inline edit behavior on the title `<h3>`.
- Prevent the card's click-to-navigate from firing when editing the title (stop propagation on the edit area).

### 5. Expose override status to frontend

Include a `titleOverride` boolean or similar in the session REST response so the frontend knows whether the title was manually set. This allows the frontend to skip OpenCode SSE title updates for overridden sessions without extra requests.

## Open Questions

None -- all resolved during design discussion.

## Acceptance Criteria

- [ ] Existing session map files with plain string values load without errors.
- [ ] Tapping the session title in the header or session card enters inline edit mode.
- [ ] Saving a title persists it across backend restarts.
- [ ] Manual titles are shown instead of OpenCode-provided titles.
- [ ] Titles are capped at 80 characters.
- [ ] Sessions without a manual title still show the OpenCode title or "Untitled Session" fallback.
