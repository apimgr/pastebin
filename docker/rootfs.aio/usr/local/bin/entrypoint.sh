#!/usr/bin/env bash
set -eo pipefail

# =============================================================================
# AIO Container Entrypoint Script
# Starts bundled redis-server (valkey-compatible cache) then execs main binary.
# Binary handles: directories, permissions, user/group, Tor, etc.
# =============================================================================

APP_NAME="pastebin"
APP_BIN="/usr/local/bin/${APP_NAME}"

# Export environment defaults (binary reads these)
export TZ="${TZ:-America/New_York}"
export CONFIG_DIR="${CONFIG_DIR:-/config/${APP_NAME}}"
export DATA_DIR="${DATA_DIR:-/data/${APP_NAME}}"

log() { echo "[entrypoint] $(date '+%Y-%m-%dT%H:%M:%S%z') $*"; }

# =============================================================================
# Start bundled redis-server (valkey-compatible in-process cache)
# Data lives under DATA_DIR so it persists via volume mounts.
# redis-server is started in background; tini reaps it when the container stops.
# =============================================================================
REDIS_DATA="${DATA_DIR}/cache/valkey"
mkdir -p "${REDIS_DATA}"
log "Starting redis-server (bundled cache)..."
redis-server \
    --daemonize no \
    --dir "${REDIS_DATA}" \
    --save "" \
    --loglevel warning \
    --bind 127.0.0.1 \
    --protected-mode yes &

# Wait up to 10s for redis to accept connections
READY=0
for i in $(seq 1 20); do
    redis-cli ping >/dev/null 2>&1 && { READY=1; break; }
    sleep 0.5
done
[ "${READY}" -eq 1 ] && log "Cache ready" || log "Cache not ready after 10s, proceeding"

# Point app at the bundled cache (can be overridden via CACHE_URL env var)
export CACHE_URL="${CACHE_URL:-redis://127.0.0.1:6379}"

# =============================================================================
# Start main application
# exec replaces this shell; tini (-p SIGTERM) propagates signals to all children
# including the background redis-server which is re-parented to tini after exec.
# =============================================================================
log "Starting ${APP_NAME}..."

FLAGS="--address ${ADDRESS:-0.0.0.0} --port ${PORT:-80}"
[ "${DEBUG:-false}" = "true" ] && FLAGS="$FLAGS --debug"

exec $APP_BIN $FLAGS "$@"
