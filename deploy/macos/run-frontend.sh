#!/usr/bin/env bash
#
# voilot frontend wrapper — sources .env and runs the Nuxt production server.
# Called by the pet.jen.voilot.frontend launchd plist.

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

export HOST=127.0.0.1
export PORT=3000

exec node "$REPO_DIR/frontend/.output/server/index.mjs"
