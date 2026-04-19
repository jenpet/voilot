# Plan: Project Context Management via Git Worktrees

**Status:** draft
**Created:** 2026-04-19
**Author:** jenpet + planitect

## Goal
Enable Planitect to manage multiple in-flight plans and branches across all projects in the Voilot working directory without losing context between sessions. Each plan branch gets its own directory via git worktrees, so switching between plans is instant and no checkout state is lost.

## Context
- One OpenCode server runs from `voilot-wd/`, serving all projects.
- Multiple projects coexist as subdirectories (voilot, schwabenclaw, bookr, etc.).
- Plans create dedicated git branches (e.g. `plan/pwa-setup`), but returning to a project later requires the directory to still be on that branch.
- Currently this is solved with duplicate clones (e.g. `voilot-secondary`), which doesn't scale.
- Symlinks are viable and the default approach for linking the main repository clone into the working directory.

## Approach

### 1. Directory Convention

The working directory (`voilot-wd/`) uses a flat structure:

- **Main repo:** `<project>/` -- symlink (default) or direct clone. This is the only full clone; its checked-out branch is not restricted to `main`.
- **Worktrees:** `<project>-<branch-slug>/` -- git worktree for a specific branch, created flat alongside the main repo.

Examples:
```
voilot-wd/
  voilot/                        -> symlink to main clone (the only full clone)
  voilot-plan-pwa-setup/         -> worktree for plan/pwa-setup branch
  voilot-plan-project-context/   -> worktree for plan/project-context-management branch
  schwabenclaw/                  -> symlink to main clone
  schwabenclaw-staging/          -> worktree for staging branch
```

### 2. Entry Points

#### A. New Project

For projects not yet in the working directory.

**A1. From scratch:**
1. Ask: project name, where the clone should live on disk (default: `~/dev/jenpet/<project>`).
2. Ask: create a GitHub repository? If yes, create it (via `gh` or GitHub API).
3. Run `git init` or `git clone` to set up the main clone at the chosen location.
4. Make an initial commit if starting from scratch.
5. Symlink the clone into the working directory: `ln -s <clone-path> voilot-wd/<project>`.

**A2. Clone from remote:**
1. User provides a remote URL (e.g. `https://github.com/jenpet/<project>.git`).
2. Ask: where should the clone live on disk (default: `~/dev/jenpet/<project>`).
3. `git clone <url> <clone-path>`.
4. Symlink into the working directory: `ln -s <clone-path> voilot-wd/<project>`.

**A3. From existing local repo (import):**
1. User provides the path to an existing clone (or the user already has a symlink target in mind).
2. Symlink it into the working directory: `ln -s <clone-path> voilot-wd/<project>`.
3. Verify git remote is set up and fetch latest.

#### B. New Plan/Feature on Existing Project

For projects already managed in the working directory.

1. Identify the main clone (resolve symlink if needed).
2. Fetch latest from remote.
3. Create a new branch from the desired base (e.g. `plan/<slug>` from `main`).
4. Create a git worktree: `git worktree add ../<project>-plan-<slug> plan/<slug>` (run from main clone).
5. Write the plan document into the worktree's `plans/` directory.
6. Commit and optionally push from the worktree.

### 3. Resuming Work on a Plan

When the user wants to continue a plan:

1. Check which worktree directories exist for the project.
2. Navigate to the relevant worktree -- it's already on the correct branch.
3. Pick up where we left off. No checkout needed.

### 4. Cleanup

When a plan is merged or abandoned:

1. Remove the worktree: `git worktree remove <path>`.
2. Delete the branch if merged.

### 5. Migration of Existing Setup

- `voilot-secondary` and `schwabenclaw-secondary` should be converted to proper worktrees or replaced.
- Main clones that aren't symlinked yet can remain as-is or be moved and symlinked.

### 6. Main Menu UI (Project Hub)

The current main screen only shows sessions. This plan replaces it with a project-level overview:

- **Project list** -- auto-detected from the working directory (symlinks and git repos).
- **Worktrees per project** -- listed under each project with their branch name and status (clean/dirty).
- **Actions:**
  - "New Project" -- triggers entry point A (from scratch, clone, or import).
  - "New Plan" per project -- triggers entry point B (spawns a worktree).
  - Tap a worktree to open the session/conversation view scoped to that context.
- Sessions are scoped to a project/worktree rather than being a flat global list.

> **Future:** The main menu could also become agent-enabled, allowing voice interaction for project navigation and management actions directly from the hub.

## Open Questions
- Should Planitect auto-discover existing worktrees on startup, or rely on directory naming convention?
- How to handle projects that aren't git repos (e.g. `test-project`)?
- Should the branch slug in the directory name strip the `plan/` prefix (e.g. `voilot-pwa-setup`) or keep a hint (e.g. `voilot-plan-pwa-setup`)?

## Acceptance Criteria
- Planitect can create a new plan with an associated worktree in a single flow.
- Switching between plans for the same project requires no git checkout -- just changing directories.
- The main repo (symlink or direct clone) is the only full clone; plan work happens exclusively in worktrees.
- Worktrees are cleaned up when plans are completed.
