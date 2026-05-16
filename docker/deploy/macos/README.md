# macOS Production Setup

One-time setup for running voilot on a Mac as a production server.

## Prerequisites

- **Homebrew** — [brew.sh](https://brew.sh)
- **Docker Desktop** — [docker.com](https://www.docker.com/products/docker-desktop/)
- **Git authentication** — SSH key or credential helper configured for the voilot repo
- **Tailscale** — installed and configured with `tailscale serve`
- **Ethernet** — USB-C adapter connected to Fritz!Box (required for Wake on LAN on Apple Silicon)
- **Fritz!Box** — Mac's Ethernet MAC address registered for WoL

## Usage

```bash
chmod +x docker/deploy/macos/setup.sh
./docker/deploy/macos/setup.sh
```

The script will:

1. Install `sleepwatcher` via Homebrew (if not already installed)
2. Configure energy settings (`pmset`) for server use
3. Add Docker Desktop to login items (auto-start on boot)
4. Create `voilot-data` Docker volume for persistent runtime data
5. Install a cron job that polls for updates every minute
6. Create a `~/.wakeup` hook for immediate update checks on wake

## What It Configures

| Setting | Value | Why |
|---------|-------|-----|
| Wake on LAN (`womp`) | enabled | Allow remote wake via Fritz!Box |
| Auto-restart after power failure | enabled | Recover from outages |
| Auto-sleep | disabled | Managed by update script's idle detection |
| Display sleep | 5 minutes | Save energy, screen not needed |
| Power Nap | disabled | No background activity during sleep |

## Logs

Deployment logs are written to `tmp/deploy.log` in the repository root.

## Wake on LAN Flow

1. Notice voilot is offline (Tailscale shows Mac disconnected)
2. Open Fritz!Box app, send WoL packet to the Mac
3. Mac wakes, Tailscale reconnects, sleepwatcher triggers update
4. Services come online via Docker Compose
