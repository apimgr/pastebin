#!/usr/bin/env bash
# @@License : MIT
# tests/incus.sh — Incus integration tests for pastebin
# Launches an Incus system container (Debian, systemd), installs the built
# binaries, and exercises the full systemd service workflow plus the same
# API/content-negotiation/endpoint coverage as tests/docker.sh, per PART 28.
# Usage: bash tests/incus.sh [--keep]
# Options:
#   --keep   Do not delete the container after the test run

set -eo pipefail

# ── Variables ────────────────────────────────────────────────────────────────
INCUS_SCRIPT_NAME="$(basename -- "$0")"
INCUS_PROJECT_DIR="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
INCUS_PROJECT_ORG="apimgr"
INCUS_PROJECT_NAME="pastebin"
INCUS_CONTAINER_NAME="test-${INCUS_PROJECT_NAME}-$$"
INCUS_IMAGE="images:debian/trixie"
INCUS_KEEP=false
INCUS_PORT="80"
INCUS_ERRORS=0

# ── Helpers ──────────────────────────────────────────────────────────────────
__pass() { printf "[PASS] %s\n" "$*"; }
__fail() { printf "[FAIL] %s\n" "$*" >&2; INCUS_ERRORS=$((INCUS_ERRORS + 1)); }
__info() { printf "[INFO] %s\n" "$*"; }
__vm()   { incus exec "${INCUS_CONTAINER_NAME}" -- bash -c "$*"; }

__check_status() {
    local desc="$1" method="$2" url="$3" expected="$4"
    shift 4
    local code
    code=$(__vm "curl -s -o /dev/null -w '%{http_code}' -X ${method} $* '${url}'")
    if [[ "${code}" == "${expected}" ]]; then
        __pass "${desc} -> ${code}"
    else
        __fail "${desc} -> expected ${expected}, got ${code}"
    fi
}

__check_body() {
    local desc="$1" url="$2" pattern="$3"
    shift 3
    local body
    body=$(__vm "curl -sf $* '${url}'" 2>/dev/null || true)
    if printf '%s' "${body}" | grep -qi -- "${pattern}"; then
        __pass "${desc}"
    else
        __fail "${desc} — pattern '${pattern}' not found in: ${body}"
    fi
}

__cleanup() {
    if [[ "${INCUS_KEEP}" == "false" ]]; then
        __info "Deleting container ${INCUS_CONTAINER_NAME}"
        incus delete --force "${INCUS_CONTAINER_NAME}" 2>/dev/null || true
    else
        __info "Keeping container ${INCUS_CONTAINER_NAME} (--keep flag set)"
    fi
}
trap __cleanup EXIT

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep) INCUS_KEEP=true; shift ;;
        *) printf "%s: unknown argument: %s\n" "${INCUS_SCRIPT_NAME}" "$1" >&2; exit 2 ;;
    esac
done

# ── Prerequisite checks ───────────────────────────────────────────────────────
command -v incus >/dev/null 2>&1 || { echo "ERROR: incus not found. Install incus or use tests/docker.sh" >&2; exit 1; }

# ── Build — use Makefile if present (output always lands in binaries/) ───────
cd "${INCUS_PROJECT_DIR}"
if [[ -f "Makefile" ]]; then
    __info "Building with make build..."
    make build
