# Plan: Guard session adoption with directory match

**Status:** done
**Created:** 2026-05-18
**Author:** jenpet + planitect

## Goal

Prevent the "adopt unrecognized sessions" logic from mis-attributing OpenCode sessions to the wrong worktree. All per-worktree OpenCode instances share a single SQLite database (`~/.local/share/opencode/opencode.db`), so `ListSessions` returns sessions from every directory. The current adopt logic blindly claims any unmapped session, assigning it to whichever worktree the request targets. This caused 40+ phantom sessions to appear under `personal-coding-agents` despite the project never being used.

## Context

- Before the provider registry, a single shared OpenCode instance served all worktrees. A one-time backfill script (`backend/cmd/backfill/`) migrated existing sessions into `session-map.json` using directory matching and `[Working in:]` message prefix extraction.
- After the registry was introduced, two code paths adopt unmapped sessions at runtime:
  - `handlers.go` lines 259-283 (`handleListSessions`)
  - `workspace_handlers.go` lines 45-63 (project list endpoint)
- Both paths check only whether a session ID is absent from the session map. If absent, they assign it to the current worktree — regardless of the session's actual `directory` in OpenCode.
- OpenCode's `GET /session` response includes a `directory` field per session, but voilot's `openCodeSession` struct does not parse it.
- The backfill already correctly mapped all pre-registry sessions. The orphans are exclusively post-backfill false adoptions.

## Approach

### 1. Add `Directory` to `openCodeSession` and propagate to `Session`

- Add `Directory string` field to `openCodeSession` struct in `opencode.go`.
- Add `Directory string` field to `Session` struct in `adapter.go`.
- Map it in `toSession()`.

### 2. Guard adopt logic with directory match

In both `handleListSessions` (handlers.go) and the project list handler (workspace_handlers.go), change the adopt condition from:

```
if entry.WorktreePath == "" {
    // adopt to current worktree
}
```

to:

```
if entry.WorktreePath == "" && sessionDirectoryMatchesWorktree(sess.Directory, worktreePath) {
    // adopt to current worktree
}
```

The match function should resolve symlinks on both sides before comparing, consistent with `evalSymlinks` already used in `handleListSessions`.

### 3. One-time purge of `personal-coding-agents` session map entries

- Back up `session-map.json` to `session-map.json.bak` (one already exists — use timestamped name like `session-map.json.pre-purge`).
- Remove all entries where `worktreePath` is `/Users/JENPET/tmp/voilot-wd/personal-coding-agents`.
- This is a manual cleanup performed once, not automated tooling.

## Open Questions

None — all resolved during design discussion.

## Acceptance Criteria

- [ ] `openCodeSession` and `Session` structs include `Directory`.
- [ ] Adopt logic in both handlers only adopts sessions whose directory matches the target worktree (symlink-resolved).
- [ ] Sessions created via CLI in the correct worktree directory still appear in the UI.
- [ ] `session-map.json` no longer contains `personal-coding-agents` entries.
- [ ] Existing correctly-mapped sessions are unaffected.
