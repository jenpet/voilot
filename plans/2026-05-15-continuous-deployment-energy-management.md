# Plan: Continuous deployment and energy management

**Status:** draft
**Created:** 2026-05-15
**Author:** jenpet + planitect

## Goal

Keep the production voilot instance on a MacBook Air automatically up to date with `main`, while managing energy consumption through intelligent sleep/wake behavior. The solution must be portable at its core, with platform-specific extensions for macOS (and Linux in the future).

## Context

The MacBook Air (2025, Apple Silicon) is the production machine. The Go backend and Nuxt frontend run natively on the host, managed by macOS launchd. TTS and STT run in Docker containers. Tailscale provides HTTPS access from outside the LAN, terminating TLS and proxying to both the frontend (port 3000) and backend (port 8080).

The backend spawns OpenCode as a child process and shells out to `git` and `wt` — this requires running on the host, not inside Docker. See ADR 0001.

When the Mac is not in use, it should sleep to save energy. The user wakes it manually via Wake on LAN from the Fritz!Box app. On wake, it should immediately check for updates and deploy if `main` has moved forward.

Apple Silicon Macs do not support WoL over Wi-Fi — Ethernet is required.

## Architecture

```
Tailscale (HTTPS termination)
  ├── :443  → Nuxt server (host, port 3000)
  └── :8080 → Go backend  (host, port 8080)

Go backend spawns:
  └── OpenCode instances (child processes per worktree)

Docker (voice services only):
  ├── TTS — Kokoro-FastAPI (:8880)
  └── STT — faster-whisper (:5003)
```

No nginx. The frontend makes direct cross-origin requests to the backend on a separate port, same as the dev setup. CORS on the backend allows this.

## Approach

### 1. Directory structure

```
deploy/
├── deploy.sh                           # Portable: git pull, build, restart services, docker compose voice
├── update.sh                           # Portable: git fetch/compare, call deploy.sh, idle sleep
├── docker/
│   ├── docker-compose.yml              # TTS + STT only, project name via VOILOT_COMPOSE_PROJECT
│   ├── Dockerfile.stt
│   └── stt-server.py
└── macos/
    ├── setup.sh                        # One-time: pmset, launchd plists, tailscale serve, sleepwatcher, cron
    ├── README.md                       # Prerequisites, setup instructions
    ├── run-backend.sh                  # Wrapper: sources .env, execs voilot-server with flags
    ├── run-frontend.sh                 # Wrapper: sources .env, execs node .output/server/index.mjs
    └── plists/
        ├── pet.jen.voilot.backend.plist
        └── pet.jen.voilot.frontend.plist
```

The old `docker/` root directory is removed entirely. `Dockerfile.backend`, `Dockerfile.frontend`, nginx configs, and `config.docker.json` are deleted.

### 2. Process management (macOS)

Two user-level launchd plists in `~/Library/LaunchAgents/`:

- **`pet.jen.voilot.backend`** — runs `deploy/macos/run-backend.sh`, which sources `.env` and execs `dist/voilot-server` with CLI flags (`--config`, `--tts-url`, `--stt-url`, `--workspace-dir`, `--data-dir`, etc.)
- **`pet.jen.voilot.frontend`** — runs `deploy/macos/run-frontend.sh`, which sources `.env` and execs `node frontend/.output/server/index.mjs`

Both plists use `KeepAlive: true` and `RunAtLoad: true`. Logs go to `tmp/backend.log` and `tmp/frontend.log`.

The `.env` file at the repo root is the single source of truth for configuration — same file used by the Taskfile in dev and by the wrapper scripts in production.

### 3. Deploy script (`deploy/deploy.sh`)

Repeatable, self-resolving (derives repo root from its own path). Steps:

1. Export `BUILD_HASH` (git short SHA) and `BUILD_TIME`
2. Build backend: `go build -ldflags "..." -o dist/voilot-server ./cmd/server`
3. Build frontend: `cd frontend && npm install && NUXT_PUBLIC_BUILD_HASH=... NUXT_PUBLIC_BUILD_TIME=... npx nuxt build`
4. Restart backend: `launchctl kickstart -k gui/$(id -u)/pet.jen.voilot.backend`
5. Restart frontend: `launchctl kickstart -k gui/$(id -u)/pet.jen.voilot.frontend`
6. Docker compose for voice (best-effort, non-blocking): `docker compose -p ${VOILOT_COMPOSE_PROJECT:-voilot} -f deploy/docker/docker-compose.yml up -d --build`

Backend and frontend always deploy, even if Docker isn't ready. Voice services are best-effort — a warning is logged if Docker is unavailable.

Brief downtime (1-2 seconds) during restart is acceptable. The frontend WebSocket reconnects automatically. OpenCode instances survive backend restarts (separate process groups, PIDs persisted to disk).

### 4. Update script (`deploy/update.sh`)

Portable bash script (same as before, updated paths). Steps:

