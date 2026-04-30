# Plan: Production Hardening for Self-Hosted Distribution

**Status:** draft
**Created:** 2026-04-30
**Author:** jenpet + planitect

## Goal

Make voilot ready for self-hosted production use on a VPS or homelab server. Harden the backend, improve security posture, add operational basics (health checks, logging, graceful shutdown), and provide clear documentation so someone can go from `git clone` to a running, TLS-secured instance in under 30 minutes.

## Context

Voilot currently works well for local development but lacks several production essentials:

- **No graceful shutdown** — backend exits immediately, dropping active WebSocket connections
- **No structured logging** — plain `log.Printf` throughout, no levels or JSON output
- **No health checks in Docker Compose** — orchestrator can't detect unhealthy services
- **CORS wide open** (`*` by default) — fine for dev, not for production
- **No request size limits** — unbounded file uploads to STT, unbounded message bodies
- **No `.env.example`** — users must read README to discover configuration
- **No LICENSE** — blocks open-source distribution
- **Minimal test coverage** — one test file (`opencode_test.go`), no frontend tests
- **No deployment guide** — README covers local dev and Tailscale, but not VPS/homelab scenarios
- **No `.dockerignore`** — Docker build context likely includes plans, docs, .git, etc.
- **No CSP headers** — nginx serves security headers but no Content-Security-Policy

### Deployment target

A single-node VPS or homelab server (1-4 cores, 2-8 GB RAM) running Docker Compose behind a reverse proxy with real TLS. The user accesses voilot from their phone or desktop browser over HTTPS.

**What's containerized and what's not:** The voilot stack (frontend, backend, TTS, STT) runs in Docker Compose. OpenCode runs directly on the host — it needs full filesystem access to read/write code, run git, execute shell commands, and use language tooling. Containerizing it would require mounting the entire workspace plus all dev tools, adding complexity for no benefit. The backend reaches OpenCode via `host.docker.internal`.

## Approach

### Track 1: Backend Stability

#### 1.1 Graceful shutdown
- Trap `SIGINT`/`SIGTERM` in `main.go`
- Use `http.Server` with `Shutdown(ctx)` instead of bare `http.ListenAndServe`
- Set a shutdown timeout (e.g. 15s) to drain active WebSocket connections
- Log shutdown progress

#### 1.2 Structured logging
- Replace `log.Printf` with `log/slog` (stdlib, no dependency needed)
- Add log levels: info, warn, error, debug
- JSON output when running in Docker (detect via env var or flag `--log-format=json`)
- Add request logging middleware (method, path, status, duration)
- Add a `--log-level` flag (default: `info`)

#### 1.3 Health checks
- Backend `/api/health` should only confirm the backend process is alive and accepting requests — NOT check downstream services (OpenCode, TTS, STT). Downstream status belongs on `/api/status` where the frontend reads it.
- Add Docker Compose `healthcheck` for backend, stt, and frontend services
- Backend healthcheck: `curl -f http://localhost:8080/api/health`
- STT healthcheck: `curl -f http://localhost:5003/health`
- Frontend healthcheck: `curl -f http://localhost:80/`
- Add `depends_on: condition: service_healthy` where appropriate

#### 1.4 Reconnection resilience
- Backend should handle OpenCode disconnections gracefully (SSE stream drops, connection refused)
- Log reconnection attempts with backoff info
- Surface connection state clearly on `/api/status` (connected, reconnecting, disconnected, error)
- Frontend WebSocket composable should auto-reconnect with exponential backoff (verify current behavior)

### Track 2: Security

#### 2.1 CORS lockdown
- Change default `--cors-origins` from `*` to empty (deny all cross-origin)
- Document how to set allowed origins for production (e.g. `--cors-origins=https://voilot.example.com`)
- In Docker Compose, wire `CORS_ORIGINS` env var to backend command

