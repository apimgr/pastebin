#!/usr/bin/env bash
# @@License : MIT
# tests/incus.sh — Incus VM integration tests for pastebin
# Launches an Incus VM, copies the binary, and exercises the full systemd
# service workflow including install/start/stop/status/uninstall.
# Usage: bash tests/incus.sh [--keep] [--image IMAGE]
# Options:
#   --keep         Do not delete the VM after the test run
#   --image IMAGE  Incus base image (default: images:ubuntu/24.04)

set -euo pipefail

# ── Variables ────────────────────────────────────────────────────────────────
INCUS_SCRIPT_NAME="$(basename -- "$0")"
INCUS_PROJECT_DIR="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
INCUS_PROJECT_ORG="apimgr"
INCUS_PROJECT_NAME="pastebin"
INCUS_VM_NAME="${INCUS_PROJECT_NAME}-test-$$"
INCUS_IMAGE="images:ubuntu/24.04"
INCUS_KEEP=false
INCUS_TMPDIR_BUILD=""

# ── Helpers ──────────────────────────────────────────────────────────────────
__pass() { printf "[PASS] %s\n" "$*"; }
__fail() { printf "[FAIL] %s\n" "$*" >&2; exit 1; }
__info() { printf "[INFO] %s\n" "$*"; }
__vm()   { incus exec "${INCUS_VM_NAME}" -- bash -c "$*"; }

__cleanup() {
    if [[ "${INCUS_KEEP}" == "false" ]]; then
        __info "Deleting VM ${INCUS_VM_NAME}"
        incus delete --force "${INCUS_VM_NAME}" 2>/dev/null || true
    else
        __info "Keeping VM ${INCUS_VM_NAME} (--keep flag set)"
    fi
    if [[ -n "${INCUS_TMPDIR_BUILD}" && -d "${INCUS_TMPDIR_BUILD}" ]]; then
        rm -rf "${INCUS_TMPDIR_BUILD}"
    fi
}
trap __cleanup EXIT

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep)         INCUS_KEEP=true; shift ;;
        --image)        INCUS_IMAGE="$2"; shift 2 ;;
        *) printf "%s: unknown argument: %s\n" "${INCUS_SCRIPT_NAME}" "$1" >&2; exit 2 ;;
    esac
done

# ── Prerequisite checks ───────────────────────────────────────────────────────
command -v incus >/dev/null 2>&1 || __fail "incus not found in PATH"

# ── Build binary for linux/amd64 ─────────────────────────────────────────────
__info "Building pastebin binary (linux/amd64) using casjaysdev/go:latest"
mkdir -p "${TMPDIR:-/tmp}/${INCUS_PROJECT_ORG}"
INCUS_TMPDIR_BUILD="$(mktemp -d "${TMPDIR:-/tmp}/${INCUS_PROJECT_ORG}/${INCUS_PROJECT_NAME}-XXXXXX")"
docker run --rm \
    --volume "${INCUS_PROJECT_DIR}":/app \
    --volume "${INCUS_TMPDIR_BUILD}":/out \
    --workdir /app \
    --env CGO_ENABLED=0 \
    --env GOFLAGS=-buildvcs=false \
    casjaysdev/go:latest \
    sh -c "GOOS=linux GOARCH=amd64 go build -o /out/pastebin ./src"
INCUS_BINARY="${INCUS_TMPDIR_BUILD}/pastebin"
[[ -f "${INCUS_BINARY}" ]] || __fail "Binary not found after build"
__pass "Binary built: ${INCUS_BINARY}"

# ── Launch VM ────────────────────────────────────────────────────────────────
__info "Launching VM ${INCUS_VM_NAME} from ${INCUS_IMAGE}"
incus launch "${INCUS_IMAGE}" "${INCUS_VM_NAME}" --vm

# Wait for cloud-init / network
__info "Waiting for VM to boot..."
for ((i = 1; i <= 60; i++)); do
    if incus exec "${INCUS_VM_NAME}" -- bash -c "systemctl is-active network-online.target" 2>/dev/null | grep -q -- active; then
        break
    fi
    sleep 2
    [[ "${i}" -lt 60 ]] || __fail "VM did not come up in 120 seconds"
