#!/usr/bin/env bash
#
# voilot backend wrapper — sources .env and runs the server binary.
# Called by the pet.jen.voilot.backend launchd plist.

set -euo pipefail

# launchd runs with minimal PATH; add Homebrew (Apple Silicon)
export PATH="/opt/homebrew/bin:$PATH"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source environment (same file as dev)
if [ -f "$REPO_DIR/.env" ]; then
  set -a
  # shellcheck source=/dev/null
  . "$REPO_DIR/.env"
  set +a
fi

exec "$REPO_DIR/dist/voilot-server" \
  --tts-url "${TTS_URL:-http://localhost:8880}" \
  --stt-url "${STT_URL:-http://localhost:5003}" \
  --workspace-dir "${WORKSPACE_DIR:-$HOME/tmp/voilot-wd}" \
  --data-dir "${DATA_DIR:-$HOME/tmp/voilot-wd/voilot-data}" \
  --hostname 127.0.0.1 \
  --port 8080 \
  --cors-origins "${CORS_ORIGINS:-}" \
  --log-level "${LOG_LEVEL:-info}" \
  --log-format "${LOG_FORMAT:-text}"
