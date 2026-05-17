#!/usr/bin/env bash
#
# voilot update wrapper — polls for changes and redeploys.
# Called by the pet.jen.voilot.update launchd plist.

set -euo pipefail

# launchd runs with minimal PATH; add Homebrew (Apple Silicon)
export PATH="/opt/homebrew/bin:$PATH"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

exec "$REPO_DIR/deploy/update.sh"
