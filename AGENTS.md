# AGENTS.md - Coding Agent Guidelines for Voilot

## Project Overview

Voilot (voice + pilot) is a voice-first, mobile-first PWA client for interacting with AI coding agents. The core workflow: **plan via voice conversations** (from mobile/desktop), then **implement later** at the computer. Currently targets OpenCode as the agent backend, with Claude Code support planned.

### Architecture

```
Browser (PWA) — Nuxt 3 + Vue + Tailwind, dark theme, mobile-first
       | HTTPS (Tailscale CLI — tailscaled, no GUI/network extension)
  :443 → Nuxt server (host, port 3000)
  :8080 → Go backend  (host, port 8080)
       |
Go Backend — REST API, WebSocket, agent adapter, voice router
       |
  +----+----------------+
  |    |                 |
Kokoro TTS (:8880)   faster-whisper STT (:5003)   OpenCode (spawned per worktree)
   (Docker)              (Docker)                   (host, child process)
```

Backend and frontend run natively on the host (macOS launchd in production,
air/npm in dev). Only TTS and STT run in Docker containers. No nginx.
See `docs/adr/0001-host-native-backend-frontend-docker-for-voice.md`.

### Key Concepts

- **Plan vs Implement mode** — "plan" restricts the agent to discussion only (via system prompt prepended to messages); "implement" gives full tool access. This is a voilot-level concept, not native to OpenCode. Session modes are stored in-memory in the Go backend (ephemeral, reset on restart).
- **Voice command router** — keyword detection separates app commands (mode switch, new session, stop) from agent messages (forwarded as natural language).
- **Smart TTS filter** — speaks text, summarizes code blocks ("Wrote 45 lines of TypeScript"), skips thinking blocks, announces errors.
- **Pluggable agent adapters** — Go interface (`agent.Adapter`) that backends implement. OpenCode is the first; Claude Code and Pi are potential next candidates for integration.
- **Multi-provider registry** — `ProviderRegistry` manages multiple named providers, each spawning agent instances per worktree. Instance keys are `(worktreePath, providerName)`. Symlinks in worktree paths are resolved at the registry boundary to prevent path mismatches.
- **Config file** — `~/.config/voilot/config.json` (or `--config` override). Defines workspace directory, providers, TTS/STT URLs, instance limits. No env vars — config file is the single source of truth. See `config.example.json`.

### Design Constraints

- **No React** — Vue.js / Nuxt 3 for the frontend
- **Go backend** — single binary, REST + WebSocket API only
- **Host-native** — backend and frontend run on the host, not in Docker
- **Open-source models only** for TTS and STT, self-hosted via Docker
- **Docker** only for voice services (TTS + STT)

## Repository Structure