else
    __info "Building in Docker (no Makefile)..."
    INCUS_GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
    INCUS_GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/${INCUS_PROJECT_NAME}}"
    mkdir -p "${INCUS_GO_CACHE}" "${INCUS_GO_BUILD}" binaries
    docker run --rm \
        --name "${INCUS_PROJECT_NAME}-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" \
        -v "${INCUS_PROJECT_DIR}":/app \
        -v "${INCUS_GO_CACHE}":/usr/local/share/go/pkg/mod \
        -v "${INCUS_GO_BUILD}":/usr/local/share/go/cache \
        -w /app -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false \
        casjaysdev/go:latest go build -buildvcs=false -trimpath -ldflags "-s -w" -o /app/binaries/"${INCUS_PROJECT_NAME}" ./src
    if [[ -d "src/client" ]]; then
        docker run --rm \
            --name "${INCUS_PROJECT_NAME}-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" \
            -v "${INCUS_PROJECT_DIR}":/app \
            -v "${INCUS_GO_CACHE}":/usr/local/share/go/pkg/mod \
            -v "${INCUS_GO_BUILD}":/usr/local/share/go/cache \
            -w /app -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false \
            casjaysdev/go:latest go build -buildvcs=false -trimpath -ldflags "-s -w" -o /app/binaries/"${INCUS_PROJECT_NAME}"-cli ./src/client
    fi
fi
[[ -f "binaries/${INCUS_PROJECT_NAME}" ]] || { echo "ERROR: binary not found after build" >&2; exit 1; }
__pass "Binaries built in binaries/"

# ── Launch container ─────────────────────────────────────────────────────────
__info "Launching Incus container (Debian + systemd) ${INCUS_CONTAINER_NAME}"
incus launch "${INCUS_IMAGE}" "${INCUS_CONTAINER_NAME}"

__info "Waiting for container to boot..."
for ((i = 1; i <= 60; i++)); do
    if __vm "systemctl is-system-running --wait" >/dev/null 2>&1 || \
       __vm "systemctl is-system-running" 2>/dev/null | grep -q -- "running\|degraded"; then
        break
    fi
    sleep 2
    [[ "${i}" -lt 60 ]] || { echo "ERROR: container did not come up in 120 seconds" >&2; exit 1; }
done
__pass "Container booted"

# ── Copy binaries to container ────────────────────────────────────────────────
__info "Copying binaries to container..."
incus file push "binaries/${INCUS_PROJECT_NAME}" "${INCUS_CONTAINER_NAME}/usr/local/bin/"
__vm "chmod +x /usr/local/bin/${INCUS_PROJECT_NAME}"

if [[ -f "binaries/${INCUS_PROJECT_NAME}-cli" ]]; then
    incus file push "binaries/${INCUS_PROJECT_NAME}-cli" "${INCUS_CONTAINER_NAME}/usr/local/bin/"
    __vm "chmod +x /usr/local/bin/${INCUS_PROJECT_NAME}-cli"
fi

# Ensure curl is available for testing
__vm "command -v curl >/dev/null 2>&1 || (apt-get update -qq && apt-get install -y -qq curl)" >/dev/null 2>&1

# ── Version / help / binary info ──────────────────────────────────────────────
__vm "${INCUS_PROJECT_NAME} --version" >/dev/null || __fail "Binary failed to run --version"
__pass "Version check"
__vm "${INCUS_PROJECT_NAME} --help" >/dev/null || __fail "Binary failed to run --help"
__pass "Help check"
__vm "ls -lh /usr/local/bin/${INCUS_PROJECT_NAME} && file /usr/local/bin/${INCUS_PROJECT_NAME}" >/dev/null || \
    __fail "Binary info check failed"
__pass "Binary info"

# ── Service install / start / status ─────────────────────────────────────────
__vm "${INCUS_PROJECT_NAME} --service --install" || __fail "--service --install failed"
__pass "--service --install"

# inside container — not a host-service mutation
__vm "systemctl start ${INCUS_PROJECT_NAME}" || __fail "systemctl start failed"
sleep 3
# inside container — not a host-service mutation
__vm "systemctl is-active ${INCUS_PROJECT_NAME}" | grep -q -- "^active$" || \
    __fail "Service is not active after start"
__pass "Service is active"

INCUS_BASE="http://localhost:${INCUS_PORT}"

# ── Health endpoints ──────────────────────────────────────────────────────────
__check_status "GET /server/healthz"        GET "${INCUS_BASE}/server/healthz"        200
__check_status "GET /api/v1/server/healthz" GET "${INCUS_BASE}/api/v1/server/healthz" 200
__check_body "healthz has version field" "${INCUS_BASE}/api/v1/server/healthz" "version"
__check_body "healthz has status field"  "${INCUS_BASE}/api/v1/server/healthz" "ok\|status\|healthy"

