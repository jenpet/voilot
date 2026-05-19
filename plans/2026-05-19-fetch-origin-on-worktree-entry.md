# Plan: Fetch origin on worktree entry

**Status:** draft
**Created:** 2026-05-19
**Author:** jen + planitect

## Goal
Ensure git refs are current with the remote when entering a worktree (session creation) or creating a new worktree. This prevents working with stale branch information if another machine has pushed changes or deleted branches.

## Context
Currently:
- `handleListBranches` fetches origin before listing branches (best-effort)
- `handleCreateSession` collects a worktree snapshot with local git state only
- `handleCreateWorktree` runs `wt switch -c` without fetching

This means the snapshot's ahead/behind counts and branch metadata reflect local refs only. If another dev pushed commits or changed the default branch on remote, you'd see stale information until a manual fetch.

## Approach

1. Create a helper function `EnsureOriginUpToDate(ctx, repoPath)` in `workspace_handlers.go` that runs `git fetch origin --quiet`

2. Call it in `handleCreateSession` before creating the session (so the snapshot sees current refs and the agent starts with up-to-date state)

3. Call it in `handleCreateWorktree` before `wt switch -c` runs (so the base branch is current)

4. Decide: should fetch failures be fatal or best-effort (like `handleListBranches`)?
   - Lean toward best-effort to not break the workflow on network issues
   - But log the error for debugging

## Open Questions

- Should fetch be fatal or best-effort? (Leaning best-effort)
- Should we handle fetch errors specially, or just log and continue?

## Acceptance Criteria

- When you create a session, the snapshot's ahead/behind counts reflect current remote state
- When you create a worktree with a base branch, that branch is up to date before `wt switch -c` runs
- Network errors don't block session/worktree creation
