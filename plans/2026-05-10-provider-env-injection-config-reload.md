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

Add `Env map[string]string` to `ProviderConfig`. Values can be either literal strings or `${VAR_NAME}` references that expand from the backend process's environment.

```json
{
  "providers": {
    "opencode": {
      "type": "opencode",
      "binary": "opencode",
      "env": {
        "AWS_BEARER_TOKEN_BEDROCK": "${AWS_BEARER_TOKEN_BEDROCK}",
        "CUSTOM_FLAG": "some-literal-value"
      }
    }
  }
}
```

**Value formats (mutually exclusive per entry):**
- **Literal string**: any value that does not contain `${` — stored and passed as-is
- **Single `${VAR_NAME}` reference**: the entire value is exactly `${VAR_NAME}` — expanded from `os.Getenv("VAR_NAME")` at config load/reload time. This is a "keep secrets off disk" convenience; the backend's process env is frozen at startup, so these do NOT benefit from hot-reload.

**Validation rules (applied at startup and on every hot-reload):**
- Key names must match `^[A-Za-z_][A-Za-z0-9_]*$`
- Values containing `${` must match exactly `^\$\{[A-Za-z_][A-Za-z0-9_]*\}$` (single reference, no mixing)
- After expansion, all values must be non-empty
- Consistent error messages:
  - Literal empty: `provider "X": env["KEY"] must not be empty`
  - `${VAR}` resolves to empty: `provider "X": env["KEY"] references ${VAR} which is not set in the process environment`

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
| Env value format | Literal values OR single `${VAR}` references | Literals for rotating tokens (hot-reloadable); `${VAR}` for stable tokens kept off disk (expanded from process env at load time) |
| `${VAR}` expansion timing | At config load/reload | Process env is frozen; no benefit delaying. Fail fast if var is unset. |
| `${VAR}` and hot-reload | References don't benefit from hot-reload | Process env doesn't change; only literal values in the config file benefit from file-based hot-reload |
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
| `${VAR}` scope | `env` field only | Other config fields (paths, URLs) aren't sensitive and benefit from being explicit |

## Files to Change

| File | Changes |
|------|---------|
| `backend/internal/config/config.go` | Add `Env map[string]string` to `ProviderConfig`; add env key/value validation in `Validate()`; add `${VAR}` reference detection, expansion via `os.Getenv`, and resolved-value validation; expand references during `Load()` before returning |
| `backend/internal/config/watcher.go` | New: `Watcher` struct with `Start(ctx)`, `Changes() <-chan ConfigChange`, mtime polling loop |
| `backend/internal/config/watcher_test.go` | New: polling detection, change emission, invalid file handling, deleted file handling |
| `backend/internal/config/config_test.go` | Extend: env key format validation, empty value rejection, `${VAR}` format validation, expansion success/failure, error message format |
| `backend/internal/agent/opencode_provider.go` | Add `env map[string]string` field; update constructor; merge env into `cmd.Env` in `Spawn()` |
| `backend/internal/agent/registry.go` | Add `ReloadProviders(providers, defaultProvider)` method with deep-compare, SIGTERM, re-register |
| `backend/internal/agent/registry_test.go` | Extend: reload logic tests, integration test spawning `sleep` process and verifying SIGTERM |
| `backend/cmd/server/main.go` | Pass `pc.Env` to provider; wire watcher to registry reload loop |
| `config.example.json` | Add `env` example showing both literal and `${VAR}` usage |
| `README.md` | Add section documenting provider env vars (literal + `${VAR}` expansion), per-provider scoping, hot-reload behavior, and validation rules |

## Testing

### Unit Tests
- Config validation: valid env keys, invalid env keys (digits-first, spaces, special chars), empty literal values rejected
- `${VAR}` reference validation: valid reference format accepted, malformed references rejected (e.g. `${123BAD}`, `${VAR` without closing brace, `prefix${VAR}` mixed content)
- `${VAR}` expansion: reference resolves to process env value, unset var produces specific error message, set-but-empty var produces specific error message
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
- [ ] Provider config accepts `env` map with `${VAR}` references that expand from process env
- [ ] `${VAR}` references must be the entire value (no mixed content like `prefix${VAR}`)
- [ ] Malformed `${VAR}` references are rejected at config load with a clear error
- [ ] Unset/empty `${VAR}` references are rejected at config load with a specific error naming the variable
- [ ] Error messages are structurally consistent between literal and reference validation failures
- [ ] Env validation rejects invalid key names and empty values at startup and reload
- [ ] Spawned OpenCode instances receive merged environment (parent + provider env)
- [ ] Config file changes detected within ~2 seconds via mtime polling
- [ ] Changed/removed providers trigger SIGTERM of their running instances
- [ ] New providers available immediately after config reload
- [ ] Invalid config file changes are rejected with console error; running config preserved
- [ ] Deleted config file is ignored; running config preserved
- [ ] `workspace` field change logs a "restart required" message
- [ ] Integration test verifies process kill on provider reload
- [ ] `config.example.json` updated with env example showing both literal and `${VAR}` usage
- [ ] `README.md` documents provider env var injection (literal + `${VAR}`), per-provider scoping, hot-reload behavior, and validation rules
