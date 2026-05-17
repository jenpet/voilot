#!/usr/bin/env bash
#
# voilot deploy — builds and deploys the production stack (v2).
#
# Builds the Go backend and Nuxt frontend, restarts launchd services,
# and brings up voice containers (TTS/STT) via Docker Compose.
#
# Shared by update.sh (automated) and task deploy (manual).
#
# Usage:
#   ./deploy/deploy.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_DIR="$SCRIPT_DIR/docker"
COMPOSE_PROJECT="${VOILOT_COMPOSE_PROJECT:-voilot}"

log() {
  echo "$(date -Iseconds) $1"
}

# ── Build metadata ─────────────────────────────────────────────────

export BUILD_HASH
BUILD_HASH="$(git -C "$REPO_DIR" rev-parse --short HEAD)"
export BUILD_TIME
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

log "INFO: Deploying ${BUILD_HASH} (built at ${BUILD_TIME})..."

# ── Build backend ──────────────────────────────────────────────────

log "INFO: Building backend..."
mkdir -p "$REPO_DIR/dist"
(
  cd "$REPO_DIR/backend"
  CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${BUILD_HASH} -X main.buildTime=${BUILD_TIME}" \
    -o "$REPO_DIR/dist/voilot-server" ./cmd/server
)
log "INFO: Backend built"

# ── Build frontend ─────────────────────────────────────────────────

log "INFO: Building frontend..."
(
  cd "$REPO_DIR/frontend"
  npm install --no-audit --no-fund 2>&1 | tail -1
  NUXT_PUBLIC_BUILD_HASH="$BUILD_HASH" \
  NUXT_PUBLIC_BUILD_TIME="$BUILD_TIME" \
  npx nuxt build 2>&1 | tail -5
)
log "INFO: Frontend built"

# ── Restart launchd services ───────────────────────────────────────

log "INFO: Restarting services..."
GUI_DOMAIN="gui/$(id -u)"

if launchctl print "$GUI_DOMAIN/pet.jen.voilot.backend" &>/dev/null; then
  launchctl kickstart -k "$GUI_DOMAIN/pet.jen.voilot.backend"
  log "INFO: Backend restarted"
else
  log "WARN: Backend launchd service not loaded, skipping restart"
fi

if launchctl print "$GUI_DOMAIN/pet.jen.voilot.frontend" &>/dev/null; then
  launchctl kickstart -k "$GUI_DOMAIN/pet.jen.voilot.frontend"
  log "INFO: Frontend restarted"
else
  log "WARN: Frontend launchd service not loaded, skipping restart"
fi

# ── Install agents & skills (idempotent symlinks) ──────────────────

log "INFO: Symlinking agents & skills..."

AGENTS_TARGET="$HOME/.config/opencode/agents"
SKILLS_TARGET="$HOME/.agents/skills"
mkdir -p "$AGENTS_TARGET" "$SKILLS_TARGET"

# Clean stale agent symlinks from any voilot checkout (dangling only)
for f in "$AGENTS_TARGET"/*.md; do
  [ -L "$f" ] && [[ "$(readlink "$f")" == */agents/opencode/* ]] && [ ! -e "$f" ] && rm "$f"
done

# Clean stale skill symlinks from any voilot checkout (dangling only)
for d in "$SKILLS_TARGET"/*/; do
  [ -L "${d%/}" ] && [[ "$(readlink "${d%/}")" == */agents/vendor/* ]] && [ ! -e "${d%/}" ] && rm "${d%/}"
done

# Symlink agent definitions
for f in "$REPO_DIR/agents/opencode/"*.md; do
  [ -f "$f" ] || continue
  target="$AGENTS_TARGET/$(basename "$f")"
  if [ -L "$target" ] && [[ "$(readlink "$target")" == */agents/opencode/* ]]; then
    ln -sf "$f" "$target"
  elif [ ! -e "$target" ]; then
    ln -sf "$f" "$target"
  else
    log "WARN: $target exists and is not ours, skipping"
  fi
done

# Symlink vendored skills
for d in "$REPO_DIR/agents/vendor"/*/; do
  [ -d "$d" ] || continue
  name="$(basename "$d")"
  target="$SKILLS_TARGET/$name"
  if [ -L "$target" ] && [[ "$(readlink "$target")" == */agents/vendor/* ]]; then
    ln -sfn "$d" "$target"
  elif [ ! -e "$target" ]; then
    ln -sfn "$d" "$target"
  else
    log "WARN: $target exists and is not ours, skipping"
  fi
done

log "INFO: Agents & skills installed"

# ── Docker Compose for voice (best-effort) ─────────────────────────

if docker info &>/dev/null; then
  log "INFO: Bringing up voice services..."
  docker compose \
    -p "$COMPOSE_PROJECT" \
    -f "$COMPOSE_DIR/docker-compose.yml" \
    --profile voice \
    up -d --build 2>&1 | tail -5
  log "INFO: Voice services up"
else
  log "WARN: Docker not available, skipping voice services"
fi

log "INFO: Deployed successfully (${BUILD_HASH})"
