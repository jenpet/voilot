# voilot

Voice-first, mobile-first PWA client for AI coding agents. Plan via voice conversations from your phone, implement later at your computer.

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
│  │ Coqui TTS│  │ Whisper  │  │ Agent Backend      │  │
│  │ XTTSv2   │  │ STT      │  │ (OpenCode / Claude)│  │
│  │ :5002    │  │ :5003    │  │ :4096              │  │
│  └──────────┘  └──────────┘  └────────────────────┘  │
└───────────────────────────────────────────────────────┘
```

### Key Concepts

- **Plan vs Implement mode** — "plan" restricts the agent to discussion only; "implement" gives full tool access
- **Voice command router** — separates app commands (mode switch, new session, stop) from agent messages
- **Smart TTS filter** — speaks text, summarizes code blocks, skips thinking, announces errors
- **Pluggable agent adapters** — OpenCode first, Claude Agent SDK planned

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

## Configuration

Environment variables (set in `.env` or pass to `docker compose`):

| Variable | Default | Description |
|---|---|---|
| `VOILOT_PORT` | `3000` | Port to expose the frontend |
| `OPENCODE_URL` | `http://host.docker.internal:4096` | OpenCode server URL |
| `TTS_URL` | (empty) | Coqui TTS server URL (auto-set when using voice profile) |
| `STT_URL` | (empty) | faster-whisper server URL (auto-set when using voice profile) |
| `TTS_MODEL` | `tts_models/multilingual/multi-dataset/xtts_v2` | Coqui TTS model |
| `WHISPER_MODEL` | `base` | Whisper model size (`tiny`, `base`, `small`, `medium`, `large-v3`) |
| `WHISPER_DEVICE` | `cpu` | Whisper device (`cpu` or `cuda`) |

## Development

### Backend (Go)

```bash
cd voilot/backend
go build ./cmd/server
./server --opencode-url http://localhost:4096
```

### Frontend (Nuxt 3)

```bash
cd voilot/frontend
npm install
npm run dev
```

Dev server runs at `http://localhost:3000` with auto-proxy to the Go backend at `:8080`.

### Generate static build

```bash
cd voilot/frontend
npx nuxt generate
# Output: .output/public/
```

## Project Structure

```
voilot/
├── backend/
│   ├── cmd/server/main.go       # Entrypoint
│   └── internal/
│       ├── agent/               # Agent adapter interface + OpenCode impl
│       ├── api/                 # HTTP routes, handlers, WebSocket
│       ├── tts/                 # TTS provider interface + Coqui impl
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
    ├── Dockerfile.tts
    ├── Dockerfile.stt
    ├── tts-server.py
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
- [ ] Phase 1: Go backend core (HTTP serving, WebSocket logic)
- [ ] Phase 2: OpenCode adapter (real HTTP+SSE to `opencode serve`)
- [ ] Phase 3: Text-only MVP (end-to-end chat working)
- [ ] Phase 4: STT service integration
- [ ] Phase 5: TTS service integration
- [ ] Phase 6: Voice pipeline (mic → STT → agent → TTS → speaker)
- [ ] Phase 7: Plan/Implement mode switching
- [ ] Phase 8: Production deployment + Tailscale
