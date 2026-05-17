#!/usr/bin/env bash
#
# voilot macOS production setup
#
# One-time script that configures a Mac as a voilot production server:
#   - Verify prerequisites (Go, Node.js, Docker, Tailscale, GitHub CLI)
#   - Energy settings (WoL, auto-restart, no auto-sleep)
#   - Tailscale HTTPS serve rules
#   - launchd plists for backend, frontend, and update
#   - Docker Desktop auto-start on login
#   - Initial deploy
#
# Usage:
#   chmod +x deploy/macos/setup.sh
#   ./deploy/macos/setup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEPLOY_SCRIPT="$REPO_DIR/deploy/deploy.sh"
UPDATE_SCRIPT="$REPO_DIR/deploy/update.sh"
PLIST_DIR="$SCRIPT_DIR/plists"
LAUNCH_AGENTS="$HOME/Library/LaunchAgents"

echo "=== voilot macOS production setup ==="
echo "Repository: $REPO_DIR"
echo ""

# ── 1. Verify prerequisites ────────────────────────────────────────

echo "Checking prerequisites..."

missing=()

command -v go &>/dev/null || missing+=("go (https://go.dev/dl/)")
command -v node &>/dev/null || missing+=("node (https://nodejs.org/)")
command -v npm &>/dev/null || missing+=("npm (comes with node)")
command -v git &>/dev/null || missing+=("git")
command -v gh &>/dev/null || missing+=("gh (brew install gh)")
command -v docker &>/dev/null || missing+=("docker (Docker Desktop)")
command -v tailscale &>/dev/null || missing+=("tailscale (brew install --cask tailscale)")

if [ "${#missing[@]}" -gt 0 ]; then
  echo "ERROR: Missing prerequisites:"
  for m in "${missing[@]}"; do
    echo "  - $m"
  done
  exit 1
fi

if ! command -v brew &>/dev/null; then
  echo "ERROR: Homebrew not found. Install from https://brew.sh"
  exit 1
fi

# Verify GitHub CLI is authenticated (needed for git fetch from cron/launchd)
if ! gh auth status &>/dev/null; then
  echo "ERROR: GitHub CLI not authenticated. Run 'gh auth login' first."
  exit 1
fi

echo "Configuring git to use GitHub CLI for authentication..."
gh auth setup-git

echo "Prerequisites OK"
echo ""

# ── 2. Energy settings ─────────────────────────────────────────────

echo "Configuring energy settings (requires sudo)..."

# Wake for network access (required for WoL)
sudo pmset -a womp 1
# Start up automatically after power failure
sudo pmset -a autorestart 1
# Disable auto-sleep (managed by update script's idle detection)
sudo pmset -a sleep 0
# Display sleep after 5 minutes
sudo pmset -a displaysleep 5
# Disable Power Nap
sudo pmset -a powernap 0

echo "Energy settings applied"
echo ""

# ── 3. Tailscale HTTPS serve rules ────────────────────────────────

echo "Configuring Tailscale HTTPS..."

if tailscale status &>/dev/null; then
  tailscale serve --bg --https=443 http://localhost:3000 2>&1 | sed 's/^/  /'
  tailscale serve --bg --https=8080 http://localhost:8080 2>&1 | sed 's/^/  /'
  echo "Tailscale serve rules configured"
else
  echo "WARN: Tailscale not running, skipping serve rules. Configure manually later."
fi
echo ""

# ── 4. launchd plists ──────────────────────────────────────────────

echo "Installing launchd plists..."
mkdir -p "$LAUNCH_AGENTS"
mkdir -p "$REPO_DIR/tmp"

for plist in "$PLIST_DIR"/*.plist; do
  name="$(basename "$plist")"
  target="$LAUNCH_AGENTS/$name"
  label="${name%.plist}"

  # Unload existing plist if loaded
  if launchctl print "gui/$(id -u)/$label" &>/dev/null; then
    launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
  fi

  # Substitute repo path and install
  sed "s|__REPO_DIR__|$REPO_DIR|g" "$plist" > "$target"

  # Load (may fail if program doesn't exist yet — deploy.sh will restart)
  if launchctl bootstrap "gui/$(id -u)" "$target" 2>/dev/null; then
    echo "  installed and loaded $name"
  else
    echo "  installed $name (will start after deploy)"
  fi
done

echo "launchd plists installed"
echo ""

# ── 5. Docker Desktop auto-start on login ──────────────────────────

echo "Configuring Docker Desktop auto-start..."
osascript -e 'tell application "System Events" to make login item at end with properties {path:"/Applications/Docker.app", hidden:true}' 2>/dev/null || true
echo "Docker Desktop login item configured"
echo ""

# ── 6. Make scripts executable ─────────────────────────────────────

chmod +x "$DEPLOY_SCRIPT" "$UPDATE_SCRIPT"
chmod +x "$SCRIPT_DIR/run-backend.sh" "$SCRIPT_DIR/run-frontend.sh"
chmod +x "$SCRIPT_DIR/run-update.sh"

# ── 7. Initial deploy ─────────────────────────────────────────────

echo "Running initial deploy..."
"$DEPLOY_SCRIPT"
echo ""

# ── Done ───────────────────────────────────────────────────────────

echo "=== Setup complete ==="
echo ""
echo "What happens now:"
echo "  - Backend runs as pet.jen.voilot.backend (launchd)"
echo "  - Frontend runs as pet.jen.voilot.frontend (launchd)"
echo "  - Update checks every 30s as pet.jen.voilot.update (launchd)"
echo "  - TTS + STT run in Docker containers"
echo "  - After 30 minutes of no activity, the Mac sleeps automatically"
echo "  - Docker Desktop starts on login"
echo ""
echo "Logs:"
echo "  Backend:  $REPO_DIR/tmp/backend.log"
echo "  Frontend: $REPO_DIR/tmp/frontend.log"
echo "  Update:   $REPO_DIR/tmp/update.log"
echo ""
echo "Status: launchctl list | grep voilot"
