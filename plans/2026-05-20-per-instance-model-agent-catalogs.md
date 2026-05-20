# Plan: Per-Instance Model and Agent Catalogs

**Status:** ready
**Created:** 2026-05-20
**Author:** jenpet + planitect

## Goal

Make the model and agent lists reflect the actual capabilities of the provider instance bound to the user's session. Currently, `/api/models` and `/api/agents` use `anyAdapter()` which picks an arbitrary running instance -- if that instance lacks a provider (e.g., no `ANTHROPIC_API_KEY`), the models from that provider are missing from the dropdown even when the user's session runs on an instance that has it.

## Context

- Voilot supports multiple providers (`opencode`, `opencode-ss`) with different env vars and therefore different connected backends (Anthropic, OpenRouter, etc.).
- `anyAdapter()` grabs `instances[0]` from a Go map iteration -- nondeterministic. The model/agent list depends on which instance the map happens to yield first.
- The frontend (`useModels`, `useAgents`) caches the list forever via `useState`. In the PWA, a stale fetch from an early page load persists across navigation -- no refetch trigger exists.
- The model selector only appears on the session page. The agent selector likewise.

## Approach

### 1. Delete `anyAdapter()` and `handleStatus`

Remove `anyAdapter()` from `resolve.go`. It encodes a false assumption ("the answer is the same regardless of which instance we ask"). Remove `handleStatus` and its route -- it is the only remaining caller and has no frontend consumers.

### 2. Scope `/api/models` to worktree+provider

Change `handleListModels` to require `worktree` and `provider` query params. Resolve the adapter via `resolveAdapterForWorktree()` (which calls `GetOrSpawn`). Same pattern as `handleListSessions`.

### 3. Scope `/api/agents` to worktree+provider

Same treatment as models. Add `worktree` and `provider` query params, resolve via `resolveAdapterForWorktree()`.

### 4. Enrich `GET /api/sessions/{id}` response

Add `worktreePath` and `provider` fields to the session detail response (`sessionWithStatus` struct). Populated from the session map entry already being looked up by `resolveAdapter`. Gives the frontend the context it needs to call the scoped models/agents endpoints.

### 5. Update `useModels.ts`

Change `fetchModels()` to accept `worktree` and `provider` params. Pass as query params to `GET /api/models?worktree=...&provider=...`. Keep the single global `useState('models')` slot -- overwrite on each fetch. Remove the auto-fetch-on-first-use pattern (`if (models.value.length === 0)`).

### 6. Update `useAgents.ts`

Same treatment as models. `fetchAgents(worktree, provider)` with query params. Single global slot, overwrite on fetch. Remove auto-fetch.

### 7. Fetch on session page mount

In `pages/session/[id].vue`, after loading the session detail (which now includes `worktreePath` and `provider`), call both:

```ts
fetchModels(session.worktreePath, session.provider)
fetchAgents(session.worktreePath, session.provider)
```

No new composables or abstractions. The session page orchestrates both fetches explicitly.

## Files changed

| File | Change |
|------|--------|
| `backend/internal/api/resolve.go` | Delete `anyAdapter()` |
| `backend/internal/api/handlers.go` | Delete `handleStatus`. Update `handleListModels` and `handleListAgents` to use worktree+provider query params via `resolveAdapterForWorktree`. Enrich `handleGetSession` response with `worktreePath` and `provider`. |
| `backend/internal/api/router.go` | Remove `/api/status` route |
| `frontend/composables/useModels.ts` | `fetchModels(worktree, provider)` with query params, remove auto-fetch |
| `frontend/composables/useAgents.ts` | `fetchAgents(worktree, provider)` with query params, remove auto-fetch |
| `frontend/pages/session/[id].vue` | Call `fetchModels` and `fetchAgents` after session detail loads |
