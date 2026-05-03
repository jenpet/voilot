# Plan: Multi-Provider Configuration

**Status:** draft
**Created:** 2026-05-03
**Author:** jenpet + planitect

## Goal

Introduce a config file and multi-provider support so voilot can manage different agent backends (OpenCode, Claude Code, Pi in the future). Users can set a global default provider, override it per worktree, and pick a provider when creating a session. Only OpenCode has a concrete implementation for now; the config schema and registry are structured for future providers.

## Context

Phase 1 (provider-registry, `5ac04f3`) introduced `ProviderRegistry` with a single `Provider`. The registry keys instances by `workDir` only, holds one provider, and has no config file — everything comes from CLI flags and env vars. This phase adds the config layer and multi-provider plumbing.

**Dependencies:** Phase 1 provider-registry (complete).

## Config File

**Location:** `~/.config/voilot/config.json`, overridable via `--config` CLI flag.

**Schema:**

```json
{
  "workspace": "/Users/jenpet/workspace",
  "defaultProvider": "opencode",
  "providers": {
    "opencode": {
      "type": "opencode",
      "binary": "opencode"
    }
  },
  "maxInstances": 5,
  "idleTimeout": "10m",
  "ttsUrl": "http://localhost:8880",
  "sttUrl": "http://localhost:5003"
}
```

- `type` maps to a built-in spawn strategy. Supported types: `opencode`. Future: `claude-code`, `pi`.
- `binary` overrides the default binary name for that type.
- `defaultProvider` is the global fallback when no per-worktree default is set.
- `maxInstances` is global across all providers (shared pool).

**Future provider types (no implementation yet):**
- `claude-code` — Claude Code CLI (`claude`).
- `pi` — Pi coding agent.

The server validates that configured provider types have implementations. Unsupported types log a warning at startup but don't prevent the server from starting.

## Config Precedence

```
hardcoded defaults < config file < CLI flags
```

No environment variables. `VOILOT_MAX_INSTANCES` env var is removed.

**CLI flags that remain (deployment-specific):**
- `--port`, `--hostname`, `--data-dir`, `--cors-origins`, `--config`

**CLI flags removed (moved to config, still available as CLI overrides):**
- `--opencode-binary` → `providers.opencode.binary` in config
- `--tts-url` → `ttsUrl` in config
- `--stt-url` → `sttUrl` in config
- `--workspace-dir` → `workspace` in config

**Missing config behavior:** If no config file exists and no `--config` flag is provided, the server prints an error with a sample config and the expected path, then exits. No interactive setup wizard — a sample `config.example.json` is provided in the repo alongside documentation.

## Runtime State Files

All under `<data-dir>/` (CLI flag, default `voilot-data/`):

| File | Key | Purpose |
|------|-----|---------|
| `session-map.json` | sessionID | Worktree path, title, timestamp, **provider name** |
| `worktree-defaults.json` | worktree path | Default provider name per worktree |
| `pids/` | — | Ephemeral PID files, cleaned on startup |

**`worktree-defaults.json` example:**

```json
{
  "/Users/jenpet/workspace/voilot": "opencode",
  "/Users/jenpet/workspace/voilot-feature-branch": "opencode"
}
```

This is runtime state mutated via the UI, not user-edited config.

## Registry Changes

- `ProviderRegistry` holds `map[string]Provider` (keyed by provider name) instead of a single `Provider`.
- Instance key becomes composite: `(workDir, providerName)`.
- `GetOrSpawn(ctx, workDir, providerName)` replaces `GetOrSpawn(ctx, workDir)`.
- `maxInstances` limit is global across all providers.
- LRU eviction considers all instances regardless of provider.

## Session Changes

- `Session` struct gains `Provider string` field.
- `sessionmap.Entry` gains `Provider string` field.
- Sessions display a provider badge in the frontend.

## API Changes

- `handleCreateSession` body gains optional `provider` field. Defaults to worktree default, then global default.
- New endpoints for worktree provider defaults:
  - `GET /api/worktrees/:path/provider` — get worktree default provider.
  - `PUT /api/worktrees/:path/provider` — set worktree default provider.
- Provider list endpoint: `GET /api/providers` — returns configured providers.

## Frontend Changes

- **Session creation form:** provider dropdown, pre-filled with worktree default. Same form pattern as worktree creation.
- **Worktree creation form:** gains a provider dropdown.
- **Worktree page:** gear icon near "new session" button to change the worktree's default provider.
- **Session list:** provider badge on each session card.

## Approach

### Step 1: Config package

Create `backend/internal/config/` package:
- `config.go` — struct definition, `Load(path string) (*Config, error)`, validation.
- Default path resolution: `~/.config/voilot/config.json`.
- CLI flag merge: accept a `*Config` and override non-zero CLI flag values.
- Error with sample config on missing file.

### Step 2: Wire config into main.go

- Replace individual CLI flags with config loading.
- Keep `--port`, `--hostname`, `--data-dir`, `--cors-origins`, `--config`.
- Remove `--opencode-binary`, `--tts-url`, `--stt-url`, `--workspace-dir`.
- Remove `VOILOT_MAX_INSTANCES` env var.
- Construct provider from config: `providers["opencode"]` → `NewOpenCodeProvider(binary)`.

### Step 3: Multi-provider registry

- Change `ProviderRegistry` to accept `map[string]Provider`.
- Change instance key to `(workDir, providerName)` composite.
- Update `GetOrSpawn`, `StopInstance`, `GetAdapter` signatures.
- Update all call sites (resolve.go, handlers, ws).

### Step 4: Worktree defaults

- Create `<data-dir>/worktree-defaults.json` management (load/save/get/set).
- API endpoints for get/set worktree default provider.
- Session creation uses worktree default → global default fallback chain.
- Worktree creation optionally sets a default.

### Step 5: Session provider tracking

- Add `Provider` field to `Session` struct and `sessionmap.Entry`.
- Store provider name on session creation in session map.
- Return provider in session API responses.

### Step 6: Frontend

- Provider dropdown component (reusable for session creation and worktree creation forms).
- Gear icon on worktree page for default provider settings.
- Provider badge on session cards.
- `GET /api/providers` integration for populating dropdowns.

### Step 7: Sample config and docs

- `config.example.json` in repo root.
- Update `README.md`, `AGENTS.md`, `docker-compose.yml` with new config-based setup.
- Document config schema and CLI flag changes.

## Open Questions

None — all design decisions resolved.

## Acceptance Criteria

- [ ] Server loads config from `~/.config/voilot/config.json` (or `--config` override).
- [ ] Server exits with sample config error when no config found.
- [ ] CLI flags `--tts-url`, `--stt-url`, `--workspace-dir`, `--opencode-binary` removed; values come from config with CLI override support for deployment-specific flags.
- [ ] `VOILOT_MAX_INSTANCES` env var removed, read from config.
- [ ] Registry supports multiple providers keyed by `(workDir, providerName)`.
- [ ] Per-worktree default provider stored in `worktree-defaults.json`, settable via API.
- [ ] Sessions track provider name in session map and API responses.
- [ ] Frontend shows provider dropdown on session and worktree creation.
- [ ] Frontend shows gear icon on worktree page for default provider.
- [ ] Frontend shows provider badge on session cards.
- [ ] `config.example.json` and updated docs in repo.
