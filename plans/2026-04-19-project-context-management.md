# Plan: Project Context Management via Git Worktrees

**Status:** draft
**Created:** 2026-04-19
**Updated:** 2026-04-20
**Author:** jenpet + planitect

## Goal
Enable voilot to manage multiple projects and in-flight work branches in a single workspace, with sessions scoped to specific worktrees. Each branch gets its own directory via git worktrees, so switching between work contexts is instant and no checkout state is lost.

## Context
- One OpenCode server runs from the workspace directory, serving all projects.
- Multiple projects coexist as subdirectories (voilot, schwabenclaw, bookr, etc.).
- Work on a branch requires the directory to stay on that branch, which conflicts with other work on the same repo.
- Previously solved with duplicate clones (e.g. `voilot-secondary`), which doesn't scale.
- Fresh workspace directory will be created — no migration of existing setup needed.

## Approach

### 1. Tooling: Worktrunk

[Worktrunk](https://worktrunk.dev/) (`wt`) is used as the CLI tool for creating, managing, and cleaning up git worktrees. It wraps `git worktree` with ergonomic commands:

- `wt switch -c <branch>` — create branch + worktree in one step
- `wt remove` — clean up worktree + branch
- `wt merge` — squash/rebase/merge + cleanup
- `wt list` — dashboard of all worktrees with status

**Worktrunk is a management tool, not a runtime dependency.** Voilot's backend does not shell out to `wt` for discovery. Instead, it scans the filesystem and uses git metadata to identify projects and worktrees independently. This avoids tight coupling — if the Worktrunk path template changes, nothing breaks.

**Worktrunk configuration:**
```toml
# ~/.config/worktrunk/config.toml
worktree-path = "{{ repo_path }}/../{{ repo }}.{{ branch | sanitize }}"
```

This produces sibling directories like `voilot.plan-pwa-setup/` next to `voilot/`.

**Installation:** The existing `task agents:install` task (which handles agent symlinks and skill installation) should be extended to check for and install Worktrunk via Homebrew (`brew install worktrunk && wt config shell install`), similar to how it already installs the grill-me skill.

### 2. Directory Convention

The workspace directory uses a flat, one-level structure:

- **Main repo:** `<project>/` — symlink (default) or direct clone. The only full clone per project.
- **Worktrees:** `<project>.<branch-slug>/` — created by Worktrunk as siblings of the main repo.

Example:
```
voilot-wd/
  voilot/                            -> symlink to ~/dev/jenpet/voilot (main clone)
  voilot.plan-pwa-setup/             -> worktree for plan/pwa-setup branch
  voilot.plan-project-context/       -> worktree for plan/project-context-management branch
  schwabenclaw/                      -> symlink to ~/dev/jenpet/schwabenclaw
  schwabenclaw.feat-new-api/         -> worktree for feat/new-api branch
```

### 3. Discovery

The voilot backend scans the workspace directory (one level deep, flat) on startup and periodically to discover projects and worktrees.

**How it works:**
1. List all direct children of the workspace directory.
2. For each directory that is a git repo, check whether it's a main worktree or a linked worktree (`git rev-parse --git-common-dir` vs `--git-dir`).
3. Group linked worktrees under their parent repo by matching the common git dir.
4. Non-git directories are ignored.

**Configuration:** The workspace directory path is provided via CLI flag: `--workspace-dir /path/to/voilot-wd`.

### 4. Session-to-Worktree Mapping

OpenCode runs as a single instance from the workspace directory. All sessions share the same `directory` field, so OpenCode cannot scope sessions to worktrees natively.

Voilot solves this with two mechanisms:

**A. Session map (persistence):**
A JSON file at `voilot-data/session-map.json` maps session IDs to worktree paths:
```json
{
  "ses_258d408c...": "/Users/jenpet/tmp/voilot-wd/voilot.plan-pwa-setup",
  "ses_25a8907a...": "/Users/jenpet/tmp/voilot-wd/schwabenclaw"
}
```
- The mapping is set at session creation time and locked to that worktree.
- Sessions created before this feature exist in an "unassigned" bucket.
- Future improvement: allow reassigning sessions to a different worktree.

**B. System prompt injection (agent scoping):**
When a session is created from a worktree context, voilot prepends a system prompt:
> "Work exclusively in /path/to/worktree. All file reads, edits, and commands should target this directory."

This reuses the same mechanism already used for plan vs implement mode.

**Discarded alternative:** Running multiple OpenCode instances (one per worktree) was considered but rejected — too cumbersome to manage, wasteful of resources, and unnecessary when system prompt injection achieves the same scoping.

### 5. UI Hierarchy

The current flat session list is replaced with a three-level navigation:

**Project → Worktree → Session**

#### Project List (home screen)
- Auto-detected from the workspace directory via filesystem scan.
- Each project shows its name and number of active worktrees.
- Action: "New Project" — set up a new repo (from scratch, clone, or import existing).

#### Worktree List (tap a project)
- Shows all worktrees for the project, including main.
- Each worktree shows its branch name and git status (clean/dirty).
- Actions:
  - "New Worktree" — user provides a short description (e.g. "PWA offline support"), voilot slugifies it to a branch name (e.g. `plan/pwa-offline-support`), and creates the worktree via `wt switch -c <branch> --no-cd --no-hooks` run from the main repo.
  - "Remove" — subtle gesture (swipe or long-press, no visible button). Calls `wt remove` under the hood. Sessions mapped to the removed worktree become orphaned and appear in a "stale" bucket.

#### Session List (tap a worktree)
- Shows sessions mapped to this worktree.
- Action: "New Session" — creates an OpenCode session, maps it to this worktree in the session map, injects the system prompt, and opens the chat view.

### 6. Entry Points

#### A. New Project

For projects not yet in the workspace.

**A1. From scratch:**
1. Ask: project name, where the clone should live on disk (default: `~/dev/jenpet/<project>`).
2. Ask: create a GitHub repository? If yes, create it via GitHub API.
3. Run `git init` or `git clone` to set up the main clone.
4. Make an initial commit if starting from scratch.
5. Symlink the clone into the workspace: `ln -s <clone-path> voilot-wd/<project>`.

**A2. Clone from remote:**
1. User provides a remote URL.
2. Ask: where should the clone live on disk (default: `~/dev/jenpet/<project>`).
3. `git clone <url> <clone-path>`.
4. Symlink into the workspace.

**A3. Import existing local repo:**
1. User provides the path to an existing clone.
2. Symlink it into the workspace.
3. Verify git remote is set up and fetch latest.

#### B. New Worktree on Existing Project

1. User provides a short description in the UI.
2. Voilot slugifies it to a branch name (e.g. "PWA offline support" becomes `plan/pwa-offline-support`).
3. Backend runs `wt switch -c <branch> --no-cd --no-hooks` from the main repo directory.
4. Worktree appears in the project's worktree list.

#### C. Plans as File Artifacts

Plans are not a first-class UI concept. They are markdown files that live in a worktree's `plans/` directory, created through conversation with the agent. The agent may suggest writing a plan when a discussion reaches a natural point of structure, or the user can ask for one at any time.

### 7. Cleanup

When a worktree is no longer needed:

1. User triggers removal via subtle UI gesture (swipe/long-press on worktree).
2. Backend calls `wt remove` to delete the worktree and optionally the branch.
3. Sessions mapped to the removed worktree become orphaned — they remain in the session map but the UI shows them in a "stale" bucket. Conversation history is preserved.

## Future Considerations
- **Voice navigation** — voice commands for project management actions from the hub (e.g. "new worktree on voilot", "open schwabenclaw staging").
- **Session reassignment** — allow moving a session from one worktree to another after creation.
- **Agent-suggested plans** — the agent could proactively suggest writing a plan document when a conversation accumulates enough context.

## Acceptance Criteria
- Voilot backend discovers projects and worktrees from the workspace directory via git metadata.
- Sessions are mapped to worktrees at creation time and scoped via system prompt injection.
- UI provides three-level navigation: Project → Worktree → Session.
- New worktrees can be created from the UI with a short description, slugified into a branch name.
- Worktree cleanup removes the worktree and orphans associated sessions gracefully.
- Worktrunk is used for worktree lifecycle management but is not a runtime dependency for discovery.
