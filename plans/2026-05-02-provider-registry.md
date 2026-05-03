# Plan: ProviderRegistry — Per-Worktree Agent Instances

**Status:** draft
**Created:** 2026-05-02
**Author:** jenpet + planitect

## Goal

Replace the single shared OpenCode instance with a per-worktree model where voilot spawns and manages dedicated agent backend processes. This eliminates the soft message-based directory scoping, gives each worktree a truly isolated agent context, and allows CLI session resumption from the correct working directory.

## Context

Today voilot connects to one externally started `opencode serve` process running in a parent directory above all projects. Worktree isolation is faked by prepending `[Working in: /path]` to every message. This has two problems:

1. **No hard isolation** — the agent can still access files outside the scoped worktree; tool calls resolve relative paths from OpenCode's CWD, not the worktree.
2. **CLI session resumption requires the parent directory** — you can't `opencode` into a session from the worktree directory because OpenCode was started elsewhere.

The refactor is split into two phases:

- **Phase 1** (this plan): Introduce `ProviderRegistry`, spawn per-worktree OpenCode instances, route sessions through the registry.
- **Phase 2** (separate plan): Clean up the `Adapter` interface — extract OpenCode-specific capabilities (permissions, questions) into optional interfaces, slim down the core contract.

## Approach

### Phase 1: ProviderRegistry + Multi-Instance OpenCode

#### 1. Define the provider lifecycle interface

New file: `backend/internal/agent/provider.go`

```go
// Provider knows how to spawn, health-check, and stop an agent backend process.
type Provider interface {
    // Name returns the provider identifier (e.g. "opencode").
    Name() string
    // Spawn starts a new instance in the given working directory.
    // Returns the base URL of the running instance.
    Spawn(ctx context.Context, workDir string) (baseURL string, err error)
    // Healthy returns true if the instance at baseURL is responsive.
    Healthy(ctx context.Context, baseURL string) bool
    // Stop terminates the instance at baseURL.
    Stop(ctx context.Context, baseURL string) error
    // NewAdapter creates an Adapter connected to the given baseURL.
    NewAdapter(baseURL string) Adapter
}
```

OpenCode implements this by:
- `Spawn`: exec-ing `opencode serve --port 0` with CWD set to `workDir` (port 0 lets OpenCode pick a free port). Captures the assigned port from stdout/logs. Writes a PID file to the state directory. Waits for health check to pass before returning.
- `Healthy`: `GET /global/health`.
- `Stop`: sending SIGTERM to the managed process, removing the PID file.
- `NewAdapter`: calling the existing `NewOpenCodeAdapter(baseURL)`.

#### 2. Implement `ProviderRegistry`

New file: `backend/internal/agent/registry.go`

The registry manages running instances keyed by worktree path:

```go
type ProviderRegistry struct {
    provider     Provider
    mu           sync.RWMutex
    instances    map[string]*Instance // worktreePath -> instance
    maxInstances int                  // configurable via VOILOT_MAX_INSTANCES (default 5)
    idleTimeout  time.Duration        // default 10 minutes
}

type Instance struct {
    WorkDir      string
    BaseURL      string
    Adapter      Adapter
    Process      *os.Process
    LastActivity time.Time
    ActiveCount  int32        // sessions with active work (atomically updated)
}
```

Key behaviors:
- **`GetOrSpawn(ctx, worktreePath) (Adapter, error)`** — returns existing adapter or spawns a new instance. If `maxInstances` is reached, evicts the least recently active idle instance to make room. If all instances are busy, returns an error.
- **Idle reaper** — background goroutine checks instances periodically. An instance is eligible for teardown when:
  - `ActiveCount == 0` (no sessions actively processing)
  - `time.Since(LastActivity) > idleTimeout` (default 10 minutes)
- **`TouchActivity(worktreePath)`** — called on every message send/receive to reset the idle timer.
- **`MarkBusy/MarkIdle(worktreePath)`** — increments/decrements `ActiveCount` based on session status events (busy/idle from SSE).
- **`StopInstance(worktreePath)`** — explicit teardown, exposed via API for the UI kill button.
- **Graceful shutdown** — `Close()` stops all instances on voilot backend shutdown.

#### 3. Wire into the API layer

Changes to `backend/internal/api/server.go` and handlers:

- `Server` holds a `*ProviderRegistry` instead of a single `agent.Adapter`.
- Session listing / worktree navigation:
  - Calls `registry.GetOrSpawn(ctx, worktreePath)` when the user navigates to a worktree's session list. This is the natural trigger — no need for a global "all sessions" view.
