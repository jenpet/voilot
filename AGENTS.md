# AGENTS.md - Coding Agent Guidelines for Voilot

## Project Overview

Voilot (voice + pilot) is a voice-first, mobile-first PWA client for interacting with AI coding agents. The core workflow: **plan via voice conversations** (from mobile/desktop), then **implement later** at the computer. Currently targets OpenCode as the agent backend, with Claude Code support planned.

### Architecture

```
Browser (PWA) — Nuxt 3 + Vue + Tailwind, dark theme, mobile-first
       | HTTP / WebSocket
Nginx — static files, /api/* and /ws/* reverse-proxied to backend
       |
Go Backend (port 8080) — REST API, WebSocket, agent adapter, voice router
       |
  +----+----------------+
  |    |                 |
Kokoro TTS (:8880)   faster-whisper STT (:5003)   OpenCode (:4096)
```

### Key Concepts

- **Plan vs Implement mode** — "plan" restricts the agent to discussion only (via system prompt prepended to messages); "implement" gives full tool access. This is a voilot-level concept, not native to OpenCode. Session modes are stored in-memory in the Go backend (ephemeral, reset on restart).
- **Voice command router** — keyword detection separates app commands (mode switch, new session, stop) from agent messages (forwarded as natural language).
- **Smart TTS filter** — speaks text, summarizes code blocks ("Wrote 45 lines of TypeScript"), skips thinking blocks, announces errors.
- **Pluggable agent adapters** — Go interface (`agent.Adapter`) that backends implement. OpenCode is the first; Claude Agent SDK is planned.

### Design Constraints

- **No React** — Vue.js / Nuxt 3 for the frontend
- **Go backend** — single binary, REST + WebSocket API only, does NOT serve the frontend
- **Frontend served by Nginx** — static build via `nuxt generate`
- **Open-source models only** for TTS and STT, self-hosted via Docker
- **Docker Compose** stack for all services

## Repository Structure

```
voilot/
├── backend/
│   ├── cmd/server/main.go       # Entrypoint, CLI flags (--tts-provider, --tts-url, etc.)
│   ├── go.mod                   # Module: github.com/jenpet/voilot
│   └── internal/
│       ├── agent/               # Adapter interface + OpenCode implementation
│       │   ├── adapter.go       # Adapter interface, Session, SessionMode, PlanModeSystemPrompt
│       │   ├── events.go        # Event types, OpenCode SSE wire-format structs
│       │   └── opencode.go      # Full OpenCode HTTP client + SSE reader (~800 lines)
│       ├── api/                 # HTTP routes, handlers, WebSocket
│       │   ├── router.go        # Route registration
│       │   ├── handlers.go      # REST handlers (sessions, STT, TTS, abort, mode)
│       │   └── ws.go            # WebSocket chat handler + voice stub (/ws/voice is still a stub)
│       ├── tts/                 # TTS provider interface + implementations
│       │   ├── provider.go      # Provider interface (Synthesize, ListVoices, Name)
│       │   ├── kokoro.go        # Kokoro-FastAPI client (OpenAI-compatible API) — DEFAULT
│       │   └── coqui.go         # Coqui XTTSv2 client (legacy fallback)
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
├── docker/
│   ├── docker-compose.yml       # Services: frontend, backend, tts (Kokoro), stt (whisper)
│   ├── nginx.conf               # Production reverse proxy with Docker DNS resolver
│   ├── Dockerfile.backend       # Multi-stage Go build
│   ├── Dockerfile.frontend      # Multi-stage Nuxt generate + nginx (NUXT_PUBLIC_BACKEND_URL="")
│   ├── Dockerfile.stt           # Python 3.11 + faster-whisper + gunicorn
│   ├── Dockerfile.tts           # Legacy Coqui XTTSv2 (unused, consider removing)
│   ├── stt-server.py            # Flask app wrapping faster-whisper (/transcribe, /health)
│   └── tts-server.py            # Legacy Flask app for Coqui (unused, consider removing)
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

# Run locally:
./server --opencode-url http://localhost:4096 \
         --tts-url http://localhost:8880 \
         --tts-provider kokoro \
         --stt-url http://localhost:5003
```

### Nuxt Frontend

```bash
cd frontend
npm install              # Install dependencies
npm run dev              # Dev server at http://localhost:3000
npx nuxt generate        # Static build -> .output/public/
```

### Docker

```bash
cd docker
docker compose up --build                                   # Frontend + backend only
TTS_URL=http://tts:8880 STT_URL=http://stt:5003 \
  docker compose --profile voice up --build                 # Full stack with voice
```

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
- Types: feat, fix, docs, style, refactor, test, chore

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

- All composables use `config.public.backendUrl` (from `nuxt.config.ts` runtimeConfig)
- Dev: defaults to `http://localhost:8080` (direct to Go backend)
- Docker: set to `""` via `NUXT_PUBLIC_BACKEND_URL=""` in Dockerfile.frontend so the static JS bundle uses relative URLs behind nginx
- WebSocket composable: absolute URL in dev, `window.location`-derived in production
- No proxying in Nuxt config — Nitro devProxy and Vite server.proxy both crash with ECONNRESET when backend is unavailable

## Docker Deployment Notes

- `NUXT_PUBLIC_BACKEND_URL` must be `""` during Dockerfile.frontend build
- TTS/STT URLs are NOT auto-wired with `--profile voice` — user must set `TTS_URL` and `STT_URL` env vars
- `host.docker.internal:host-gateway` extra_hosts needed for backend to reach host's OpenCode server
- Nginx DNS caching: uses `resolver 127.0.0.11 valid=10s` and a variable for the upstream (`set $backend_upstream http://backend:8080`) so nginx re-resolves on container recreation

## Known Issues / Remaining Work

- `/ws/voice` WebSocket endpoint is still a stub (may not be needed — current architecture uses REST for STT/TTS)
- Session modes are in-memory only, reset on backend restart
- `docker/Dockerfile.tts` and `docker/tts-server.py` are dead code from the Coqui era — can be removed
- Browser end-to-end testing not yet done (mic -> STT -> OpenCode -> TTS playback in a real browser)
- WebSocket reconnect uses exponential backoff (1s base, 30s max, 1.5x multiplier) with HMR-safe global state on `window.__voilot_ws`

---

*This file should be updated as the project evolves.*
