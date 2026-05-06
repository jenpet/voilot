# Deployment Guide

> **Security warning:** voilot has no built-in authentication. If you expose it to the internet without a VPN, reverse proxy auth, or other access control, anyone can use your coding agent. See [Security](#security) below.

## Prerequisites

- Docker and Docker Compose v2
- A domain name (optional but recommended for TLS)
- An OpenCode instance running on the host (or accessible via network)

## Quick Start

```bash
git clone https://github.com/jenpet/voilot.git
cd voilot/docker
cp .env.example .env        # Edit as needed
cp config.example.json config.docker.json  # Edit provider settings

# Frontend + backend only
docker compose up -d

# Full stack with voice (TTS + STT)
docker compose --profile voice up -d
```

## Configuration

voilot uses a JSON config file (`docker/config.docker.json`) as the single source of truth. No environment variables are needed for core settings — the `.env` file only controls Docker Compose variables like ports and model sizes.

Edit `docker/config.docker.json` to configure:
- Provider settings (OpenCode binary path, type)
- TTS/STT URLs (use Docker service names: `http://tts:8880`, `http://stt:5003`)
- Workspace directory
- Max instances and idle timeout

## TLS Termination

voilot serves HTTP only. **You must provide your own TLS layer.** This is not optional if you want voice features — `getUserMedia` (microphone access) requires a secure context (HTTPS or localhost).

Common options:

- **Caddy** — automatic HTTPS with Let's Encrypt
- **Traefik** — Docker-native reverse proxy with auto TLS
- **nginx + certbot** — manual but well-documented
- **Tailscale** — zero-config for personal/team use (`tailscale serve --bg --https=443 http://localhost:3000`)

voilot does not recommend one over another. Use whatever fits your infrastructure.

## Production Hardening

For production deployments, use the production override file:

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.production.yml up -d

# With voice services:
docker compose -f docker/docker-compose.yml -f docker/docker-compose.production.yml --profile voice up -d
```

This adds:
- Log rotation (10MB max, 3-5 files per service)
- Isolated Docker network
- Commented resource limits (uncomment and adjust as needed)

### Resource Requirements

| Service  | CPU   | Memory  | Notes                              |
|----------|-------|---------|------------------------------------|
| frontend | 0.1   | 32 MB   | Static nginx, minimal              |
| backend  | 0.2   | 64 MB   | Go binary, grows with sessions     |
| tts      | 1.0   | 512 MB  | Kokoro 82M param model on CPU      |
| stt      | 1.0   | 512 MB  | `base` model; `large-v3` needs 2GB |

## Firewall

Only expose the frontend port (default 3000) or your TLS proxy port (80/443). Keep internal service ports off the public network:
- `8080` — backend API (proxied through nginx)
- `8880` — TTS (internal to Docker network)
- `5003` — STT (internal to Docker network)

## Security

voilot has **no built-in authentication**. Access control is your responsibility:

- Use a VPN (Tailscale, WireGuard) to restrict access
- Put basic auth or OAuth on your reverse proxy
- Use firewall rules to allow only trusted IPs

CORS alone does not protect against direct API access — it only restricts browser-based cross-origin requests. An attacker with your URL can still call the API directly.

## Persistent Storage

- `stt-models` Docker volume — cached Whisper model weights
- `voilot-data/` — session map, PID files, worktree defaults (mounted via `--data-dir`)

Back up `voilot-data/` if you want to preserve session mappings across reinstalls.

## Updates

```bash
cd voilot
git pull
docker compose up --build -d
```

This rebuilds images with the latest code and restarts services. Persistent data is preserved.

## OpenCode on the Host

OpenCode runs on the host machine (not containerized). The backend reaches it via `host.docker.internal`. On Linux, this requires the `extra_hosts` mapping in `docker-compose.yml` (already configured).

If `host.docker.internal` doesn't resolve on your Linux distro, add it manually:

```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```