```
voilot/
├── backend/
│   ├── cmd/server/main.go       # Entrypoint, config loading, CLI flags (--config, --data-dir, etc.)
│   ├── go.mod                   # Module: github.com/jenpet/voilot
│   └── internal/
│       ├── agent/               # Adapter interface, registry, provider implementations
│       │   ├── adapter.go       # Adapter interface, Session, SessionMode, ScopePrompt
│       │   ├── registry.go      # ProviderRegistry: multi-provider instance lifecycle
│       │   ├── provider.go      # Provider interface
│       │   ├── opencode.go      # Full OpenCode HTTP client + SSE reader
│       │   ├── opencode_provider.go # OpenCode Provider implementation (spawn/stop)
│       │   ├── worktree_defaults.go # Per-worktree default provider persistence
│       │   └── events.go        # Event types, OpenCode SSE wire-format structs
│       ├── api/                 # HTTP routes, handlers, WebSocket
│       │   ├── router.go        # Route registration
│       │   ├── handlers.go      # REST handlers (sessions, STT, TTS, abort, mode)
│       │   └── ws.go            # WebSocket chat handler + voice stub (/ws/voice is still a stub)
│       ├── tts/                 # TTS provider interface + Kokoro implementation
│       │   ├── provider.go      # Provider interface (Synthesize, ListVoices, Name)
│       │   └── kokoro.go        # Kokoro-FastAPI client (OpenAI-compatible API)
│       ├── stt/                 # STT provider interface + implementation
│       │   ├── provider.go      # Provider interface
│       │   └── whisper.go       # faster-whisper HTTP client
│       └── voice/               # Voice processing pipeline
│           ├── router.go        # Voice command keyword router
│           └── filter.go        # TTS text filter logic
├── frontend/
│   ├── nuxt.config.ts           # No proxies, backendUrl runtimeConfig, PWA config
│   ├── app.vue                  # Dark shell + NuxtPage
│   ├── pages/
│   │   ├── index.vue            # Session list, connection status
│   │   └── session/[id].vue     # Chat page with abort, voice, mode toggle
│   ├── components/              # ChatView, ChatMessage, SessionCard, ModeToggle, VoiceButton, AudioPlayer
│   └── composables/
│       ├── useWebSocket.ts      # HMR-safe shared WS connection (state on window.__voilot_ws)
│       ├── useAgent.ts          # WebSocket-based agent communication with TTS
│       ├── useSession.ts        # Session CRUD via REST
│       ├── useVoice.ts          # MediaRecorder + STT REST API (sends multipart form data)
│       ├── useTTS.ts            # Sequential queue-based TTS playback
│       └── useTTSFilter.ts      # Client-side TTS text filter
├── deploy/
│   ├── deploy.sh                # Build backend + frontend, restart services, docker compose voice
│   ├── update.sh                # Git fetch/compare, deploy on changes, idle sleep
│   ├── docker/
│   │   ├── docker-compose.yml   # TTS + STT only (voice services)
│   │   ├── Dockerfile.stt       # Python 3.11 + faster-whisper + gunicorn
│   │   └── stt-server.py        # Flask app wrapping faster-whisper (/transcribe, /health)
│   └── macos/
│       ├── setup.sh             # One-time macOS production setup
│       ├── README.md            # Setup docs and prerequisites
│       ├── run-backend.sh       # Wrapper: sources .env, execs voilot-server
│       ├── run-frontend.sh      # Wrapper: sources .env, execs node server
│       ├── run-update.sh        # Wrapper: sets PATH, execs update.sh
│       └── plists/              # launchd plist templates
└── README.md
```

## Build, Lint, and Test Commands

### Go Backend

```bash
cd backend
go mod download          # Install dependencies
go build ./...           # Verify compilation
go build ./cmd/server    # Build server binary
go test ./...            # Run all tests
go test ./internal/agent -run TestName  # Run specific test
```

**Local dev setup:**

```bash
cp .env.example .env     # then edit API keys and paths
task install             # installs deps, dev tools (air), creates ~/.config/voilot/config.json
task dev                 # starts everything (frontend, backend, voice, tunnel)
```

The `.env` file provides: `ANTHROPIC_API_KEY`, `WORKSPACE_DIR`, `DATA_DIR`, `TTS_URL`, `STT_URL`. The Taskfile loads `.env` automatically via `dotenv` and passes these as CLI flags to the backend.

