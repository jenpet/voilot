#!/bin/sh
# tailscale-dev.sh — Manage Tailscale HTTPS proxies for local dev
#
# Sets up (or tears down) tailscale serve to proxy the Nuxt dev server
# and Go backend over HTTPS, prints the URL as a clickable link and QR
# code for easy phone access.
#
# Usage:
#   ./scripts/tailscale-dev.sh              # start tunnel (idempotent)
#   ./scripts/tailscale-dev.sh down         # stop tunnel
#   ./scripts/tailscale-dev.sh 3001         # custom frontend port
#   ./scripts/tailscale-dev.sh 3001 9090    # custom frontend + backend ports
#
# Prerequisites:
#   - Tailscale installed and running (`brew install --cask tailscale`)
#   - qrencode installed (`brew install qrencode`)

set -e

# ── Preflight checks ────────────────────────────────────────────────
if ! command -v tailscale >/dev/null 2>&1; then
	echo "Error: tailscale not found. Install with: brew install --cask tailscale" >&2
	exit 1
fi

if ! tailscale status >/dev/null 2>&1; then
	echo "Error: Tailscale is not running. Start the Tailscale app first." >&2
	exit 1
fi

# ── Handle "down" ───────────────────────────────────────────────────
if [ "$1" = "down" ]; then
	echo "Tearing down Tailscale HTTPS proxies..."
	tailscale serve reset
	echo "Tunnel stopped."
	exit 0
fi

# ── Resolve ports ───────────────────────────────────────────────────
FRONTEND_PORT="${1:-3000}"
BACKEND_PORT="${2:-8080}"

# ── Resolve the machine's Tailscale FQDN ────────────────────────────
# Extract Self.DNSName from the JSON status. The trailing dot in the
# FQDN is stripped (e.g., "foo.tailnet.ts.net." → "foo.tailnet.ts.net").
FQDN=$(tailscale status --json 2>/dev/null |
	python3 -c "import sys,json; print(json.load(sys.stdin).get('Self',{}).get('DNSName','').rstrip('.'))" \
		2>/dev/null)

if [ -z "$FQDN" ]; then
	echo "Error: Could not determine Tailscale FQDN. Is Tailscale connected?" >&2
	exit 1
fi

URL="https://${FQDN}"

# ── Set up HTTPS proxies (idempotent) ───────────────────────────────
echo "Setting up Tailscale HTTPS proxies..."
echo ""

tailscale serve --bg --https=443 "http://localhost:${FRONTEND_PORT}" 2>&1 | sed 's/^/  /'
tailscale serve --bg --https="${BACKEND_PORT}" "http://localhost:${BACKEND_PORT}" 2>&1 | sed 's/^/  /'

echo ""

# ── Print clickable link ────────────────────────────────────────────
# OSC 8 hyperlink escape: clickable in most modern terminals
printf "\033]8;;%s\033\\  %s\033]8;;\033\\\n" "$URL" "$URL"
echo ""

# ── Print QR code ───────────────────────────────────────────────────
if command -v qrencode >/dev/null 2>&1; then
	qrencode -t ANSIUTF8 "$URL"
	echo ""
fi

echo "Scan the QR code or click the link to open voilot on your phone."
echo "Run 'task dev:tunnel -- down' to tear down."
