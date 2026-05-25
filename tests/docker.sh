#!/usr/bin/env bash
# tests/docker.sh — Docker integration tests for pastebin
# Runs the server in a Docker container and exercises the API.
# Usage: bash tests/docker.sh [--keep]
# Options:
#   --keep   Do not remove the container after the test run

set -euo pipefail

# ── Variables ────────────────────────────────────────────────────────────────
SCRIPT_NAME="$(basename -- "$0")"
PROJECT_DIR="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
CONTAINER_NAME="pastebin-test-$$"
IMAGE_NAME="pastebin:test-$$"
PORT="18080"
BASE_URL="http://127.0.0.1:${PORT}"
KEEP=false

# ── Helpers ──────────────────────────────────────────────────────────────────
pass() { printf "[PASS] %s\n" "$*"; }
fail() { printf "[FAIL] %s\n" "$*" >&2; exit 1; }
info() { printf "[INFO] %s\n" "$*"; }

cleanup() {
    if [[ "$KEEP" == "false" ]]; then
        info "Removing container ${CONTAINER_NAME}"
        docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
        docker rmi -f "${IMAGE_NAME}" 2>/dev/null || true
    else
        info "Keeping container ${CONTAINER_NAME} (--keep flag set)"
    fi
}
trap cleanup EXIT

# ── Argument parsing ─────────────────────────────────────────────────────────
for arg in "$@"; do
    case "$arg" in
        --keep) KEEP=true ;;
        *) printf "%s: unknown argument: %s\n" "${SCRIPT_NAME}" "$arg" >&2; exit 2 ;;
    esac
done

# ── Build test image ─────────────────────────────────────────────────────────
info "Building test image ${IMAGE_NAME}"
docker build \
    --file "${PROJECT_DIR}/docker/Dockerfile.dev" \
    --tag "${IMAGE_NAME}" \
    "${PROJECT_DIR}"

# ── Start container ──────────────────────────────────────────────────────────
info "Starting container ${CONTAINER_NAME} on port ${PORT}"
docker run --detach \
    --name "${CONTAINER_NAME}" \
    --publish "127.0.0.1:${PORT}:80" \
    --env MODE=development \
    --env DEBUG=true \
    --tmpfs /data \
    --tmpfs /config \
    "${IMAGE_NAME}"

# Wait for server to become ready (up to 30 s)
info "Waiting for server to start..."
for i in $(seq 1 30); do
    if curl --silent --fail "${BASE_URL}/healthz" >/dev/null 2>&1; then
        break
    fi
    sleep 1
    if [[ "$i" -eq 30 ]]; then
        docker logs "${CONTAINER_NAME}" >&2
        fail "Server did not start within 30 seconds"
    fi
done
pass "Server is up"

# ── Tests ─────────────────────────────────────────────────────────────────────

# 1. Create a paste (native API)
RESPONSE=$(curl --silent --fail \
    --header "Content-Type: application/json" \
    --data '{"content":"hello from docker test","language":"text"}' \
    "${BASE_URL}/api/v1/paste")
PASTE_ID=$(printf '%s' "${RESPONSE}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
[[ -n "${PASTE_ID}" ]] || fail "Create paste: no ID in response: ${RESPONSE}"
pass "Create paste: id=${PASTE_ID}"

# 2. Retrieve the paste
GET=$(curl --silent --fail "${BASE_URL}/api/v1/paste/${PASTE_ID}")
printf '%s' "${GET}" | grep -q "hello from docker test" || \
    fail "Get paste: content mismatch: ${GET}"
pass "Get paste: content verified"

# 3. Get raw text
RAW=$(curl --silent --fail "${BASE_URL}/api/v1/paste/${PASTE_ID}/raw")
[[ "${RAW}" == "hello from docker test" ]] || \
    fail "Raw paste: unexpected content: ${RAW}"
pass "Get raw paste: verified"

# 4. List pastes
LIST=$(curl --silent --fail "${BASE_URL}/api/v1/pastes")
printf '%s' "${LIST}" | grep -q "${PASTE_ID}" || \
    fail "List pastes: paste ID not found"
pass "List pastes: verified"

# 5. Health check
HEALTH=$(curl --silent --fail "${BASE_URL}/healthz")
printf '%s' "${HEALTH}" | grep -qi "ok\|true\|healthy" || \
    fail "Health check: unexpected response: ${HEALTH}"
pass "Health check: ok"

# 6. --status flag
docker exec "${CONTAINER_NAME}" /usr/local/bin/pastebin --status >/dev/null || \
    fail "--status exited non-zero"
pass "--status: ok"

info "All Docker tests passed."
