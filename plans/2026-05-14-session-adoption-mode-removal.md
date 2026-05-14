# Plan: Adopt CLI-created sessions and remove session mode

**Status:** draft
**Created:** 2026-05-14
**Author:** jenpet + planitect

## Goal

Make CLI-created OpenCode sessions visible in voilot's UI, and remove the unused `SessionMode` concept which adds complexity without providing value.

## Context

When a user runs `opencode` directly in a worktree that voilot manages, OpenCode creates sessions that voilot never learns about. The `handleListSessions` handler filters sessions through the `sessionMap`, which only contains sessions created via voilot. CLI-created sessions are silently dropped.

Since each OpenCode instance is scoped to exactly one worktree, any session returned by that instance inherently belongs to that worktree. Unrecognized sessions can be safely adopted.

Separately, `SessionMode` (plan/implement) was an early concept that never gained real business logic. Its only effect is defaulting the agent name on session creation (`plan` -> `planitect`, `implement` -> `build`). The agent name already carries this information, making mode redundant. Removing it simplifies the codebase and eliminates a field that would otherwise need handling during adoption.

## Approach

### 1. Adopt unrecognized sessions in `handleListSessions`

In `backend/internal/api/handlers.go`, after fetching sessions from OpenCode and filtering to top-level, iterate through all sessions and register any that are missing from the session map:

- For each session not in `sessionMap`, call `SetEntry` with:
  - `WorktreePath`: the resolved worktree path from the request
  - `Provider`: the resolved provider name already in scope
- Set the agent to `"planitect"` (safe default; OpenCode's session response doesn't include agent info)
- After adoption, proceed with the existing worktree filter as before -- adopted sessions now pass through

This is an eager approach: adoption happens on list, no background sync needed.

**Files:**
- `backend/internal/api/handlers.go` — `handleListSessions`: add adoption loop before the worktree filter

### 2. Adopt in workspace handler session listing

The workspace handler at `backend/internal/api/workspace_handlers.go` also iterates sessions from running instances to compute last-activity timestamps. Apply the same adoption logic here: when iterating sessions from each instance, register any unrecognized session into the session map using the instance's `WorkDir` and `ProviderName`. This ensures CLI sessions are adopted eagerly on app launch, not just when navigating into a specific worktree.

**Files:**
- `backend/internal/api/workspace_handlers.go` — add adoption loop in the session iteration block (around line 43)

### 3. Remove `SessionMode` from the backend

**Remove the mode type and constants:**
- `backend/internal/agent/adapter.go` — delete `SessionMode`, `ModePlan`, `ModeImplement`
- `backend/internal/agent/adapter.go` — remove `Mode` field from `Session` and `SessionOptions`
- `backend/internal/agent/adapter.go` — remove `SetSessionMode` and `GetSessionMode` from the `Adapter` interface

**Remove mode storage from the OpenCode adapter:**
- `backend/internal/agent/opencode.go` — remove the `sessionModes` map, `SetSessionMode`, `GetSessionMode` methods
- `backend/internal/agent/opencode.go` — remove mode assignment in `ListSessions`, `CreateSession`, `ResumeSession`

**Remove mode from API handlers:**
- `backend/internal/api/handlers.go` — remove mode defaulting logic in `handleCreateSession` (lines 276-278); default agent to `"planitect"` directly when agent is empty
- `backend/internal/api/handlers.go` — delete `handleSetSessionMode` handler entirely
- `backend/internal/api/router.go` — remove the `PATCH /sessions/{id}/mode` route
- `backend/internal/api/ws.go` — remove the mode-switch case in the WebSocket message handler

**Remove from mock adapter:**
- `backend/internal/agent/agenttest/mock_adapter.go` — remove `SetSessionMode`, `GetSessionMode`, mode-related fields

**Update tests:**
- `backend/internal/api/handlers_test.go` — remove any mode-related test assertions
- Any other tests referencing `ModePlan`, `ModeImplement`, or `SessionMode`

### 4. Remove `SessionMode` from the frontend

- `frontend/composables/useSession.ts` — remove `mode` field from session types
- `frontend/composables/useAgent.ts` — remove mode-related event handling (lines 438-439)
- `frontend/pages/session/[id].vue` — remove any mode references
- Remove any remaining mode toggle components or references if they exist

### 5. Default agent on session creation

With mode removed, simplify `handleCreateSession`:
- If `opts.Agent` is empty, default to `"planitect"`
- No mode-based branching needed

## Acceptance Criteria

- CLI-created OpenCode sessions appear in voilot's session list for the correct worktree
- Adopted sessions persist across backend restarts (session map is updated on disk)
- `SessionMode` type, fields, endpoints, and handlers are fully removed from backend and frontend
- Existing tests pass after mode removal
- Default agent for new sessions is `"planitect"` without mode indirection
