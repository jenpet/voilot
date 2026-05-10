# Plan: Provider Environment Variable Injection + Config Hot-Reload

**Status:** ready
**Created:** 2026-05-10
**Author:** jenpet + planitect

## Goal

Allow voilot to pass environment variables (API tokens, credentials) to spawned OpenCode instances via provider config, and hot-reload config changes without restarting the backend. Solves the immediate problem where spawned OpenCode instances fail to authenticate because the backend process lacks required env vars like `AWS_BEARER_TOKEN_BEDROCK`.

## Context

- OpenCode providers (e.g. GenAI Nexus via `@ai-sdk/amazon-bedrock`) require authentication tokens passed as environment variables.
- The voilot backend spawns OpenCode via `exec.Command`, which inherits the parent process's env. If the backend wasn't started with the required vars, spawned instances fail with auth errors.
- Tokens may rotate, requiring updates without restarting the server.
- Users want multiple provider configurations (same binary, different env) and the ability to iterate on provider setup without restarts.

## Approach

### 1. Add `env` field to provider config

Add `Env map[string]string` to `ProviderConfig`. Values are literal strings stored directly in `~/.config/voilot/config.json`.

```json
{
  "providers": {
    "opencode": {
      "type": "opencode",
      "binary": "opencode",
      "env": {
        "AWS_BEARER_TOKEN_BEDROCK": "adcf7bdb-7b8e-43ad-8f2a-521b81bcd4f0"
      }
    }
  }
}
```

**Validation rules:**
- Key names must match `^[A-Za-z_][A-Za-z0-9_]*$`
- Values must be non-empty strings
- Validation applied at startup and on every hot-reload

### 2. Pass env to spawned instances

In `OpenCodeProvider.Spawn()`, build `cmd.Env` by merging `os.Environ()` (parent env) with the provider's `env` map. Provider values override same-named parent vars.

**Changes to `OpenCodeProvider`:**
- Add `env map[string]string` field to struct
- Update constructor: `NewOpenCodeProvider(binaryPath string, env map[string]string)`
- In `Spawn()`: if `env` is non-nil, set `cmd.Env = mergedEnv(os.Environ(), p.env)`

### 3. Config file watcher (`config.Watcher`)

New `Watcher` type in the `config` package using stdlib `os.Stat` polling (2-second interval). No external dependencies.

**Interface:**
```go
type ConfigChange struct {
    Old *Config
    New *Config
}

type Watcher struct { /* unexported fields */ }

func NewWatcher(path string, interval time.Duration) *Watcher
func (w *Watcher) Start(ctx context.Context)
func (w *Watcher) Changes() <-chan ConfigChange
```

**Behavior:**
- Polls file mtime every 2 seconds
- On change: re-reads, parses, validates the entire config
- Atomic reload: if parse or validation fails, log error, keep old config, do not emit on channel
- If config file is deleted/missing: log error, keep old config
- Stops cleanly when context is cancelled

### 4. Registry `ReloadProviders` method

Add a method to `ProviderRegistry` that accepts a new provider map and default provider name:

```go
func (r *ProviderRegistry) ReloadProviders(providers map[string]Provider, defaultProvider string)
```

**Behavior:**
- Deep-compare old vs new `ProviderConfig` for each provider name
- **Changed provider** (env or binary differs): SIGTERM all running instances for that provider (fire-and-forget), replace the provider in the registry
- **Removed provider**: SIGTERM all running instances, remove from registry
- **New provider**: register immediately, available for next spawn
- **Unchanged provider**: no action, running instances untouched
- Brief write lock during reload (milliseconds)
- Update default provider name

### 5. Hot-reload scope

| Config field | Hot-reloadable | Notes |
|---|---|---|
| `providers[x].env` | Yes | Kill affected instances, new spawns get fresh env |
| `providers[x].binary` | Yes | Kill affected instances, new spawns use new binary |
| New provider added | Yes | Registered immediately |
| Provider removed | Yes | Instances killed, provider deregistered |
| `defaultProvider` | Yes | Affects next session without explicit provider |
| `maxInstances` | Yes | Don't evict, just stop spawning until count drops |
| `idleTimeout` | Yes | Apply to future idle checks |
| `ttsUrl` / `sttUrl` | Yes | Next TTS/STT call uses new URL |
| `workspace` | No | Log warning, requires restart |

