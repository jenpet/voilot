#!/usr/bin/env bash
#
# voilot macOS production setup
#
# One-time script that configures a Mac as a voilot production server:
#   - Verify prerequisites (Go, Node.js, Docker, Tailscale, sleepwatcher)
#   - Energy settings (WoL, auto-restart, no auto-sleep)
#   - Tailscale HTTPS serve rules
#   - launchd plists for backend + frontend
#   - Docker Desktop auto-start on login
#   - Cron job for periodic update checks
#   - Sleepwatcher wake hook for immediate update on wake
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
DEPLOY_LOG="$REPO_DIR/tmp/deploy.log"
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

if ! brew list sleepwatcher &>/dev/null; then
  echo "Installing sleepwatcher..."
  brew install sleepwatcher
fi

# Start sleepwatcher service if not running
if ! brew services list | grep sleepwatcher | grep -q started; then
  echo "Starting sleepwatcher service..."
  brew services start sleepwatcher
fi

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

  # Load
  launchctl bootstrap "gui/$(id -u)" "$target"
  echo "  installed $name"
done

echo "launchd plists installed"
echo ""

# ── 5. Docker Desktop auto-start on login ──────────────────────────

echo "Configuring Docker Desktop auto-start..."
osascript -e 'tell application "System Events" to make login item at end with properties {path:"/Applications/Docker.app", hidden:true}' 2>/dev/null || true
echo "Docker Desktop login item configured"
echo ""

# ── 6. Cron job ────────────────────────────────────────────────────

echo "Installing cron job..."

CRON_ENTRY="* * * * * $UPDATE_SCRIPT >> $DEPLOY_LOG 2>&1 # voilot-update"

# Remove any existing voilot-update cron entry (handles relocation)
crontab -l 2>/dev/null | grep -v '# voilot-update' | crontab - 2>/dev/null || true

# Add the new entry
(crontab -l 2>/dev/null || true; echo "$CRON_ENTRY") | crontab -
echo "Cron entry installed"
echo ""

# ── 7. Sleepwatcher wake hook ──────────────────────────────────────

echo "Configuring sleepwatcher wake hook..."

WAKEUP_SCRIPT="$HOME/.wakeup"
cat > "$WAKEUP_SCRIPT" << EOF
#!/usr/bin/env bash
# Triggered by sleepwatcher on wake — run voilot update immediately
$UPDATE_SCRIPT >> $DEPLOY_LOG 2>&1
EOF
chmod +x "$WAKEUP_SCRIPT"

echo "Wake hook installed: $WAKEUP_SCRIPT"
echo ""

# ── 8. Make scripts executable ─────────────────────────────────────

chmod +x "$DEPLOY_SCRIPT" "$UPDATE_SCRIPT"
chmod +x "$SCRIPT_DIR/run-backend.sh" "$SCRIPT_DIR/run-frontend.sh"

# ── 9. Initial deploy ─────────────────────────────────────────────

echo "Running initial deploy..."
"$DEPLOY_SCRIPT"
echo ""

# ── Done ───────────────────────────────────────────────────────────

echo "=== Setup complete ==="
echo ""
echo "What happens now:"
echo "  - Backend runs as pet.jen.voilot.backend (launchd)"
echo "  - Frontend runs as pet.jen.voilot.frontend (launchd)"
echo "  - TTS + STT run in Docker containers"
echo "  - Every minute, cron runs the update script"
echo "  - On wake from sleep, sleepwatcher triggers an immediate update"
echo "  - After 30 minutes of no activity, the Mac sleeps automatically"
echo "  - Docker Desktop starts on login"
echo ""
echo "Logs: $DEPLOY_LOG"
echo "Backend log: $REPO_DIR/tmp/backend.log"
echo "Frontend log: $REPO_DIR/tmp/frontend.log"
echo ""
echo "To test: $UPDATE_SCRIPT"