# ── Content negotiation: frontend routes ─────────────────────────────────────
__info "--- Content negotiation: frontend routes ---"
__check_body "/ Accept:text/html -> HTML" "${INCUS_BASE}/" "<!DOCTYPE html\|<html" \
    "--header 'Accept: text/html'"
__check_status "/ Accept:text/plain -> 200" GET "${INCUS_BASE}/" 200 \
    "--header 'Accept: text/plain'"
__check_status "GET /recent -> 200" GET "${INCUS_BASE}/recent" 200
__check_status "GET /create -> 200" GET "${INCUS_BASE}/create" 200

# ── Content negotiation: API routes ───────────────────────────────────────────
__info "--- Content negotiation: API routes ---"
__check_body "GET /api/v1/pastes -> JSON" "${INCUS_BASE}/api/v1/pastes" '"ok"' \
    "--header 'Accept: application/json'"
__check_status "GET /api/v1/pastes -> 200" GET "${INCUS_BASE}/api/v1/pastes" 200
__check_status "GET /api/v1/pastes.txt -> 200" GET "${INCUS_BASE}/api/v1/pastes.txt" 200

# ── Create / retrieve / delete paste ──────────────────────────────────────────
__info "--- Paste creation ---"
INCUS_CREATE_DATA='{\"content\":\"hello from incus test\",\"language\":\"text\"}'
INCUS_RESPONSE=$(__vm "curl -sf --header 'Content-Type: application/json' \
    --header 'Accept: application/json' --data '${INCUS_CREATE_DATA}' \
    ${INCUS_BASE}/api/v1/pastes")