- Wait for Docker daemon (best-effort, non-blocking for core services)
- `git fetch origin main`, compare local HEAD against `origin/main`
- If SHAs differ: `git pull --ff-only origin main`, then call `deploy/deploy.sh`
- If SHAs match: do nothing
- After each cycle, check `/api/health/detailed` for `lastActivityAt`
- If idle longer than `VOILOT_IDLE_TIMEOUT` (default 30 minutes): trigger platform sleep (`pmset sleepnow` on macOS, `systemctl suspend` on Linux)
- Supports `--loop` for continuous polling or single-shot for cron
- Exit cleanly on SIGTERM

### 5. Build hash injection

**Backend:** `go build -ldflags "-X main.version=... -X main.buildTime=..."` — exposed via `/api/health` and `/api/health/detailed`.

**Frontend:** `NUXT_PUBLIC_BUILD_HASH` and `NUXT_PUBLIC_BUILD_TIME` set at build time, baked into the Nuxt build.

### 6. Docker Compose (voice only)

`deploy/docker/docker-compose.yml` contains only TTS and STT services. Project name set via `-p ${VOILOT_COMPOSE_PROJECT:-voilot}`. The `VOILOT_COMPOSE_PROJECT` variable is defined in `.env` (or Taskfile vars) with fallback to `voilot`.

Services use `restart: unless-stopped`. The `stt-models` volume caches whisper models. The old `voilot-data` volume is removed (backend runs on host, uses host filesystem directly).

### 7. macOS platform setup (`deploy/macos/setup.sh`)

One-time setup script:

1. Verify prerequisites: Go, Node.js, npm, Docker Desktop, Tailscale, sleepwatcher, git
2. Apply energy settings via `pmset`:
   - `womp 1` — Wake on LAN
   - `autorestart 1` — auto-restart after power failure
   - `sleep 0` — disable auto-sleep (managed by update script)
   - `displaysleep 5` — display off after 5 minutes
   - `powernap 0` — no background activity during sleep
3. Configure Tailscale HTTPS:
   - `tailscale serve --bg --https=443 http://localhost:3000`
   - `tailscale serve --bg --https=8080 http://localhost:8080`
4. Copy launchd plists (with repo path substitution) to `~/Library/LaunchAgents/`
5. `launchctl load` both plists
6. Add Docker Desktop to login items
7. Install sleepwatcher `~/.wakeup` hook → calls `update.sh`
8. Install cron job: `update.sh` every minute
9. Run initial `deploy/deploy.sh`

### 8. Tailscale networking

Two HTTPS serve rules, configured once by `setup.sh`:

| External | Internal |
|----------|----------|
| `https://<fqdn>:443` | `http://127.0.0.1:3000` (frontend) |
| `https://<fqdn>:8080` | `http://127.0.0.1:8080` (backend) |

Both services bind to `127.0.0.1`. Tailscale proxies locally — no need to expose on all interfaces.

The frontend's `useBackendUrl()` composable rewrites `localhost:8080` to the browser's hostname when accessed from another device (same behavior as dev).

### 9. Wake on LAN flow

1. User notices voilot is offline (Tailscale shows Mac as disconnected)
2. User opens Fritz!Box app, sends WoL packet to the Mac's Ethernet MAC address
3. Mac wakes, Tailscale reconnects, sleepwatcher fires `update.sh` via `~/.wakeup`
4. `update.sh` checks for changes, calls `deploy.sh` if needed
5. Services come online, accessible via Tailscale HTTPS

### 10. Taskfile changes

- `task deploy` → calls `deploy/deploy.sh`
- `build:frontend` → `npx nuxt build` (not `nuxt generate`)
- `dev:voice` → uses `deploy/docker/docker-compose.yml` with `-p` flag
- `docker:*` tasks simplified to voice-only operations
- `VOILOT_COMPOSE_PROJECT` variable with fallback to `voilot`

## Prerequisites

- Go (latest stable) installed on the production Mac
- Node.js (LTS) and npm installed on the production Mac
- Docker Desktop installed (for TTS/STT containers)
- Tailscale installed and configured
- Sleepwatcher installed via Homebrew (`brew install sleepwatcher`)
- Git authentication configured (SSH key or credential helper)
- USB-C Ethernet adapter connected to Fritz!Box
- Fritz!Box configured with the Mac's Ethernet MAC address for WoL

## Deferred

- **SSR (`ssr: true`)** — enables proper CSP nonces instead of `unsafe-inline`. Deferred because the codebase was built as an SPA; enabling SSR requires auditing all composables for `window`/`document` usage. Will be a separate PR.
- **Status page** — separate plan

## Open Questions

- Status page design and content (separate plan)

## Acceptance Criteria

- Pushing to `main` results in automatic deployment on the Mac within 60 seconds (while awake)
- Build hash is visible on the backend health endpoint and in the frontend
- Mac sleeps after 30 minutes of no voilot activity
- WoL from Fritz!Box wakes the Mac and triggers an immediate update check
- Setup is reproducible by running `deploy/macos/setup.sh` on a fresh Mac
- Update script runs without modification on both macOS and Linux (sleep/wake hooks are platform-specific)
- Frontend and backend run as launchd services with automatic restart on crash
- TTS and STT run in Docker with `restart: unless-stopped`
- No nginx in the stack