The backend uses [air](https://github.com/air-verse/air) for hot reload — saving a `.go` file triggers automatic rebuild and restart. Note that restarts kill all running OpenCode instances; they respawn on the next request.

**Backend CLI flags** (handled by `.env` + Taskfile for local dev):

```
--config <path>           Config file (default: ~/.config/voilot/config.json)
--data-dir <path>         Runtime data directory (session map, PID files)
--tts-url <url>           TTS server URL
--stt-url <url>           STT server URL
--workspace-dir <path>    Workspace directory
--port <n>                Listen port (default: 8080)
--hostname <addr>         Listen address (default: 127.0.0.1)
--cors-origins <csv>      Allowed CORS origins
--log-level <level>       debug, info, warn, error (default: info)
--log-format <fmt>        text, json (default: text)
```

### Nuxt Frontend

```bash
cd frontend
npm install              # Install dependencies
npm run dev              # Dev server at http://localhost:3000
npx nuxt build           # Production build -> .output/server/
```

### Docker (voice services only)

```bash
cd deploy/docker
docker compose -p voilot --profile voice up --build    # TTS + STT
docker compose -p voilot --profile voice down           # Stop
```

Voice services (TTS + STT) run in Docker. Backend and frontend run on the host.
See `docs/adr/0001-host-native-backend-frontend-docker-for-voice.md`.

## Code Style Guidelines

### Go

- Standard library first, third-party second, internal third
- Use `context.Context` for all handler/provider methods
- Error wrapping with `fmt.Errorf("...: %w", err)`
- Interfaces in separate files from implementations
- Verify interface compliance: `var _ Provider = (*KokoroProvider)(nil)`

### TypeScript/Vue (Frontend)

- **Indentation**: 2 spaces
- **Quotes**: Single quotes
- **Semicolons**: Yes
- **Imports**: Standard, third-party, local (alphabetical within groups)
- **Types**: Use `import type` for type-only imports; avoid `any`, use `unknown`
- **Naming**: kebab-case files, PascalCase components, camelCase functions, UPPER_SNAKE_CASE constants
- Nuxt auto-imports (`ref`, `useState`, `$fetch`, `useRuntimeConfig`, `readonly`) resolve after `npm install` generates `.nuxt/` types — LSP errors before that are expected

### Commits

- Conventional commits: `type: message` or `type(scope): message`
- Types: feat, fix, docs, refactor, test, chore
- Keep commit messages short and concise
- Limit the subject line to 50 characters
- Capitalize the subject/description line
- Do not end the subject line with a period
- Use imperative mood in the subject line
- Include a body only when it adds important context
- If a body is included, separate it from the subject with a blank line
- Wrap commit body lines at 72 characters
- Maintain a linear history on `main`
- Only use rebase-based updates and fast-forward merges
- Do not create merge commits

## External Service APIs

### OpenCode API (`opencode serve`, default port 4096)

The Go backend communicates with OpenCode via HTTP REST + SSE.

**REST Endpoints:**
- `GET /global/health` -> `{healthy: bool, version: string}`
- `GET /session` -> `Session[]`
- `POST /session` -> `Session` (body: `{title?, parentID?}`)
- `GET /session/:id` -> `Session`
- `DELETE /session/:id` -> `boolean`
- `POST /session/:id/message` -> synchronous (waits for full response)
- `POST /session/:id/prompt_async` -> async (returns 204, events via SSE)
- `POST /session/:id/abort` -> abort running session
- `GET /event` -> SSE stream

**SSE Event Types:**
- `message.part.updated` — full content snapshots (text, reasoning, tool parts)
- `message.part.delta` — incremental `delta` field for streaming text (**separate event type** from updated)
- `session.status` — idle/busy/retry
- `session.idle` — done signal
- `session.created`, `session.updated`, `session.error`
- `message.updated` — role tracking (user vs assistant); also carries error info
- `server.connected`, `server.heartbeat`, `session.diff` — ignored

**OpenCode Part Types:** `text`, `reasoning`, `tool` (state: pending/running/completed/error), `step-start`, `step-finish`, `file`, `subtask`, `snapshot`, `patch`, `agent`, `retry`, `compaction`

**Critical SSE Behavior:**
- OpenCode echoes the user's own message as `message.updated` (role: "user") + `message.part.updated` (type: "text"). These MUST be filtered by tracking user message IDs from `message.updated` where `role === "user"`, then skipping any `message.part.updated` and `message.part.delta` events for those IDs.
- `message.part.updated` for text sends both initial empty (`text: ""`) and final full updates. Skip the initial empty update to avoid clearing accumulated delta content.
- User message IDs have a 30-minute TTL with periodic cleanup to prevent unbounded growth.

### Kokoro-FastAPI TTS (default, port 8880)

Docker image: `ghcr.io/remsky/kokoro-fastapi-cpu:latest` (82M param model, Apache 2.0)

**Endpoints:**
- `POST /v1/audio/speech` — OpenAI-compatible TTS
  - Body: `{"model": "kokoro", "input": "text", "voice": "af_heart", "response_format": "wav", "speed": 1.0}`
  - Returns: audio stream (WAV), Content-Type from response header
- `GET /v1/audio/voices` — list available voices
  - Returns: `{"voices": ["af_heart", "af_bella", ...]}`

Default voice: `af_heart`, model: `kokoro`, 24kHz mono WAV output.
Performance on CPU: ~0.5s for 5 words, ~1.9s for 20 words, ~4.7s for 50 words.

### faster-whisper STT (port 5003)

Custom Flask sidecar wrapping the faster-whisper library.

**Endpoints:**
- `POST /transcribe` — multipart form: `file` field with audio data
  - Returns: `{"text": "transcribed text", "confidence": 0.99, "language": "en"}`
- `GET /health` -> `{"status": "ok"}`

## Frontend URL Architecture

- All composables use `resolveBackendUrl()` from `composables/useBackendUrl.ts`
- Dev on localhost: returns `http://localhost:8080` (from `config.public.backendUrl`)
- Dev on LAN IP: auto-rewrites to use the browser's actual hostname (e.g., `http://192.168.178.93:8080`)
- Production: same as dev — `http://localhost:8080`, rewritten by `resolveBackendUrl()` for cross-device access
- WebSocket composable: absolute URL in dev, `window.location`-derived in production
- No proxying in Nuxt config — Nitro devProxy and Vite server.proxy both crash with ECONNRESET when backend is unavailable

## HTTPS / Network Access

- HTTPS is required for mobile mic access (`getUserMedia` needs a secure context)
- TLS termination is handled by **Tailscale CLI** (`tailscale serve`, Homebrew formula — no GUI app/network extension)
- `tailscaled` runs as a system LaunchDaemon; state stored at `/var/lib/tailscale`
- Two Tailscale HTTPS rules: `:443` → frontend (`:3000`), `:8080` → backend (`:8080`)
- Both services bind to `127.0.0.1` — Tailscale proxies locally
- For local dev: `task dev:tunnel` applies the same serve rules (idempotent)
- Access from phone via `https://<machine>.<tailnet>.ts.net` (Tailscale must be installed on both devices)
- See `docs/adr/0002-tailscale-cli-over-gui-app.md` for rationale (GUI app's network extension breaks LAN)

## Deployment Notes

- Backend and frontend run on the host as macOS launchd services (production) or via air/npm (dev)
- TTS and STT run in Docker containers (`deploy/docker/docker-compose.yml`)
- `deploy/deploy.sh` builds backend + frontend, restarts launchd, brings up voice containers
- `deploy/update.sh` polls `origin/main`, calls `deploy.sh` on changes, sleeps Mac on idle
- Update service runs as a launchd plist (`pet.jen.voilot.update`) polling every 30 seconds
- `deploy/macos/setup.sh` is a one-time setup for macOS production (pmset, launchd plists, Tailscale daemon, gh auth)
- GitHub CLI (`gh`) is required for git credential management in non-interactive launchd context
- Build hash/time injected via Go ldflags (backend) and `NUXT_PUBLIC_*` env vars (frontend)

---

## Debug Log Analysis

When the user uploads a debug log file (JSON or extracted from a `.zip`) and describes an observed issue (e.g. "mic stopped working", "TTS never played", "voice loop didn't restart"), follow this procedure:

1. **Read the interaction state reference** at `docs/frontend-interaction-state-machine.md` first. This document contains the full state machine (12 states, allowed transitions), the component registry (which components are allowed to log in which states), and common failure patterns with diagnostic guidance.

2. **Parse the debug log entries** and trace the sequence of `state` transitions and component events leading up to the reported issue. Pay attention to:
   - Whether state transitions follow the allowed transition table
   - Whether the `component` field matches what's expected for the current `state`
   - Gaps in expected events (e.g. missing `rms_sample` entries, missing `synth_complete` after `synth_start`)
   - `warn` and `error` level entries around the time of the issue

3. **Cross-reference** the observed event sequence against the state-component permission matrix and transition rules in the reference doc to identify the root cause.

4. **Report findings** with specific log entry timestamps, the state at the time of failure, and which transition or component event was missing or unexpected.

---

*This file should be updated as the project evolves.*
