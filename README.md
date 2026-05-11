# voilot

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Voice-first, mobile-first PWA client for AI coding agents. Plan via voice conversations from your phone, implement later at your computer.

> **Security warning:** voilot has no built-in authentication. If you expose it to the internet without a VPN, reverse proxy auth, or other access control, anyone can use your coding agent. See [docs/deployment.md](docs/deployment.md#security) for details.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  Browser (PWA)                                       │
│  Nuxt 3 + Vue + Tailwind · dark theme · mobile-first │
└────────────────┬──────────────────────────────────────┘
                 │ HTTP / WebSocket
┌────────────────▼──────────────────────────────────────┐
│  Nginx                                                │
│  Static files · /api/* → backend · /ws/* → backend    │
└────────────────┬──────────────────────────────────────┘
                 │
┌────────────────▼──────────────────────────────────────┐
│  Go Backend (port 8080)                               │
│  REST API · WebSocket · Agent adapter · Voice router   │
├───────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌────────────────────┐  │
│  │ Kokoro   │  │ Whisper  │  │ Agent Backend      │  │
│  │ TTS      │  │ STT      │  │ (OpenCode / Claude)│  │
│  │ :8880    │  │ :5003    │  │ :4096              │  │
│  └──────────┘  └──────────┘  └────────────────────┘  │
└───────────────────────────────────────────────────────┘
```

### Key Concepts

- **Plan vs Implement mode** — "plan" restricts the agent to discussion only; "implement" gives full tool access
- **Voice command router** — separates app commands (mode switch, new session, stop) from agent messages
- **Smart TTS filter** — speaks text, summarizes code blocks, skips thinking, announces errors
- **Pluggable agent adapters** — OpenCode first; Claude Code and Pi are potential next candidates for integration

## Prerequisites

- Docker and Docker Compose
- [OpenCode](https://opencode.ai) running via `opencode serve` (default port 4096)

## Quick Start

```bash
# Start frontend + backend (text-only, no TTS/STT)
cd voilot/docker
docker compose up --build

# Start with voice services (TTS + STT)
docker compose --profile voice up --build
```

The app will be available at `http://localhost:3000`.

Backend configuration is in `docker/config.docker.json` (mounted into the container). Edit it to change provider settings, TTS/STT URLs, workspace path, etc.

### HTTPS (required for mobile mic access)

Use [Tailscale](https://tailscale.com) for TLS termination. Install Tailscale on your
dev machine and phone, then proxy the frontend:

```bash
tailscale serve --bg --https=443 http://localhost:3000
```

Access voilot via `https://<machine>.<tailnet>.ts.net` from any device on your tailnet.

## Configuration

The backend reads its configuration from a JSON file at `~/.config/voilot/config.json` (override with `--config`). Copy `config.example.json` to get started:

```bash
mkdir -p ~/.config/voilot
cp config.example.json ~/.config/voilot/config.json
# Edit the file: set your workspace path, provider binary, TTS/STT URLs
```

See `config.example.json` for all available fields. Key settings:

| Field | Default | Description |
|---|---|---|
| `workspace` | — | Directory containing your git repos (may use symlinks) |
| `defaultProvider` | `"opencode"` | Global default agent provider |
| `providers` | — | Named providers with type and binary path |
| `maxInstances` | `5` | Max concurrent agent instances (shared across providers) |
| `idleTimeout` | `"10m"` | Auto-stop idle instances after this duration |
| `ttsUrl` | `"http://localhost:8880"` | Kokoro TTS server URL |
| `sttUrl` | `"http://localhost:5003"` | faster-whisper STT server URL |

### Provider Environment Variables

Each provider can define environment variables passed to spawned agent instances. This is how you provide API tokens, credentials, or other secrets that the agent backend needs.

```json
{
  "providers": {
    "opencode-anthropic": {
      "type": "opencode",
      "binary": "opencode",
      "env": {
        "ANTHROPIC_API_KEY": "${ANTHROPIC_API_KEY}"
      }
    },
    "opencode-openrouter": {
      "type": "opencode",
      "binary": "opencode",
      "env": {
        "OPENROUTER_API_KEY": "sk-or-v1-your-key-here"
      }
    }
  }
}
```

**Value formats:**

| Format | Example | Behavior |
|--------|---------|----------|
| Literal | `"sk-ant-abc123"` | Passed as-is to spawned instances |
| `${VAR}` reference | `"${ANTHROPIC_API_KEY}"` | Expanded from the backend process environment at config load time |

- Each value must be either a pure literal OR a single `${VAR_NAME}` reference (no mixing).
- Key names must be valid env var identifiers (`A-Z`, `a-z`, `0-9`, `_`, cannot start with a digit).
- Empty values and unresolvable `${VAR}` references are rejected at startup.
- `${VAR}` references expand from the backend's process environment (set in your shell profile). They keep secrets off disk but do not benefit from hot-reload since process env is frozen at startup.

### Config Hot-Reload

The backend watches the config file for changes (2-second polling). When the file is modified:

- **Provider env or binary changed** — running instances for that provider are stopped (SIGTERM) and the next session creation spawns a fresh instance with the new config.
- **New provider added** — available immediately for new sessions.
- **Provider removed** — running instances are stopped, provider is deregistered.
- **`maxInstances`, `idleTimeout`, `ttsUrl`, `sttUrl`, `defaultProvider`** — applied immediately.
- **`workspace`** — requires a backend restart (logged as a warning).

Invalid config changes (malformed JSON, validation errors) are rejected and the running config is preserved. All reload activity is logged to the console.

**Docker-specific env vars** (set in `.env` or pass to `docker compose`):

| Variable | Default | Description |
|---|---|---|
| `VOILOT_PORT` | `3000` | Port to expose the frontend |
| `KOKORO_DEFAULT_VOICE` | `af_heart` | Default Kokoro TTS voice |
| `WHISPER_MODEL` | `base` | Whisper model size (`tiny`, `base`, `small`, `medium`, `large-v3`) |
| `WHISPER_DEVICE` | `cpu` | Whisper device (`cpu` or `cuda`) |

## Development

### Prerequisites

- Go 1.22+
- Node 20+
- Docker (for voice services)
- [Task](https://taskfile.dev) (`brew install go-task`)
- [OpenCode](https://opencode.ai) running via `opencode serve` in your target project

### First-time setup

```bash
task install        # Go modules + npm packages + config file + data dir
# Edit ~/.config/voilot/config.json — at minimum set your providers
task dev            # Start everything
```

### What `task dev` runs

| Task | What runs | Where | Port |
|------|-----------|-------|------|
| `dev:frontend` | Nuxt 3 dev server (HMR) | Native (Node) | 3000 |
| `dev:backend` | Go server via `go run` | Native (Go) | 8080 |
| `dev:voice` | Kokoro TTS + faster-whisper STT | Docker (detached) | 8880, 5003 |
| `dev:tunnel` | Tailscale HTTPS proxy | Native (auto-skips if unavailable) | 443 |

The browser connects directly to the backend at `http://localhost:8080` (configured via `backendUrl` in `nuxt.config.ts`) — there is no proxy through the Nuxt dev server.

Voice services are optional. If you only need text chat, run `task dev:frontend` and `task dev:backend` individually.

If Tailscale is running, `task dev` automatically proxies HTTPS for mobile mic access. The tunnel is only needed for testing voice on a mobile device — desktop-only text dev works fine without it.

### Taskfile variables

Override with `VAR=value task dev:backend`:

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKSPACE_DIR` | `~/tmp/voilot-wd` | Directory containing your project repos/worktrees that voilot manages agent instances for. Not the voilot repo itself. |
| `DATA_DIR` | `~/tmp/voilot-wd/voilot-data` | Runtime state: session mappings, worktree provider defaults, PID tracking. Persists across restarts. |
| `TTS_URL` | `http://localhost:8880` | TTS service URL |
| `STT_URL` | `http://localhost:5003` | STT service URL |

CLI flags (`--tts-url`, `--stt-url`, `--workspace-dir`) override config file values. The config file is still required for provider definitions.

### Other useful tasks

- `task dev:stop` — stop voice containers
- `task build` — compile backend + generate frontend static build
- `task check` — go vet + nuxt typecheck
- `task test` — run Go tests

### Manual (without Task)

```bash
# Backend
cd backend
go run ./cmd/server --workspace-dir ~/tmp/voilot-wd --data-dir ~/tmp/voilot-wd/voilot-data

# Frontend
cd frontend
npm run dev
```

## Project Structure

```
voilot/
├── backend/
│   ├── cmd/server/main.go       # Entrypoint
│   └── internal/
│       ├── agent/               # Agent adapter interface + OpenCode impl
│       ├── api/                 # HTTP routes, handlers, WebSocket
│       ├── tts/                 # TTS provider interface + Kokoro impl
│       ├── stt/                 # STT provider interface + Whisper impl
│       └── voice/               # Command router + TTS filter
├── frontend/
│   ├── pages/                   # index (sessions), session/[id] (chat)
│   ├── components/              # UI components
│   ├── composables/             # useSession, useAgent, useVoice, useTTS
│   └── nuxt.config.ts
└── docker/
    ├── docker-compose.yml
    ├── nginx.conf
    ├── Dockerfile.backend
    ├── Dockerfile.frontend
    ├── Dockerfile.stt
    └── stt-server.py
```

## API Endpoints

### REST

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/status` | Agent connection status |
| `GET` | `/api/sessions` | List sessions |
| `POST` | `/api/sessions` | Create session |
| `DELETE` | `/api/sessions/:id` | Delete session |
| `POST` | `/api/sessions/:id/message` | Send message (REST fallback) |
| `POST` | `/api/tts/synthesize` | Text-to-speech |
| `GET` | `/api/tts/voices` | List TTS voices |
| `POST` | `/api/stt/transcribe` | Speech-to-text |

### WebSocket

| Path | Description |
|---|---|
| `/ws/chat` | Bidirectional chat with agent (text) |
| `/ws/voice` | Bidirectional voice pipeline (audio in/out) |

## Roadmap

- [x] Phase 0a: Go backend skeleton (interfaces, stubs, compiles)
- [x] Phase 0b: Nuxt frontend skeleton (pages, components, composables)
- [x] Phase 0c: Docker Compose stack
- [x] Phase 0d: README
- [x] Phase 1: Go backend core (HTTP serving, WebSocket logic)
- [x] Phase 2: OpenCode adapter (real HTTP+SSE to `opencode serve`)
- [x] Phase 3: Text-only MVP (end-to-end chat working)
- [x] Phase 4: STT service integration
- [x] Phase 5: TTS service integration
- [x] Phase 6: Voice pipeline (mic → STT → agent → TTS → speaker)
- [x] Phase 7: Plan/Implement mode switching
- [x] Phase 8: Production hardening (structured logging, health checks, security headers, non-root containers)
- [ ] Phase 9: Browser end-to-end testing
- [ ] Phase 10: CI/CD pipeline

## Production Deployment

For production/homelab deployment, see **[docs/deployment.md](docs/deployment.md)** covering:

- TLS termination options (Caddy, Traefik, nginx+certbot, Tailscale)
- Docker Compose production overrides with log rotation and resource limits
- Firewall and security considerations
- Persistent storage and updates

For common issues, see **[docs/troubleshooting.md](docs/troubleshooting.md)**.