INCUS_PASTE_ID=$(printf '%s' "${INCUS_RESPONSE}" | grep -o -- '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
INCUS_OWNER_TOKEN=$(printf '%s' "${INCUS_RESPONSE}" | grep -o -- '"owner_token":"[^"]*"' | cut -d'"' -f4)
[[ -n "${INCUS_PASTE_ID}" ]] || __fail "Create paste failed: ${INCUS_RESPONSE}"
__pass "Create paste: id=${INCUS_PASTE_ID}"

__info "--- Paste retrieval ---"
INCUS_GET=$(__vm "curl -sf --header 'Accept: application/json' ${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}")
printf '%s' "${INCUS_GET}" | grep -q -- "hello from incus test" || __fail "Get paste (JSON): content mismatch"
__pass "Get paste (JSON)"

INCUS_RAW=$(__vm "curl -sf ${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}/raw")
[[ "${INCUS_RAW}" == "hello from incus test" ]] || __fail "Raw paste: unexpected content: ${INCUS_RAW}"
__pass "Get raw paste"

__info "--- Paste deletion ---"
__check_status "Delete with no token -> 401" DELETE "${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}" 401
if [[ -n "${INCUS_OWNER_TOKEN}" ]]; then
    INCUS_DEL=$(__vm "curl -sf --request DELETE --header 'Authorization: Bearer ${INCUS_OWNER_TOKEN}' ${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}")
    printf '%s' "${INCUS_DEL}" | grep -qi -- '"ok".*true\|deleted' || __fail "Delete paste: unexpected response: ${INCUS_DEL}"
    __pass "Delete paste with owner token"
    __check_status "Deleted paste returns 404" GET "${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}" 404
fi

# ── Compat APIs ────────────────────────────────────────────────────────────────
__info "--- Compat API endpoints ---"
INCUS_COMPAT_RESPONSE=$(__vm "curl -sf --data 'api_paste_code=compat+test&api_paste_private=0' ${INCUS_BASE}/api/api_post.php")
printf '%s' "${INCUS_COMPAT_RESPONSE}" | grep -qi -- "http\|paste\|error" || \
    __fail "pastebin.com compat POST: unexpected response: ${INCUS_COMPAT_RESPONSE}"
__pass "pastebin.com compat POST /api/api_post.php"

__check_status "lenpaste /api/new -> 200/201" POST "${INCUS_BASE}/api/new" 200 \
    "--header 'Content-Type: application/json' --data '{\"text\":\"lenpaste compat test\"}'"
__check_status "lenpaste /api/v1/getServerInfo -> 200" GET "${INCUS_BASE}/api/v1/getServerInfo" 200

# ── Swagger / GraphQL endpoints ───────────────────────────────────────────────
__info "--- API documentation endpoints ---"
__check_status "GET /api/swagger -> 200"           GET "${INCUS_BASE}/api/swagger"           200
__check_status "GET /api/v1/server/swagger -> 200" GET "${INCUS_BASE}/api/v1/server/swagger" 200
__check_status "GET /api/graphql -> 200"           GET "${INCUS_BASE}/api/graphql"           200
__check_status "GET /api/v1/server/graphql -> 200" GET "${INCUS_BASE}/api/v1/server/graphql" 200
__check_status "GET /server/docs/graphql -> 200"   GET "${INCUS_BASE}/server/docs/graphql"   200

# ── Static / SEO endpoints ────────────────────────────────────────────────────
__info "--- Static endpoints ---"
__check_status "GET /robots.txt -> 200"   GET "${INCUS_BASE}/robots.txt"   200
__check_status "GET /security.txt -> 200" GET "${INCUS_BASE}/security.txt" 200

# ── Binary rename tests ───────────────────────────────────────────────────────
__info "--- Binary rename tests ---"
__vm "cp /usr/local/bin/${INCUS_PROJECT_NAME} /tmp/renamed-server && chmod +x /tmp/renamed-server"
if __vm "/tmp/renamed-server --help" 2>&1 | grep -q -- "renamed-server"; then
    __pass "Server binary rename works (--help shows actual name)"
else
    __fail "Server --help does not show renamed binary name"
fi

# ── Client tests (if built) ───────────────────────────────────────────────────
__info "--- Client tests (if exists) ---"
if __vm "test -f /usr/local/bin/${INCUS_PROJECT_NAME}-cli" 2>/dev/null; then
    __vm "${INCUS_PROJECT_NAME}-cli --version" >/dev/null || __fail "CLI --version failed"
    __vm "${INCUS_PROJECT_NAME}-cli --help" >/dev/null || __fail "CLI --help failed"
    __pass "CLI version/help"

    __vm "cp /usr/local/bin/${INCUS_PROJECT_NAME}-cli /tmp/renamed-cli && chmod +x /tmp/renamed-cli"
    if __vm "/tmp/renamed-cli --help" 2>&1 | grep -q -- "renamed-cli"; then
        __pass "CLI binary rename works"
    else
        __fail "CLI --help does not show renamed binary name"
    fi

    __vm "${INCUS_PROJECT_NAME}-cli --server ${INCUS_BASE} status" >/dev/null || \
        __fail "CLI status (no token) failed"
    __pass "CLI status against running server"
else
    __info "client not installed - skipping"
fi

# ── Service stop / uninstall ──────────────────────────────────────────────────
# inside container — not a host-service mutation
__vm "systemctl stop ${INCUS_PROJECT_NAME}" || __fail "systemctl stop failed"
sleep 2
__vm "systemctl is-active ${INCUS_PROJECT_NAME}" | grep -qv -- "^active$" || \
    __fail "Service is still active after stop"
__pass "Service stopped"

__vm "${INCUS_PROJECT_NAME} --service --uninstall" || __fail "--service --uninstall failed"
__pass "--service --uninstall"

# ── Final result ─────────────────────────────────────────────────────────────
if [[ "${INCUS_ERRORS}" -gt 0 ]]; then
    printf "\n[RESULT] %d test(s) FAILED\n" "${INCUS_ERRORS}" >&2
    exit 1
else
    __info "All Incus integration tests passed."
fi
