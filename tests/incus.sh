#!/usr/bin/env bash
# @@License : WTFPL
# tests/incus.sh — Incus VM integration tests for pastebin
# Launches an Incus VM, copies the binary, and exercises the full systemd
# service workflow including install/start/stop/status/uninstall.
# Usage: bash tests/incus.sh [--keep] [--image IMAGE]
# Options:
#   --keep         Do not delete the VM after the test run
#   --image IMAGE  Incus base image (default: images:ubuntu/24.04)

set -euo pipefail

# ── Variables ────────────────────────────────────────────────────────────────
SCRIPT_NAME="$(basename -- "$0")"
PROJECT_DIR="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
PROJECT_ORG="apimgr"
PROJECT_NAME="pastebin"
VM_NAME="${PROJECT_NAME}-test-$$"
INCUS_IMAGE="images:ubuntu/24.04"
KEEP=false

# ── Helpers ──────────────────────────────────────────────────────────────────
pass() { printf "[PASS] %s\n" "$*"; }
fail() { printf "[FAIL] %s\n" "$*" >&2; exit 1; }
info() { printf "[INFO] %s\n" "$*"; }
vm()   { incus exec "${VM_NAME}" -- bash -c "$*"; }

cleanup() {
    if [[ "$KEEP" == "false" ]]; then
        info "Deleting VM ${VM_NAME}"
        incus delete --force "${VM_NAME}" 2>/dev/null || true
    else
        info "Keeping VM ${VM_NAME} (--keep flag set)"
    fi
    if [[ -n "${TMPDIR_BUILD:-}" && -d "${TMPDIR_BUILD}" ]]; then
        rm -rf "${TMPDIR_BUILD}"
    fi
}
trap cleanup EXIT

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep)         KEEP=true; shift ;;
        --image)        INCUS_IMAGE="$2"; shift 2 ;;
        *) printf "%s: unknown argument: %s\n" "${SCRIPT_NAME}" "$1" >&2; exit 2 ;;
    esac
done

# ── Prerequisite checks ───────────────────────────────────────────────────────
command -v incus >/dev/null 2>&1 || fail "incus not found in PATH"

# ── Build binary for linux/amd64 ─────────────────────────────────────────────
info "Building pastebin binary (linux/amd64) using casjaysdev/go:latest"
mkdir -p "${TMPDIR:-/tmp}/${PROJECT_ORG}"
TMPDIR_BUILD="$(mktemp -d "${TMPDIR:-/tmp}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX")"
docker run --rm \
    --volume "${PROJECT_DIR}":/app \
    --volume "${TMPDIR_BUILD}":/out \
    --workdir /app \
    --env CGO_ENABLED=0 \
    casjaysdev/go:latest \
    sh -c "GOOS=linux GOARCH=amd64 go build -o /out/pastebin ./src"
BINARY="${TMPDIR_BUILD}/pastebin"
[[ -f "${BINARY}" ]] || fail "Binary not found after build"
pass "Binary built: ${BINARY}"

# ── Launch VM ────────────────────────────────────────────────────────────────
info "Launching VM ${VM_NAME} from ${INCUS_IMAGE}"
incus launch "${INCUS_IMAGE}" "${VM_NAME}" --vm

# Wait for cloud-init / network
info "Waiting for VM to boot..."
for i in $(seq 1 60); do
    if incus exec "${VM_NAME}" -- bash -c "systemctl is-active network-online.target" 2>/dev/null | grep -q active; then
        break
    fi
    sleep 2
    [[ "$i" -lt 60 ]] || fail "VM did not come up in 120 seconds"
done
pass "VM booted"

# ── Copy binary and test ──────────────────────────────────────────────────────
info "Copying binary to VM"
incus file push "${BINARY}" "${VM_NAME}/usr/local/bin/pastebin"
vm "chmod 755 /usr/local/bin/pastebin"

# Verify binary runs
vm "/usr/local/bin/pastebin --version" || fail "Binary failed to run"
pass "Binary executes"

# Install as system service
vm "/usr/local/bin/pastebin --service install" || fail "--service install failed"
pass "--service install"

# Start service
vm "systemctl start pastebin" || fail "systemctl start pastebin failed"
sleep 3

# Check service status
vm "systemctl is-active pastebin" | grep -q "^active$" || \
    fail "Service is not active after start"
pass "Service is active"

# Get the port the server is listening on
SVC_PORT=$(vm "/usr/local/bin/pastebin --status 2>/dev/null" | grep -i "^Port:" | awk '{print $2}' || echo "64580")
[[ -n "${SVC_PORT}" ]] || SVC_PORT="64580"
info "Server port: ${SVC_PORT}"
BASE="http://127.0.0.1:${SVC_PORT}"

# ── API smoke tests ───────────────────────────────────────────────────────────

# Health check via standard path
HEALTH=$(vm "curl --silent --fail ${BASE}/server/healthz")
printf '%s' "${HEALTH}" | grep -qi "ok\|true\|healthy" || \
    fail "Health check: unexpected response"
pass "Health check /server/healthz"

# Create a paste
RESPONSE=$(vm "curl --silent --fail ${BASE}/api/v1/paste \
    --header 'Content-Type: application/json' \
    --data '{\"content\":\"hello from incus test\",\"language\":\"text\"}'")
PASTE_ID=$(printf '%s' "${RESPONSE}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
[[ -n "${PASTE_ID}" ]] || fail "Create paste failed: ${RESPONSE}"
OWNER_TOKEN=$(printf '%s' "${RESPONSE}" | grep -o '"owner_token":"[^"]*"' | cut -d'"' -f4)
pass "Create paste: id=${PASTE_ID}"

# Retrieve paste (JSON)
GET=$(vm "curl --silent --fail --header 'Accept: application/json' ${BASE}/api/v1/paste/${PASTE_ID}")
printf '%s' "${GET}" | grep -q "hello from incus test" || \
    fail "Get paste (JSON): content mismatch"
pass "Get paste (JSON)"

# Get raw text
RAW=$(vm "curl --silent --fail ${BASE}/api/v1/paste/${PASTE_ID}/raw")
[[ "${RAW}" == "hello from incus test" ]] || \
    fail "Raw paste: unexpected content: ${RAW}"
pass "Get raw paste"

# Delete paste with owner token
if [[ -n "${OWNER_TOKEN}" ]]; then
    DEL=$(vm "curl --silent --fail --request DELETE \
        --header 'Authorization: Bearer ${OWNER_TOKEN}' \
        ${BASE}/api/v1/paste/${PASTE_ID}")
    printf '%s' "${DEL}" | grep -qi '"ok".*true\|deleted' || \
        fail "Delete paste: unexpected response: ${DEL}"
    pass "Delete paste with owner token"
fi

# Stop service
vm "systemctl stop pastebin" || fail "systemctl stop pastebin failed"
sleep 2
vm "systemctl is-active pastebin" | grep -qv "^active$" || \
    fail "Service is still active after stop"
pass "Service stopped"

# Uninstall service
vm "/usr/local/bin/pastebin --service uninstall" || fail "--service uninstall failed"
pass "--service uninstall"

info "All Incus VM tests passed."