done
__pass "VM booted"

# ── Copy binary and test ──────────────────────────────────────────────────────
__info "Copying binary to VM"
incus file push "${INCUS_BINARY}" "${INCUS_VM_NAME}/usr/local/bin/pastebin"
__vm "chmod 755 /usr/local/bin/pastebin"

# Verify binary runs
__vm "/usr/local/bin/pastebin --version" || __fail "Binary failed to run"
__pass "Binary executes"

# Install as system service
__vm "/usr/local/bin/pastebin --service install" || __fail "--service install failed"
__pass "--service install"

# Start service
__vm "systemctl start pastebin" || __fail "systemctl start pastebin failed"
sleep 3

# Check service status
__vm "systemctl is-active pastebin" | grep -q -- "^active$" || \
    __fail "Service is not active after start"
__pass "Service is active"

# Get the port the server is listening on
INCUS_SVC_PORT=$(__vm "/usr/local/bin/pastebin --status 2>/dev/null" | grep -i -- "^Port:" | awk '{print $2}' || echo "64580")
[[ -n "${INCUS_SVC_PORT}" ]] || INCUS_SVC_PORT="64580"
__info "Server port: ${INCUS_SVC_PORT}"
INCUS_BASE="http://127.0.0.1:${INCUS_SVC_PORT}"

# ── API smoke tests ───────────────────────────────────────────────────────────

# Health check via standard path
INCUS_HEALTH=$(__vm "curl --silent --fail ${INCUS_BASE}/server/healthz")
printf '%s' "${INCUS_HEALTH}" | grep -qi -- "ok\|true\|healthy" || \
    __fail "Health check: unexpected response"
__pass "Health check /server/healthz"

# Create a paste
INCUS_RESPONSE=$(__vm "curl --silent --fail ${INCUS_BASE}/api/v1/pastes \
    --header 'Content-Type: application/json' \
    --data '{\"content\":\"hello from incus test\",\"language\":\"text\"}'")
INCUS_PASTE_ID=$(printf '%s' "${INCUS_RESPONSE}" | grep -o -- '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
[[ -n "${INCUS_PASTE_ID}" ]] || __fail "Create paste failed: ${INCUS_RESPONSE}"
INCUS_OWNER_TOKEN=$(printf '%s' "${INCUS_RESPONSE}" | grep -o -- '"owner_token":"[^"]*"' | cut -d'"' -f4)
__pass "Create paste: id=${INCUS_PASTE_ID}"

# Retrieve paste (JSON)
INCUS_GET=$(__vm "curl --silent --fail --header 'Accept: application/json' ${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}")
printf '%s' "${INCUS_GET}" | grep -q -- "hello from incus test" || \
    __fail "Get paste (JSON): content mismatch"
__pass "Get paste (JSON)"

# Get raw text
INCUS_RAW=$(__vm "curl --silent --fail ${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}/raw")
[[ "${INCUS_RAW}" == "hello from incus test" ]] || \
    __fail "Raw paste: unexpected content: ${INCUS_RAW}"
__pass "Get raw paste"

# Delete paste with owner token
if [[ -n "${INCUS_OWNER_TOKEN}" ]]; then
    INCUS_DEL=$(__vm "curl --silent --fail --request DELETE \
        --header 'Authorization: Bearer ${INCUS_OWNER_TOKEN}' \
        ${INCUS_BASE}/api/v1/pastes/${INCUS_PASTE_ID}")
    printf '%s' "${INCUS_DEL}" | grep -qi -- '"ok".*true\|deleted' || \
        __fail "Delete paste: unexpected response: ${INCUS_DEL}"
    __pass "Delete paste with owner token"
fi

# Stop service
__vm "systemctl stop pastebin" || __fail "systemctl stop pastebin failed"
sleep 2
__vm "systemctl is-active pastebin" | grep -qv -- "^active$" || \
    __fail "Service is still active after stop"
__pass "Service stopped"

# Uninstall service
__vm "/usr/local/bin/pastebin --service uninstall" || __fail "--service uninstall failed"
__pass "--service uninstall"

__info "All Incus VM tests passed."
