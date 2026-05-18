#!/usr/bin/env bash
#
# Tailscale CLI setup (dev + production)
#
# Installs tailscaled as a LaunchDaemon, authenticates, and configures
# HTTPS serve rules. Safe to run on both dev and production machines.
#
# Prerequisites:
#   - brew install tailscale (formula, NOT cask)
#
# Usage:
#   ./deploy/macos/setup-tailscale.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PLIST_DIR="$SCRIPT_DIR/plists"

echo "=== Tailscale CLI setup (no GUI app/network extension) ==="
echo ""

# ── Prereq check ───────────────────────────────────────────────────

if ! [ -x /opt/homebrew/bin/tailscale ]; then
  echo "ERROR: tailscale not found at /opt/homebrew/bin/tailscale"
  echo "       Install with: brew install tailscale (formula, NOT cask)"
  exit 1
fi

TAILSCALE=/opt/homebrew/bin/tailscale

# ── Install LaunchDaemon ───────────────────────────────────────────

TAILSCALE_PLIST_SRC="$PLIST_DIR/pet.jen.voilot.tailscale.plist"
TAILSCALE_PLIST_DST="/Library/LaunchDaemons/pet.jen.voilot.tailscale.plist"
TAILSCALE_STATE_DIR="/var/lib/tailscale"

echo "Installing Tailscale LaunchDaemon (requires sudo)..."

# Create state directory
sudo mkdir -p "$TAILSCALE_STATE_DIR"

# Install plist (runs as root)
sed "s|__REPO_DIR__|$REPO_DIR|g" "$TAILSCALE_PLIST_SRC" > /tmp/pet.jen.voilot.tailscale.plist
sudo mv /tmp/pet.jen.voilot.tailscale.plist "$TAILSCALE_PLIST_DST"
sudo chown root:wheel "$TAILSCALE_PLIST_DST"
sudo chmod 644 "$TAILSCALE_PLIST_DST"

# Load the daemon (or restart if already loaded)
if sudo launchctl list pet.jen.voilot.tailscale &>/dev/null; then
  sudo launchctl kickstart -k system/pet.jen.voilot.tailscale
  echo "  Tailscale daemon restarted"
else
  sudo launchctl bootstrap system "$TAILSCALE_PLIST_DST"
  echo "  Tailscale daemon installed and started"
fi

# Wait for daemon to be ready
sleep 2

# ── Authenticate ───────────────────────────────────────────────────

echo ""
echo "Authenticating Tailscale (follow the URL in your browser)..."
HOSTNAME_SHORT="$(hostname -s)"
$TAILSCALE up --accept-routes=false --hostname="$HOSTNAME_SHORT"
echo "  Tailscale authenticated as $HOSTNAME_SHORT"

# ── Configure serve rules ──────────────────────────────────────────

echo ""
echo "Configuring Tailscale HTTPS serve rules..."
$TAILSCALE serve --bg --https=443 http://localhost:3000 2>&1 | sed 's/^/  /'
$TAILSCALE serve --bg --https=8080 http://localhost:8080 2>&1 | sed 's/^/  /'
echo "Tailscale serve rules configured"

echo ""
echo "=== Tailscale setup complete ==="
echo ""
echo "Verify with: /opt/homebrew/bin/tailscale status && /opt/homebrew/bin/tailscale serve status"