#### 2.2 Request limits
- Add max request body size in nginx (`client_max_body_size 10m`)
- Add Go-side request body limits for REST endpoints (e.g. 10MB for STT, 1MB for messages)
- Set WebSocket message size limit (already default in gorilla/nhooyr, but verify and document)

#### 2.3 Nginx security headers
- Add `Content-Security-Policy` header (default-src 'self', with allowances for WebSocket and audio blob URLs)
- Add `Permissions-Policy` header (allow microphone for self, deny everything else). **Implementation gate:** test mic access explicitly in installed PWA mode on iOS Safari and Android Chrome before merging — standalone PWA windows may handle this header differently than regular browser tabs.
- Review existing headers (`X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy` — already present)

#### 2.4 Secrets and configuration
- Create `.env.example` with all supported variables, sensible defaults, and comments
- Add `.env` to `.gitignore` (verify)
- Document which variables are required vs optional
- Avoid logging sensitive values (API keys, URLs with credentials)

#### 2.5 Non-root containers
- Backend Dockerfile: run as non-root user (straightforward — binary only listens on 8080, no file writes)
- Frontend Dockerfile: nginx already runs workers as non-root, but verify
- STT Dockerfile: create a dedicated `stt` user, set `HF_HOME=/home/stt/.cache/huggingface`, and update the Docker Compose volume mount from `/root/.cache/huggingface` to match. Otherwise model downloads fail under non-root.

### Track 3: Deployment & Operations

#### 3.1 VPS / homelab deployment guide
Create `docs/deployment.md` covering:

- **Prerequisites**: Docker, Docker Compose v2, a domain name (optional but recommended)
- **TLS termination** — voilot serves HTTP only; document that the user must provide their own TLS layer. Give brief examples for common options (Caddy, Traefik, nginx+certbot, Tailscale) without recommending one over another
- **HTTPS requirement** — explain that mic access (`getUserMedia`) requires a secure context, so TLS is not optional for voice features
- **Firewall basics** — only expose 80/443 (or whatever the reverse proxy uses), keep 8080/8880/5003 internal to the Docker network
- **External access** — note that access control (VPN, Tailscale, basic auth, etc.) is the user's responsibility; voilot has no built-in auth
- **Persistent storage**: where data lives, how to back it up (`stt-models` volume, `voilot-data/`)
- **Updates**: `git pull && docker compose up --build -d`

#### 3.2 Docker Compose production hardening
- Add `docker-compose.production.yml` override file with:
  - Commented-out resource limits as starting points (with a resource requirements table in docs explaining typical usage per service and model size)
  - Restart policies (already `unless-stopped` — good)
  - Log driver config (`json-file` with max-size/max-file rotation)
  - Health checks (from 1.3)
  - Non-default network (isolate voilot services)
- Add `.dockerignore` to reduce build context (exclude `.git`, `plans/`, `docs/`, `tests/`, `agents/`, `*.md`)

#### 3.3 Startup diagnostics
- Backend should log a clear summary on startup: what's connected, what's missing, what's optional
- Health check endpoint surfaces downstream status so `docker compose ps` gives actionable info

### Track 4: Documentation

#### 4.1 README improvements
- Add a "Production Deployment" section linking to `docs/deployment.md`
- Add badges (license, Docker, build status if CI exists)
- Add a "Troubleshooting" section or link to `docs/troubleshooting.md`
- Clean up roadmap (remove completed phases, add production/hardening items)

#### 4.2 Troubleshooting guide
Create `docs/troubleshooting.md` covering common issues:
- "Mic doesn't work" — HTTPS required, check Tailscale/TLS setup
- "Can't connect to OpenCode" — check `OPENCODE_URL`, `host.docker.internal` on Linux
- "TTS/STT not working" — check `--profile voice`, verify URLs
- "WebSocket disconnects" — check nginx timeouts, proxy config
- "Container keeps restarting" — check logs with `docker compose logs <service>`
- `host.docker.internal` not resolving on Linux — add `extra_hosts` mapping