- Session creation (`handleCreateSession`):
  - Requires `worktreePath` (no longer optional in workspace mode).
  - Calls `registry.GetOrSpawn(ctx, worktreePath)` to get the adapter.
  - Stores `sessionID -> worktreePath` in the session map (already exists).
- Message sending (`ws.go`):
  - Looks up worktree path from session map, gets adapter from registry.
  - Drops the `[Working in: ...]` prefix — no longer needed since the OpenCode instance is already scoped.
- SSE aggregation:
  - The registry aggregates all adapter SSE streams into one internal event bus.
  - The WebSocket handler subscribes to this single aggregated channel — no frontend changes needed.
  - Frontend continues to filter events by session ID as it does today.
- Worktree instance management:
  - New API endpoint to stop a specific instance (kill button in the worktree overview UI).
  - Instance status (running/stopped) exposed in the worktree list response.

#### 4. Port allocation

- `opencode serve` supports `--port 0` (default) which lets the OS assign a free port — no race window.
- Capture the assigned port from OpenCode's startup output (stdout/stderr) or by probing after launch.
- Also supports `--hostname` (default `127.0.0.1`) — no changes needed there.
- Store assigned port in the `Instance` struct.

#### 5. Process lifecycle and orphan cleanup

- **PID files**: On spawn, write `<voilot-state-dir>/<worktree-hash>.pid` containing the process PID and port.
- **Startup sweep**: On voilot startup, scan the PID file directory. For each file, check if the process is still running (`kill -0`). If alive, kill it. Delete the PID file either way.
- **Graceful shutdown**: `Close()` sends SIGTERM to all managed processes and removes PID files.
- **Orphan tolerance**: If voilot hard-crashes, orphaned OpenCode processes survive until the next voilot startup. An idle OpenCode process consumes minimal resources, so this is acceptable.
- **No background watchdog or stdin pipe tricks** — just PID files and a startup sweep. Tested: OpenCode does not exit on stdin EOF.
- **Crash handling**: If an OpenCode instance crashes mid-session, surface an error to the frontend. No auto-respawn — let the user retry, which triggers a fresh `GetOrSpawn`.

#### 6. Remove soft scoping

- Remove the `[Working in: ...]` message prefix from `ws.go`.
- Remove the automatic scoping first-message from `handleCreateSession`.
- The session map remains for UI purposes (showing which worktree a session belongs to) but no longer drives scoping logic.

#### 7. Non-workspace mode

When `--workspace-dir` is not set (single-project mode), the registry spawns one instance for CWD, behaving identically to today but with voilot managing the process.

### Phase 2: Adapter Interface Cleanup (separate plan)

Deferred to a follow-up plan. Key ideas:
- Extract `RespondToPermission`, `RespondToQuestion`, `RejectQuestion` into an optional `InteractiveProvider` interface.
- Extract `SetSessionAgent`, `GetSessionAgent`, `SetSessionMode`, `GetSessionMode` and similar voilot-level state out of the adapter — these are voilot concepts, not provider concerns.
- Slim the core `Adapter` to: `CreateSession`, `ListSessions`, `DeleteSession`, `SendMessageAsync`, `AbortSession`, `SubscribeEvents`, `GetStatus`.

## Open Questions

- **Port discovery**: When using `--port 0`, how does OpenCode communicate the assigned port? Need to check if it logs it to stdout/stderr or if we need to scan for it another way.
- **Config isolation**: Each OpenCode instance creates its own `.opencode/` directory in the worktree. Per-worktree config is acceptable for now; global OpenCode config (e.g. `~/.config/opencode`) is shared naturally.

## Test Suite

### Required refactors

- Export `reapIdle` on `ProviderRegistry` (or add a `ReapIdle()` wrapper) so tests can trigger idle reaping deterministically without waiting for the background ticker.

### Shared test infrastructure: `backend/internal/agent/agenttest`

A shared package providing mock implementations importable by both `agent` and `api` test files.

**`MockProvider`** — in-memory fake implementing `Provider`:
- Tracks spawned instances (workDir → PID/URL)
- Controllable: can inject spawn failures, unhealthy responses
- Assigns fake PIDs and base URLs
- `Stop` just removes from internal tracking

**`MockAdapter`** — fake implementing `Adapter`:
- `SubscribeEvents` returns a channel the test can push events into
- `ListSessions` returns configurable session lists
- Other methods return sensible defaults or configurable responses
- Tracks calls for assertion (e.g. "was AbortSession called?")

### Test file: `backend/internal/agent/registry_test.go`

**GetOrSpawn lifecycle:**
- Spawn a new instance for a worktree path, verify adapter returned
- Second call for same worktree returns the same adapter (no re-spawn)
- Different worktree spawns a new instance

