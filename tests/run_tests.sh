#!/usr/bin/env bash
# Integration test runner — auto-detects incus or docker.
# Usage: ./tests/run_tests.sh
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="pastebin"
PROJECT_ORG="apimgr"
TEST_NETWORK="${PROJECT_ORG}-${PROJECT_NAME}-test"

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

# Detect runtime
if command -v incus >/dev/null 2>&1; then
    RUNTIME="incus"
elif command -v docker >/dev/null 2>&1; then
    RUNTIME="docker"
else
    die "Neither incus nor docker found. Install one to run integration tests."
fi

log "Runtime: ${RUNTIME}"

run_docker_tests() {
    local image="${PROJECT_ORG}/${PROJECT_NAME}:test"
    local container="${PROJECT_ORG}-${PROJECT_NAME}-test-$$"

    # Build test image
    log "Building test image..."
    docker build \
        -f "${PROJECT_DIR}/docker/Dockerfile" \
        -t "${image}" \
        "${PROJECT_DIR}"

    # Create test network
    docker network create "${TEST_NETWORK}" 2>/dev/null || true

    # Run container
    log "Starting test container..."
    docker run --rm -d \
        --name "${container}" \
        --network "${TEST_NETWORK}" \
        -p 0:80 \
        "${image}"

    # Wait for server
    log "Waiting for server..."
    local port
    port=$(docker port "${container}" 80/tcp | cut -d: -f2)
    local attempts=0
    until curl -sf "http://localhost:${port}/server/healthz" >/dev/null 2>&1; do
        attempts=$((attempts + 1))
        [ "${attempts}" -gt 30 ] && { docker stop "${container}"; die "Server did not start"; }
        sleep 2
    done

    log "Server ready on port ${port}"

    # Basic smoke tests
    local base_url="http://localhost:${port}"
    local errors=0

    test_endpoint() {
        local desc="$1" method="$2" url="$3" expected_code="$4"
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" -X "${method}" "${url}")
        if [ "${code}" = "${expected_code}" ]; then
            log "PASS: ${desc} (${code})"
        else
            log "FAIL: ${desc} — expected ${expected_code}, got ${code}"
            errors=$((errors + 1))
        fi
    }

    test_endpoint "health check"          GET  "${base_url}/server/healthz"  200
    test_endpoint "home page"             GET  "${base_url}/"                 200
    test_endpoint "recent pastes"         GET  "${base_url}/recent"           200
    test_endpoint "api pastes list"       GET  "${base_url}/api/v1/pastes"   200
    test_endpoint "swagger docs"          GET  "${base_url}/api/v1/server/swagger" 200
    test_endpoint "graphql endpoint"      GET  "${base_url}/graphql"         200
    test_endpoint "nonexistent paste"     GET  "${base_url}/notfound123456"  404
    test_endpoint "auth compat (login)"   GET  "${base_url}/login"           302

    # Create a paste
    local create_response
    create_response=$(curl -s -X POST "${base_url}/api/v1/paste" \
        -H 'Content-Type: application/json' \
        -d '{"content":"test paste from integration test","language":"text"}')
    local paste_id
    paste_id=$(printf '%s' "${create_response}" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

    if [ -n "${paste_id}" ]; then
        log "PASS: create paste (id: ${paste_id})"
        test_endpoint "view paste"  GET "${base_url}/${paste_id}" 200
        test_endpoint "raw paste"   GET "${base_url}/raw/${paste_id}" 200
    else
        log "FAIL: create paste — no id in response"
        errors=$((errors + 1))
    fi

    # Cleanup
    docker stop "${container}" 2>/dev/null || true
    docker network rm "${TEST_NETWORK}" 2>/dev/null || true

    if [ "${errors}" -gt 0 ]; then
        die "${errors} test(s) failed"
    fi
    log "All tests passed"
}

case "${RUNTIME}" in
    docker) run_docker_tests ;;
    incus)  log "Incus integration tests not yet implemented — falling back to docker"; run_docker_tests ;;
esac
