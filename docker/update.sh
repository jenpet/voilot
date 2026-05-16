#!/usr/bin/env bash
#
# voilot update script — polls origin/main and redeploys on changes.
# Also checks backend idle time and triggers platform-specific sleep.
#
# Environment variables:
#   VOILOT_UPDATE_INTERVAL     Poll interval in seconds (default: 60)
#   VOILOT_IDLE_TIMEOUT        Idle timeout in minutes before sleep (default: 30)
#   VOILOT_COMPOSE_PROFILES    Compose profiles to activate (default: voice, passed to deploy.sh)
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
IDLE_TIMEOUT="${VOILOT_IDLE_TIMEOUT:-30}"
BACKEND_URL="${VOILOT_BACKEND_URL:-http://localhost:8080}"

log() {
  echo "$(date -Iseconds) $1"
}

# Wait for Docker daemon to become ready (up to 60 seconds).
wait_for_docker() {
  local max_attempts=30
  local attempt=1
  while [ "$attempt" -le "$max_attempts" ]; do
    if docker info >/dev/null 2>&1; then
      return 0
    fi
    log "INFO: Waiting for Docker daemon (attempt $attempt/$max_attempts)..."
    sleep 2
    attempt=$((attempt + 1))
  done
  log "ERROR: Docker daemon not ready after 60s, aborting"
  return 1
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

# Check backend idle time and trigger sleep if idle too long.
check_idle_and_sleep() {
  local health_url="$BACKEND_URL/api/health/detailed"
  local response

  response="$(curl -sf "$health_url" 2>/dev/null)" || {
    log "WARN: Could not reach backend health endpoint, skipping idle check"
    return 0
  }

  # Extract lastActivityAt (ISO 8601) — requires the backend to include it.
  local last_activity
  last_activity="$(echo "$response" | grep -o '"lastActivityAt":"[^"]*"' | cut -d'"' -f4)" || true

  if [ -z "$last_activity" ]; then
    log "DEBUG: No lastActivityAt in health response, skipping idle check"
    return 0
  fi

  # Convert to epoch seconds. date -d works on Linux, date -jf on macOS.
  local last_epoch now_epoch idle_minutes
  if date -d "$last_activity" +%s >/dev/null 2>&1; then
    last_epoch="$(date -d "$last_activity" +%s)"
  elif date -jf "%Y-%m-%dT%H:%M:%S" "$(echo "$last_activity" | cut -c1-19)" +%s >/dev/null 2>&1; then
    last_epoch="$(date -jf "%Y-%m-%dT%H:%M:%S" "$(echo "$last_activity" | cut -c1-19)" +%s)"
  else
    log "WARN: Could not parse lastActivityAt: $last_activity"
    return 0
  fi

  now_epoch="$(date +%s)"
  idle_minutes=$(( (now_epoch - last_epoch) / 60 ))

  if [ "$idle_minutes" -ge "$IDLE_TIMEOUT" ]; then
    log "INFO: Backend idle for ${idle_minutes}m (timeout: ${IDLE_TIMEOUT}m), triggering sleep..."

    # Platform-specific sleep
    case "$(uname -s)" in
      Darwin)
        pmset sleepnow
        ;;
      Linux)
        systemctl suspend
        ;;
      *)
        log "WARN: Unsupported platform for sleep: $(uname -s)"
        ;;
    esac
  else
    log "DEBUG: Backend active ${idle_minutes}m ago (timeout: ${IDLE_TIMEOUT}m)"
  fi
}

# Handle SIGTERM for clean shutdown
trap 'log "INFO: Received SIGTERM, exiting"; exit 0' SIGTERM SIGINT

# Main
wait_for_docker || exit 1

if [ "${1:-}" = "--loop" ]; then
  log "INFO: Starting continuous update loop (interval: ${UPDATE_INTERVAL}s, idle timeout: ${IDLE_TIMEOUT}m)"
  while true; do
    check_and_deploy || true
    check_idle_and_sleep || true
    sleep "$UPDATE_INTERVAL"
  done
else
  check_and_deploy || true
  check_idle_and_sleep || true
fi
