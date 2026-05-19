#!/usr/bin/env bash
#
# voilot update script — polls origin/main and redeploys on changes.
# Also manages office-hours power mode (auto-sleep toggle).
#
# Environment variables:
#   VOILOT_UPDATE_INTERVAL     Poll interval in seconds (default: 60)
#   VOILOT_BACKEND_URL         Backend URL for health checks (default: http://localhost:8080)
#   VOILOT_REPO_DIR            Repository root (default: script's parent directory)
#
# Designed to run from cron (single invocation) or as a long-running loop.
# When invoked without arguments, runs a single check-and-deploy cycle.
# With --loop, polls continuously at VOILOT_UPDATE_INTERVAL.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="${VOILOT_REPO_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"

UPDATE_INTERVAL="${VOILOT_UPDATE_INTERVAL:-60}"
BACKEND_URL="${VOILOT_BACKEND_URL:-http://localhost:8080}"
POWER_MODE_FILE="$REPO_DIR/tmp/.power-mode"

log() {
  echo "$(date -Iseconds) $1"
}

# Check for updates and deploy if main has moved forward.
check_and_deploy() {
  cd "$REPO_DIR"

  log "INFO: Fetching origin/main..."
  if ! git fetch origin main 2>&1; then
    log "ERROR: git fetch failed"
    return 1
  fi

  local local_sha
  local remote_sha
  local_sha="$(git rev-parse HEAD)"
  remote_sha="$(git rev-parse origin/main)"

  if [ "$local_sha" = "$remote_sha" ]; then
    log "INFO: Up to date ($local_sha)"
    return 0
  fi

  log "INFO: Update available: $local_sha -> $remote_sha"

  if ! git pull --ff-only origin main 2>&1; then
    log "ERROR: git pull --ff-only failed"
    return 1
  fi

  log "INFO: Building and deploying..."
  if ! "$SCRIPT_DIR/deploy.sh" 2>&1; then
    log "ERROR: deploy.sh failed"
    return 1
  fi
}

# Toggle macOS auto-sleep based on office hours (06:00-23:00 local time).
# Only calls pmset on day/night transitions to avoid redundant writes.
check_power_mode() {
  local hour
  hour="$(date +%H)"
  local desired_mode

  if [ "$hour" -ge 6 ] && [ "$hour" -lt 23 ]; then
    desired_mode="day"
  else
    desired_mode="night"
  fi

  local current_mode
  current_mode="$(cat "$POWER_MODE_FILE" 2>/dev/null || echo "")"

  if [ "$current_mode" != "$desired_mode" ]; then
    case "$desired_mode" in
      day)
        sudo pmset -a sleep 0
        log "INFO: Office hours started — auto-sleep disabled"
        ;;
      night)
        sudo pmset -a sleep 15
        log "INFO: Office hours ended — auto-sleep enabled (15 min)"
        ;;
    esac
    echo "$desired_mode" > "$POWER_MODE_FILE"
  fi
}

# Handle SIGTERM for clean shutdown
trap 'log "INFO: Received SIGTERM, exiting"; exit 0' SIGTERM SIGINT

# Main
if [ "${1:-}" = "--loop" ]; then
  log "INFO: Starting continuous update loop (interval: ${UPDATE_INTERVAL}s)"
  while true; do
    check_and_deploy || true
    check_power_mode || true
    sleep "$UPDATE_INTERVAL"
  done
else
  check_and_deploy || true
  check_power_mode || true
fi
