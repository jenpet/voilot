# Plan: Continuous deployment and energy management

**Status:** draft
**Created:** 2026-05-15
**Author:** jenpet + planitect

## Goal

Keep the production voilot instance on a MacBook Air automatically up to date with `main`, while managing energy consumption through intelligent sleep/wake behavior. The solution must be portable at its core, with platform-specific extensions for macOS (and Linux in the future).

## Context

The MacBook Air (2025, Apple Silicon) is the production machine, running the full Docker Compose stack (frontend, backend, TTS, STT). It connects via Ethernet (USB-C adapter) to a Fritz!Box router. Tailscale provides HTTPS access from outside the LAN.

When the Mac is not in use, it should sleep to save energy. The user wakes it manually via Wake on LAN from the Fritz!Box app when needed. On wake, it should immediately check for updates and deploy if `main` has moved forward.

Apple Silicon Macs do not support WoL over Wi-Fi — Ethernet is required.

## Approach

### 1. Portable update script (`docker/update.sh`)

A bash script that:

- Runs `git fetch origin main` and compares local HEAD against `origin/main`
- If SHAs differ: `git pull --ff-only origin main` then `docker compose -f docker/docker-compose.yml -f docker/docker-compose.production.yml --profile voice up --build -d`
- If SHAs match: do nothing
- Logs all actions with timestamps to stdout (captured by cron/journald)
- On build failure: logs the error, does **not** auto-rollback. The status page will show the version mismatch
- Configurable poll interval (default: 60 seconds) via environment variable `VOILOT_UPDATE_INTERVAL`
- Voice profile (`--profile voice`) included by default, configurable via `VOILOT_COMPOSE_PROFILES`
- **Docker readiness gate:** Before any Docker operations, waits up to 60 seconds for the Docker daemon (polls `docker info` every 2 seconds). Exits with an error if the daemon isn't ready — prevents failures after wake or reboot.
- Exits cleanly on SIGTERM for cron/service manager compatibility

All services in `docker-compose.production.yml` use `restart: unless-stopped`, so containers recover automatically if Docker Desktop restarts or an individual container crashes. The update script only needs to run `docker compose up` when the git SHA changes.

The script uses only `git`, `docker`, and standard POSIX utilities — fully portable across macOS and Linux.

### 2. Build hash injection

**Backend:** Inject the short git SHA and build timestamp via `go build -ldflags`:

```
-X main.version=$(git rev-parse --short HEAD)
-X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
```

Expose these fields on the existing `/api/health` and `/api/health/detailed` endpoints.

**Frontend:** Inject via build-time environment variables (`NUXT_PUBLIC_BUILD_HASH`, `NUXT_PUBLIC_BUILD_TIME`). The Dockerfiles pass these during build.

**Display:** A dedicated status page (separate plan) will show these values along with service health. The status page plan will be written separately.

### 3. Idle detection and sleep trigger

**Backend change:** Track a `lastActivityAt` timestamp in the Server struct, updated on every API request and WebSocket message. Include it in the `/api/health/detailed` response.

**Update script responsibility:** After each poll cycle, the script also checks `lastActivityAt` from `/api/health/detailed`. If the last activity is older than the idle timeout (default: 30 minutes, configurable via `VOILOT_IDLE_TIMEOUT`), the script triggers platform-specific sleep:

- macOS: `pmset sleepnow`
- Linux: `systemctl suspend` (future)

This keeps the backend unaware of power management — clean separation of concerns.

### 4. macOS platform setup (`docker/deploy/macos/`)

#### 4.1 Setup script (`docker/deploy/macos/setup.sh`)

A one-time setup script that applies:

**Energy settings via `pmset`:**

```bash
# Wake for network access (required for WoL)
sudo pmset -a womp 1

# Start up automatically after power failure
sudo pmset -a autorestart 1

# Disable auto-sleep (managed by update script's idle detection)
sudo pmset -a sleep 0

# Display sleep after 5 minutes (saves energy, screen doesn't matter for a server)
sudo pmset -a displaysleep 5

# Disable Power Nap (no background activity during sleep)
sudo pmset -a powernap 0
```

**Cron job installation:**

Adds a cron entry that runs the update script every minute:

```
* * * * * /path/to/voilot/docker/update.sh >> /path/to/voilot/deploy.log 2>&1
```

**Wake hook installation via sleepwatcher:**

Installs [sleepwatcher](https://www.bernhard-baehr.de/) (`brew install sleepwatcher`) — a lightweight daemon that executes scripts on macOS sleep/wake events. Homebrew automatically sets up its launchd plist. The setup script creates `~/.wakeup` pointing to the update script, which sleepwatcher calls immediately on wake. This is more reliable than a custom launchd plist, as launchd has no native wake event trigger.

**Docker Desktop auto-start on login:**

```bash
osascript -e 'tell application "System Events" to make login item at end with properties {path:"/Applications/Docker.app", hidden:true}'
```

Ensures Docker Desktop launches automatically after reboot. Idempotent — adding a duplicate login item is a no-op.

### 5. Directory structure

```
docker/
├── update.sh                        # Portable update script
├── deploy/
│   ├── macos/
│   │   ├── setup.sh                 # One-time macOS setup (pmset, cron, wake hook)
│   │   └── README.md                # Setup prerequisites (sleepwatcher via brew, etc.)
│   └── linux/                       # Future: systemd units, suspend hooks
```

### 6. Wake on LAN flow

1. User notices voilot is offline (Tailscale shows Mac as disconnected)
2. User opens Fritz!Box app, sends WoL packet to the Mac's Ethernet MAC address
3. Mac wakes, Tailscale reconnects, sleepwatcher fires update script via `~/.wakeup`
4. Update script checks for changes, deploys if needed
5. Services come online, accessible via Tailscale HTTPS

## Prerequisites

- Sleepwatcher installed via Homebrew (`brew install sleepwatcher`)
- Git authentication configured on the production Mac (SSH key or credential helper)
- Docker Desktop installed (auto-start configured by `setup.sh`)
- Tailscale installed and configured with `tailscale serve`
- USB-C Ethernet adapter connected to Fritz!Box
- Fritz!Box configured with the Mac's Ethernet MAC address for WoL

## Open Questions

- Status page design and content (separate plan)

## Acceptance Criteria

- Pushing to `main` results in automatic deployment on the Mac within 60 seconds (while awake)
- Build hash is visible on the backend health endpoint and in the frontend
- Mac sleeps after 30 minutes of no voilot activity
- WoL from Fritz!Box wakes the Mac and triggers an immediate update check
- Setup is reproducible by running `docker/deploy/macos/setup.sh` on a fresh Mac
- Update script runs without modification on both macOS and Linux (sleep/wake hooks are platform-specific)
