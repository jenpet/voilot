# Plan: Local Welcome Message

**Status:** draft
**Created:** 2026-05-17
**Author:** jenpet + planitect

## Goal

Replace the agent-generated session greeting with a locally-produced welcome
message. The current approach sends a scope prompt to the agent on every new
session, which triggers tool calls (git commands), takes several seconds, and
produces a structurally identical greeting every time. By running a handful of
git commands in the backend and rendering a template, we eliminate the latency,
the API cost, and a significant chunk of frontend complexity.

## Context

- `ScopePrompt()` in `adapter.go` builds a prompt that is sent via
  `InitializeSession` → `SendMessageAsync` to the agent backend.
- The agent runs `git branch`, `git status`, `git log` via tool calls, then
  responds with a short greeting.
- The frontend suppresses the tool-call events during this initial phase via
  `suppressInitialTools` — a flag woven through `useAgent.ts`,
  `useSessionState.ts`, `useAudioOrchestrator.ts`, and their tests.
- The `GetMessages` path in `opencode.go` (line ~394) filters out the scope
  prompt and hides initial tool messages. This becomes dead code.

## Approach

### 1. Collect git snapshot at session creation

In the `handleCreateSession` handler, after the session is created, run these
git commands against the worktree path:

| Command | Stored property | Notes |
|---|---|---|
| `git rev-parse --abbrev-ref HEAD` | `branch` | "HEAD" if detached |
| `git rev-parse --short HEAD` | `headSHA` | For detached HEAD display |
| `git status --porcelain` | `staged`, `modified`, `untracked` (counts) | Parse first two columns |
| `git log -1 --format=%s` | `lastCommitSubject` | Empty if no commits |
| `git rev-parse --abbrev-ref @{upstream}` | `upstream` | Empty if no tracking branch |
| `git rev-list --count HEAD..@{upstream}` | `commitsBehind` | 0 if no upstream |
| `git rev-list --count @{upstream}..HEAD` | `commitsAhead` | 0 if no upstream |

If any command fails, store zero-values for that field. All commands should
complete in under 50ms.

Add a helper (e.g. `agent.CollectWorktreeSnapshot(path string)`) that returns
a `WorktreeSnapshot` struct with these fields, plus an `isEmpty` boolean
(detected when `git log` fails).

### 2. Store snapshot in session map

Extend `sessionmap.Entry` with a `WelcomeSnapshot *WorktreeSnapshot` field
(pointer, `omitempty`). Populated at creation time, persisted with the session
map JSON. Old entries without the field gracefully default to nil.

### 3. Render welcome message in GetMessages handler

In `handleGetSessionMessages`, before returning the agent's message history:

1. Look up the session map entry.
2. If `WelcomeSnapshot` is present, render it via a template into a
   `HistoryMessage` with:
   - ID: `welcome-{sessionID}` (deterministic, stable across calls)
   - Role: `assistant`
   - Timestamp: session creation time
   - Meta: `{"hidden": false}` (or omit meta entirely)
3. Prepend it to the message list.

If `WelcomeSnapshot` is nil (old session or error), skip — no welcome message.

### 4. Template scenarios

| Scenario | Example output |
|---|---|
| Clean, on branch | "You're on `plan/welcome-message`. Working tree is clean. Last commit: *Refactor TTS filter*." |
| Clean, on branch, with upstream | "You're on `plan/welcome-message`, 2 ahead / 1 behind `origin/plan/welcome-message`. Working tree is clean. Last commit: *Refactor TTS filter*." |
| Dirty, on branch | "You're on `main` with 3 modified, 1 untracked files. Last commit: *Fix audio loop*." |
| Detached HEAD | "Detached at `a1b2c3d`. Working tree is clean. Last commit: *Initial commit*." |
| Empty repo | "Fresh repo on `main`, no commits yet." |
| Error / missing snapshot | "Working in `/path/to/worktree`." |

### 5. Remove backend welcome infrastructure

- Delete `ScopePrompt()` from `adapter.go`.
- Remove `InitializeSession` from the `Adapter` interface.
- Remove `InitializeSession` implementation from `opencode.go`.
- Remove `InitializeSession` from `MockAdapter` in `agenttest/`.
- Remove the `InitializeSession` call in `handleCreateSession`.
- Remove the scope-prompt filter and initial-tool-message hiding logic in
  `OpenCodeAdapter.GetMessages()`.

### 6. Remove frontend suppress logic

- Delete `suppressInitialTools` state and `_setSuppressInitialTools` from
  `useSessionState.ts`.
- Remove all `suppressInitialTools` references from `useAgent.ts`:
  - Event filtering in `fetchMessages` (line ~295)
  - Tool-use / tool-result suppression in the event switch (lines ~369, ~386)
  - Status event suppression (line ~421)
  - The `initial_tools` dispatch (line ~372)
  - The `_setSuppressInitialTools(false)` call on first text event (line ~364)
  - The check for user messages in history (line ~305)
- Remove `suppressInitialTools` from `useAudioOrchestrator.ts` (line ~92, ~170).
- Remove or simplify related tests in `useSessionState.test.ts` and
  `useAudioOrchestrator.watchers.test.ts` / `transitions.test.ts`.

### 7. Startup dependency check

On backend startup, iterate a `requiredBinaries` list (initially `["git", "wt"]`)
and verify each is available on `$PATH` via `exec.LookPath`. If any are missing,
fail startup with a clear error naming the missing binaries. Easy to extend
when new dependencies are added later.

### 8. Audio feedback

Evaluate whether the "working hum" on session entry still makes sense given
the welcome message is now instant. This is a UX decision to make during
implementation — the hum may feel odd for a zero-latency greeting.

## Open Questions

None — all branches resolved during planning.

## Acceptance Criteria

- [ ] New session shows a welcome message instantly (no agent round-trip).
- [ ] Welcome message includes branch, dirty state, last commit, and
      upstream ahead/behind when available.
- [ ] Welcome message appears as a normal assistant message in chat history.
- [ ] `suppressInitialTools` is fully removed from the frontend.
- [ ] `InitializeSession` and `ScopePrompt` are removed from the backend.
- [ ] Old sessions without a stored snapshot still work (no welcome, no crash).
- [ ] Session map JSON remains backward-compatible (omitempty on new fields).
- [ ] Backend fails startup if `git` or `wt` are not found on `$PATH`.