#### 4.3 LICENSE
- Add MIT LICENSE file

#### 4.4 Security warning
- Add a prominent warning in both README and deployment docs: "voilot has no built-in authentication. If you expose it to the internet without a VPN, reverse proxy auth, or other access control, anyone can use your coding agent."

#### 4.4 .env.example
```env
# voilot configuration
# Copy to .env and adjust as needed

# ── Core ─────────────────────────────────────────────
# Port to expose the frontend (default: 3000)
VOILOT_PORT=3000

# OpenCode server URL (required)
# Use host.docker.internal to reach host services from Docker
OPENCODE_URL=http://host.docker.internal:4096

# Allowed CORS origins (comma-separated, empty = deny cross-origin)
# Set to your domain in production, e.g. https://voilot.example.com
# CORS_ORIGINS=

# ── Voice services (optional) ───────────────────────
# Enable with: docker compose --profile voice up
#
# Docker Compose: use container names (http://tts:8880, http://stt:5003)
# Local dev:      use localhost (http://localhost:8880, http://localhost:5003)
# TTS_URL=http://tts:8880
# STT_URL=http://stt:5003

# Kokoro TTS default voice
KOKORO_DEFAULT_VOICE=af_heart

# Whisper model size: tiny, base, small, medium, large-v3
WHISPER_MODEL=base
# Whisper device: cpu or cuda
WHISPER_DEVICE=cpu

# ── Advanced ─────────────────────────────────────────
# Backend log level: debug, info, warn, error
# LOG_LEVEL=info
# Backend log format: text, json
# LOG_FORMAT=text
```

### Track 5: Testing (foundation only)

Backend unit tests only — frontend E2E tests are impractical to maintain given the full-stack dependency.

#### 5.1 Backend
- Add health check handler tests (mock downstream services)
- Add voice router unit tests (command keyword matching)
- Add TTS filter unit tests (code block summarization, thinking block skipping)
- Ensure `go test ./...` passes cleanly in CI/Docker build

#### 5.2 Docker build validation
- Add a `task docker:test` that builds all images and verifies health checks pass
- Validates the full stack can start and respond to requests

## Open Questions

1. ~~**License choice**~~ — MIT. Compatible with all dependencies (Kokoro is Apache-2.0, faster-whisper is MIT).
2. ~~**Auth**~~ — Intentionally out of scope. Access control (VPN, Tailscale, basic auth, firewall rules) is the user's responsibility. Document this clearly but don't build it in.

### Future follow-ups (out of scope)

- **Built-in auth** — simple shared-secret or basic auth as a safety net for users who expose voilot without a VPN. Important: CORS alone does not protect against direct API access.
- **Session tracing & correlation** — request/session ID propagation in logs, correlating voilot backend sessions with OpenCode session IDs across the full lifecycle.
- **CI/CD** — GitHub Actions for build/test/lint.
- **Monitoring** — Prometheus metrics endpoint for homelab users running Grafana.

## Acceptance Criteria

- [ ] Backend shuts down gracefully on SIGTERM (drains connections within 15s)
- [ ] All services have Docker Compose health checks; stack reports healthy after startup
- [ ] Structured logging with configurable level and format
- [ ] `.env.example` exists with all variables documented
- [ ] `.dockerignore` reduces build context to essential files only
- [ ] Nginx has CSP and Permissions-Policy headers
- [ ] Request body size limits enforced at nginx and Go level
- [ ] Backend containers run as non-root
- [ ] `docs/deployment.md` covers TLS options, firewall, persistent storage, and updates
- [ ] `docs/deployment.md` includes resource requirements table per service/model size
- [ ] `docs/troubleshooting.md` covers top 6 common issues
- [ ] Prominent no-auth security warning in README and deployment docs
- [ ] LICENSE file present (MIT)
- [ ] `docker-compose.production.yml` adds log rotation, health checks, commented resource limits
- [ ] `go test ./...` has at least health, voice router, and TTS filter coverage