For non-reloadable fields that changed, log a message indicating the change will take effect after the next restart.

### 6. Wiring in `main.go`

```go
watcher := config.NewWatcher(cfgPath, 2*time.Second)
go watcher.Start(ctx)
go func() {
    for change := range watcher.Changes() {
        newProviders := buildProviders(change.New)
        registry.ReloadProviders(newProviders, change.New.DefaultProvider)
        // Update other hot-reloadable fields (TTS/STT URLs, etc.)
    }
}()
```

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Env value format | Literal values in config | No indirection needed; same security as any `~/.config` file |
| File watching mechanism | `os.Stat` polling (2s) | No external deps; config changes are rare; mtime comparison is naturally idempotent (no debouncing needed) |
| Effect on running instances | SIGTERM on provider change | Clean slate for new env; old instances would hit auth errors anyway |
| SIGTERM delivery | Fire-and-forget | Don't wait for process exit; no port conflicts (`--port 0`) |
| Race (spawn vs reload) | Let it race | Extremely rare; user gets an error and retries |
| Deleted config file | Ignore, keep running config | Likely accidental; don't nuke running state |
| Reload atomicity | All-or-nothing | Partial application is confusing to debug |
| Reload feedback | Console logging (`slog`) only | Operator sees terminal; no UI complexity |
| Watcher interface | Channel-based (`<-chan ConfigChange`) | Idiomatic Go; testable; composable |
| Registry locking during reload | Brief write lock | Milliseconds; acceptable latency |
| Workspace change | Not hot-reloaded | Dangerous; log warning to restart |

## Files to Change

| File | Changes |
|------|---------|
| `backend/internal/config/config.go` | Add `Env map[string]string` to `ProviderConfig`; add env key/value validation in `Validate()` |
| `backend/internal/config/watcher.go` | New: `Watcher` struct with `Start(ctx)`, `Changes() <-chan ConfigChange`, mtime polling loop |
| `backend/internal/config/watcher_test.go` | New: polling detection, change emission, invalid file handling, deleted file handling |
| `backend/internal/config/config_test.go` | Extend: env key format validation, empty value rejection |
| `backend/internal/agent/opencode_provider.go` | Add `env map[string]string` field; update constructor; merge env into `cmd.Env` in `Spawn()` |
| `backend/internal/agent/registry.go` | Add `ReloadProviders(providers, defaultProvider)` method with deep-compare, SIGTERM, re-register |
| `backend/internal/agent/registry_test.go` | Extend: reload logic tests, integration test spawning `sleep` process and verifying SIGTERM |
| `backend/cmd/server/main.go` | Pass `pc.Env` to provider; wire watcher to registry reload loop |
| `config.example.json` | Add `env` example with placeholder value |

## Testing

### Unit Tests
- Config validation: valid env keys, invalid env keys (digits-first, spaces, special chars), empty values rejected
- Config reload: changed file emits on channel, unchanged file does not, deleted file logged and ignored, malformed JSON rejected

### Integration Test (registry reload + process kill)
- Mock provider spawns `sleep 3600` as a real subprocess
- Registry spawns instance, PID tracked
- Call `ReloadProviders` with changed env for that provider
- Assert old process received SIGTERM and is no longer alive
- Assert registry has updated provider registered
- Assert new spawn uses updated env

## Open Questions

None remaining.

## Acceptance Criteria

- [ ] Provider config accepts `env` map with literal key/value pairs
- [ ] Env validation rejects invalid key names and empty values at startup and reload
- [ ] Spawned OpenCode instances receive merged environment (parent + provider env)
- [ ] Config file changes detected within ~2 seconds via mtime polling
- [ ] Changed/removed providers trigger SIGTERM of their running instances
- [ ] New providers available immediately after config reload
- [ ] Invalid config file changes are rejected with console error; running config preserved
- [ ] Deleted config file is ignored; running config preserved
- [ ] `workspace` field change logs a "restart required" message
- [ ] Integration test verifies process kill on provider reload
- [ ] `config.example.json` updated with env example
