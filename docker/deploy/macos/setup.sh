#!/usr/bin/env bash
#
# voilot macOS production setup
#
# One-time script that configures a Mac as a voilot production server:
#   - Energy settings (WoL, auto-restart, no auto-sleep)
#   - Docker Desktop auto-start on login
#   - Cron job for periodic update checks
#   - Sleepwatcher wake hook for immediate update on wake
#
# Prerequisites:
#   - Homebrew installed
#   - sleepwatcher installed: brew install sleepwatcher
#   - Docker Desktop installed
#   - Git authentication configured (SSH key or credential helper)
#
# Usage:
#   chmod +x docker/deploy/macos/setup.sh
#   ./docker/deploy/macos/setup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
UPDATE_SCRIPT="$REPO_DIR/docker/update.sh"
DEPLOY_LOG="$REPO_DIR/tmp/deploy.log"

echo "=== voilot macOS production setup ==="
echo "Repository: $REPO_DIR"
echo "Update script: $UPDATE_SCRIPT"
echo ""

# ── 1. Verify prerequisites ────────────────────────────────────────

echo "Checking prerequisites..."

if ! command -v brew &>/dev/null; then
  echo "ERROR: Homebrew not found. Install from https://brew.sh"
  exit 1
fi

if ! command -v docker &>/dev/null; then
  echo "ERROR: Docker not found. Install Docker Desktop first."
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

# Display sleep after 5 minutes (saves energy, screen doesn't matter for a server)
sudo pmset -a displaysleep 5

# Disable Power Nap (no background activity during sleep)
sudo pmset -a powernap 0

echo "Energy settings applied"
echo ""

# ── 3. Docker Desktop auto-start on login ──────────────────────────

echo "Configuring Docker Desktop auto-start..."
osascript -e 'tell application "System Events" to make login item at end with properties {path:"/Applications/Docker.app", hidden:true}' 2>/dev/null || true
echo "Docker Desktop login item configured"
echo ""

echo "Creating Docker volume for persistent data..."
docker volume create voilot-data 2>/dev/null || true
echo "Docker volume ready"
echo ""

# ── 4. Cron job ────────────────────────────────────────────────────

echo "Installing cron job..."

CRON_ENTRY="* * * * * $UPDATE_SCRIPT >> $DEPLOY_LOG 2>&1"

# Check if entry already exists
if crontab -l 2>/dev/null | grep -qF "$UPDATE_SCRIPT"; then
  echo "Cron entry already exists, skipping"
else
  # Append to existing crontab (or create new one)
  (crontab -l 2>/dev/null || true; echo "$CRON_ENTRY") | crontab -
  echo "Cron entry added: $CRON_ENTRY"
fi
echo ""

# ── 5. Sleepwatcher wake hook ──────────────────────────────────────

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

# ── Done ───────────────────────────────────────────────────────────

echo "=== Setup complete ==="
echo ""
echo "What happens now:"
echo "  - Every minute, cron runs the update script"
echo "  - On wake from sleep, sleepwatcher triggers an immediate update"
echo "  - After 30 minutes of no activity, the Mac sleeps automatically"
echo "  - Docker Desktop starts on login"
echo "  - On power failure, the Mac restarts automatically"
echo ""
echo "Logs: $DEPLOY_LOG"
echo "To test: $UPDATE_SCRIPT"
