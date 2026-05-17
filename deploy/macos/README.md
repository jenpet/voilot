# macOS Production Setup

One-time setup for running voilot on a Mac as a production server.

## Prerequisites

- **Go** — [go.dev](https://go.dev/dl/) (latest stable)
- **Node.js** — [nodejs.org](https://nodejs.org/) (LTS)
- **Homebrew** — [brew.sh](https://brew.sh)
- **Docker Desktop** — [docker.com](https://www.docker.com/products/docker-desktop/) (for TTS/STT voice containers)
- **Tailscale** — `brew install --cask tailscale`, signed in and connected
- **Git authentication** — SSH key or credential helper configured for the voilot repo
- **Ethernet** — USB-C adapter connected to Fritz!Box (required for Wake on LAN on Apple Silicon)
- **Fritz!Box** — Mac's Ethernet MAC address registered for WoL

## Architecture

The backend (Go) and frontend (Nuxt) run natively on the host, managed by macOS launchd. Only TTS and STT run in Docker containers. See `docs/adr/0001-host-native-backend-frontend-docker-for-voice.md` for rationale.

```
Tailscale (HTTPS)
  :443  → Nuxt server (localhost:3000)
  :8080 → Go backend  (localhost:8080)

Docker: TTS (:8880), STT (:5003)
```

## Usage

```bash
chmod +x deploy/macos/setup.sh
./deploy/macos/setup.sh
```

The script will:

1. Verify prerequisites (Go, Node.js, Docker, Tailscale, sleepwatcher)
2. Configure energy settings (`pmset`) for server use
3. Set up Tailscale HTTPS serve rules (`:443` → frontend, `:8080` → backend)
4. Install launchd plists for backend and frontend (`~/Library/LaunchAgents/`)
5. Add Docker Desktop to login items (auto-start on boot)
6. Install a cron job that polls for updates every minute
7. Create a `~/.wakeup` hook for immediate update checks on wake
8. Run the initial deploy (build + start everything)

## What It Configures

| Setting | Value | Why |
|---------|-------|-----|
| Wake on LAN (`womp`) | enabled | Allow remote wake via Fritz!Box |
| Auto-restart after power failure | enabled | Recover from outages |
| Auto-sleep | disabled | Managed by update script's idle detection |
| Display sleep | 5 minutes | Save energy, screen not needed |
| Power Nap | disabled | No background activity during sleep |

## Services

| Service | Manager | Plist / Container |
|---------|---------|-------------------|
| Backend | launchd | `pet.jen.voilot.backend` |
| Frontend | launchd | `pet.jen.voilot.frontend` |
| TTS | Docker | `voilot-tts-1` |
| STT | Docker | `voilot-stt-1` |

## Logs

| Log | Path |
|-----|------|
| Deploy / update | `tmp/deploy.log` |
| Backend | `tmp/backend.log` |
| Frontend | `tmp/frontend.log` |

## Wake on LAN Flow

1. Notice voilot is offline (Tailscale shows Mac disconnected)
2. Open Fritz!Box app, send WoL packet to the Mac
3. Mac wakes, Tailscale reconnects, sleepwatcher triggers update
4. Update script pulls latest `main`, builds, restarts services
5. Voilot comes online via Tailscale HTTPS
