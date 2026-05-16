#!/usr/bin/env bash
#
# voilot deploy — builds and deploys the production Docker stack.
#
# Exports BUILD_HASH and BUILD_TIME, then runs docker compose with
# production overrides. Shared by update.sh (automated) and task deploy (manual).
#
# Usage:
#   ./docker/deploy.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

COMPOSE_PROFILES="${VOILOT_COMPOSE_PROFILES:-voice}"

# Ensure persistent data volume exists (idempotent)
docker volume create voilot-data 2>/dev/null || true

export BUILD_HASH
BUILD_HASH="$(git -C "$REPO_DIR" rev-parse --short HEAD)"
export BUILD_TIME
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "$(date -Iseconds) INFO: Deploying ${BUILD_HASH} (built at ${BUILD_TIME})..."

docker compose \
  -f "$SCRIPT_DIR/docker-compose.yml" \
  -f "$SCRIPT_DIR/docker-compose.production.yml" \
  --profile "$COMPOSE_PROFILES" \
  up --build -d

echo "$(date -Iseconds) INFO: Deployed successfully (${BUILD_HASH})"