**LRU eviction:**
- Set `WithMaxInstances(2)`, spawn instances for A, B
- Touch activity on A (making B the LRU)
- Spawn C — verify B was evicted, A and C remain
- All-busy error: spawn 2 instances, mark both busy, attempt third — verify error returned (not eviction)

**Idle reaping:**
- Set `WithIdleTimeout(50ms)`, spawn an instance
- Call exported `ReapIdle()` immediately — instance should survive (not idle long enough)
- Sleep 60ms, call `ReapIdle()` again — instance should be reaped
- Instance with active work (`MarkBusy`) should never be reaped regardless of idle timeout

**PID file orphan cleanup:**
- Spawn a real `sleep 9999` process
- Write a PID file for it in the registry's PID directory
- Create a new `ProviderRegistry` (triggers `sweepOrphans`)
- Verify the sleep process was killed and the PID file removed
- Also test with a stale PID file (process already dead) — verify file is cleaned up without error

**SSE aggregation:**
- Spawn two instances via MockProvider (returns MockAdapters with controllable event channels)
- Subscribe to registry's aggregated `SubscribeEvents` channel
- Push events into each mock adapter's channel
- Verify all events arrive on the aggregated channel
- Stop one instance, verify remaining events still flow

**Activity tracking:**
- `TouchActivity` updates `LastActivity`
- `MarkBusy` / `MarkIdle` correctly track `ActiveCount` (reflected in `Instance.IsIdle()`)

**Concurrency:**
- Parallel `GetOrSpawn` calls for the same worktree — verify only one spawn occurs (no double-spawn race)

### Test file: `backend/internal/api/handlers_test.go`

All tests use `httptest.Server` with a real `Server` struct, `MockProvider`-backed `ProviderRegistry`, and real `sessionmap.Map` (temp file).

**`resolveAdapter` pattern (session ID → worktree → adapter):**
- `GET /api/sessions/{id}` — valid session in session map → 200
- `GET /api/sessions/{id}` — unknown session ID (no mapping) → error response
- `POST /api/sessions/{id}/abort` — valid session → adapter's `AbortSession` called

**`resolveAdapterForWorktree` pattern (worktree path → adapter):**
- `GET /api/sessions?worktree=/path` — spawns instance, returns sessions
- `POST /api/sessions` with `worktreePath` in body — creates session, stores mapping

**`anyAdapter` pattern (any running instance):**
- `GET /api/agents` with running instance → 200 with agent list
- `GET /api/agents` with no running instances → error response

**Multi-instance aggregation:**
- `GET /api/projects` with two running instances, each with sessions — verify `lastActivity` computed correctly across both instances

**Instance management:**
- `GET /api/instances` — returns list of running instances with correct fields
- `POST /api/instances/stop` with valid workDir → instance stopped, 200
- `POST /api/instances/stop` with unknown workDir → error response

## Acceptance Criteria

- [ ] Voilot spawns a dedicated OpenCode process per active worktree on first session creation or worktree navigation.
- [ ] Each OpenCode instance runs in the worktree's directory with its own port.
- [ ] Sessions route to the correct OpenCode instance based on worktree mapping.
- [ ] Max concurrent instances is configurable via `VOILOT_MAX_INSTANCES` env var (default 5), with LRU eviction of idle instances.
- [ ] Idle instances (no active sessions, no in-flight work) are torn down after 10 minutes (configurable).
- [ ] Explicit kill button in the worktree UI to stop an instance on demand.
- [ ] Soft scoping (`[Working in: ...]` prefix) is removed.
- [ ] CLI session resumption works from the worktree directory (since OpenCode runs there).
- [ ] Non-workspace mode (no `--workspace-dir`) still works with a single auto-managed instance.
- [ ] Graceful shutdown stops all managed OpenCode processes and cleans up PID files.
- [ ] Startup sweep kills orphaned OpenCode processes from previous crashes via PID files.
- [ ] Crashed instances surface an error to the frontend; no auto-respawn.
- [ ] SSE streams from all instances are aggregated into one internal bus; no frontend changes needed.
- [ ] `agenttest` package provides shared MockProvider and MockAdapter for both `agent` and `api` tests.
- [ ] Registry unit tests cover: GetOrSpawn lifecycle, LRU eviction (including all-busy error), idle reaping, PID orphan cleanup (real process), SSE aggregation, activity tracking, and concurrent spawn safety.
- [ ] API handler tests cover: all three resolve patterns (`resolveAdapter`, `resolveAdapterForWorktree`, `anyAdapter`), multi-instance session aggregation, and instance management endpoints.
- [ ] `go test ./...` passes with all new tests.
