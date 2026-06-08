#!/usr/bin/env bash
# tests/docker.sh — Docker integration tests for pastebin
# Runs the server in a Docker container and exercises the API with full
# content negotiation and endpoint coverage per PART 28 requirements.
# Usage: bash tests/docker.sh [--keep]
# Options:
#   --keep   Do not remove the container after the test run

set -euo pipefail

# ── Variables ────────────────────────────────────────────────────────────────
SCRIPT_NAME="$(basename -- "$0")"
PROJECT_DIR="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
PROJECT_ORG="apimgr"
PROJECT_NAME="pastebin"
CONTAINER_NAME="${PROJECT_NAME}-test-$$"
IMAGE_NAME="${PROJECT_ORG}/${PROJECT_NAME}:test-$$"
PORT="18080"
BASE_URL="http://127.0.0.1:${PORT}"
KEEP=false
ERRORS=0

# ── Helpers ──────────────────────────────────────────────────────────────────
pass() { printf "[PASS] %s\n" "$*"; }
fail() { printf "[FAIL] %s\n" "$*" >&2; ERRORS=$((ERRORS + 1)); }
info() { printf "[INFO] %s\n" "$*"; }

check_status() {
    local desc="$1" method="$2" url="$3" expected="$4"
    shift 4
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" -X "${method}" "$@" "${url}")
    if [[ "${code}" == "${expected}" ]]; then
        pass "${desc} → ${code}"
    else
        fail "${desc} → expected ${expected}, got ${code}"
    fi
}

check_body() {
    local desc="$1" url="$2" pattern="$3"
    shift 3
    local body
    body=$(curl -sf "$@" "${url}" 2>/dev/null || true)
    if printf '%s' "${body}" | grep -qi "${pattern}"; then
        pass "${desc}"
    else
        fail "${desc} — pattern '${pattern}' not found in: ${body}"
    fi
}

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
    if curl --silent --fail "${BASE_URL}/server/healthz" >/dev/null 2>&1; then
        break
    fi
    sleep 1
    if [[ "$i" -eq 30 ]]; then
        docker logs "${CONTAINER_NAME}" >&2
        fail "Server did not start within 30 seconds"
        exit 1
    fi
done
pass "Server is up"

# ── Health endpoints ──────────────────────────────────────────────────────────
check_status "GET /server/healthz"       GET "${BASE_URL}/server/healthz"       200
check_status "GET /api/v1/server/healthz" GET "${BASE_URL}/api/v1/server/healthz" 200

# Healthz must include required fields
check_body "healthz has version field" "${BASE_URL}/api/v1/server/healthz" "version"
check_body "healthz has status field"  "${BASE_URL}/api/v1/server/healthz" "ok\|status\|healthy"

# ── Content negotiation: frontend routes ─────────────────────────────────────
info "--- Content negotiation: frontend routes ---"
# Home page — browser gets HTML, plain-text client gets text
check_body "/ Accept:text/html → HTML"  "${BASE_URL}/" "<!DOCTYPE html\|<html" \
    --header "Accept: text/html"
check_status "/ Accept:text/plain → 200" GET "${BASE_URL}/" 200 \
    --header "Accept: text/plain"
check_status "GET /recent → 200"         GET "${BASE_URL}/recent" 200
check_status "GET /create → 200"         GET "${BASE_URL}/create" 200

# ── Content negotiation: API routes ─────────────────────────────────────────
info "--- Content negotiation: API routes ---"
check_body   "GET /api/v1/pastes → JSON"  "${BASE_URL}/api/v1/pastes" '"ok"' \
    --header "Accept: application/json"
check_status "GET /api/v1/pastes → 200"   GET "${BASE_URL}/api/v1/pastes" 200

# ── Create paste: multiple request formats ───────────────────────────────────
info "--- Paste creation ---"

# JSON body
RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"hello from docker test","language":"text"}' \
    "${BASE_URL}/api/v1/paste") || fail "Create paste (JSON): request failed"
PASTE_ID=$(printf '%s' "${RESPONSE}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
OWNER_TOKEN=$(printf '%s' "${RESPONSE}" | grep -o '"owner_token":"[^"]*"' | cut -d'"' -f4)
[[ -n "${PASTE_ID}" ]] || { fail "Create paste (JSON): no ID in response: ${RESPONSE}"; }
pass "Create paste (JSON): id=${PASTE_ID}"

# Raw body (curl pipe style)
RAW_RESPONSE=$(curl -sf \
    --header "Content-Type: text/plain" \
    --header "Accept: application/json" \
    --data "raw paste from curl" \
    "${BASE_URL}/api/v1/paste") || fail "Create paste (raw): request failed"
printf '%s' "${RAW_RESPONSE}" | grep -q '"id"' || \
    fail "Create paste (raw): no id in response: ${RAW_RESPONSE}"
pass "Create paste (raw body)"

# Form submission (browser style — should redirect)
FORM_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    --header "Content-Type: application/x-www-form-urlencoded" \
    --data "content=form+paste+test" \
    "${BASE_URL}/paste")
[[ "${FORM_CODE}" =~ ^(201|303|302)$ ]] || \
    fail "Create paste (form): expected 201/302/303, got ${FORM_CODE}"
pass "Create paste (form): code=${FORM_CODE}"

# With expiry
EXP_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"expiring paste","expires_in":"1h"}' \
    "${BASE_URL}/api/v1/paste") || fail "Create paste (expiry): request failed"
printf '%s' "${EXP_RESPONSE}" | grep -q '"expires_at"' || \
    fail "Create paste (expiry): expires_at missing in: ${EXP_RESPONSE}"
pass "Create paste (with expiry)"

# With burn_after
BURN_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"burn after 2","burn_after":2}' \
    "${BASE_URL}/api/v1/paste") || fail "Create paste (burn_after): request failed"
printf '%s' "${BURN_RESPONSE}" | grep -q '"burn_after"' || \
    fail "Create paste (burn_after): field missing in: ${BURN_RESPONSE}"
pass "Create paste (burn_after=2)"

# Unlisted
UNLIST_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"unlisted paste","visibility":"unlisted"}' \
    "${BASE_URL}/api/v1/paste") || fail "Create paste (unlisted): request failed"
printf '%s' "${UNLIST_RESPONSE}" | grep -q '"visibility"' || \
    fail "Create paste (unlisted): field missing"
pass "Create paste (unlisted)"

# ── Retrieve paste ───────────────────────────────────────────────────────────
info "--- Paste retrieval ---"
GET=$(curl -sf \
    --header "Accept: application/json" \
    "${BASE_URL}/api/v1/paste/${PASTE_ID}") || fail "Get paste: request failed"
printf '%s' "${GET}" | grep -q "hello from docker test" || \
    fail "Get paste: content mismatch: ${GET}"
pass "Get paste (JSON)"

# Raw text
RAW=$(curl -sf "${BASE_URL}/api/v1/paste/${PASTE_ID}/raw") || fail "Get raw paste: request failed"
[[ "${RAW}" == "hello from docker test" ]] || \
    fail "Get raw paste: unexpected content: ${RAW}"
pass "Get raw paste"

# List pastes
LIST=$(curl -sf --header "Accept: application/json" "${BASE_URL}/api/v1/pastes") || \
    fail "List pastes: request failed"
printf '%s' "${LIST}" | grep -q "${PASTE_ID}" || \
    fail "List pastes: paste ID not found"
pass "List pastes"

# ── Delete paste ─────────────────────────────────────────────────────────────
info "--- Paste deletion ---"

# Wrong token → 404
check_status "Delete with wrong token → 404" DELETE \
    "${BASE_URL}/api/v1/paste/${PASTE_ID}" 404 \
    --header "Authorization: Bearer tok_wrongwrongwrongwrongwrongwrongwx"

# No token → 401
check_status "Delete with no token → 401" DELETE \
    "${BASE_URL}/api/v1/paste/${PASTE_ID}" 401

# Correct owner token → 200
if [[ -n "${OWNER_TOKEN}" ]]; then
    DEL=$(curl -sf \
        --request DELETE \
        --header "Authorization: Bearer ${OWNER_TOKEN}" \
        "${BASE_URL}/api/v1/paste/${PASTE_ID}") || fail "Delete paste: request failed"
    printf '%s' "${DEL}" | grep -qi '"ok".*true\|deleted' || \
        fail "Delete paste: unexpected response: ${DEL}"
    pass "Delete paste (owner token)"

    # Verify it's gone
    check_status "Deleted paste returns 404" GET \
        "${BASE_URL}/api/v1/paste/${PASTE_ID}" 404
fi

# ── Error cases ──────────────────────────────────────────────────────────────
info "--- Error cases ---"
check_status "Get nonexistent paste → 404" GET "${BASE_URL}/api/v1/paste/notexist" 404
check_status "Empty content → 400" POST "${BASE_URL}/api/v1/paste" 400 \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":""}'

# ── Compat APIs ───────────────────────────────────────────────────────────────
info "--- Compat API endpoints ---"
# pastebin.com compat
COMPAT_RESPONSE=$(curl -sf \
    --data "api_paste_code=compat+test&api_paste_private=0" \
    "${BASE_URL}/api/api_post.php") || fail "pastebin.com compat POST: request failed"
printf '%s' "${COMPAT_RESPONSE}" | grep -qi "http\|paste\|error" || \
    fail "pastebin.com compat POST: unexpected response: ${COMPAT_RESPONSE}"
pass "pastebin.com compat POST /api/api_post.php"

check_status "pastebin.com login stub → 200" POST "${BASE_URL}/api/api_login.php" 200 \
    --data "api_dev_key=x&api_user_name=x&api_user_password=x"

# lenpaste compat
check_status "lenpaste /api/new → 200/201" POST "${BASE_URL}/api/new" 200 \
    --header "Content-Type: application/json" \
    --data '{"text":"lenpaste compat test"}'

check_status "lenpaste /api/getServerInfo → 200" GET "${BASE_URL}/api/getServerInfo" 200

# ── Swagger / GraphQL endpoints ───────────────────────────────────────────────
info "--- API documentation endpoints ---"
check_status "GET /api/swagger.json → 200"              GET "${BASE_URL}/api/swagger.json"        200
check_status "GET /api/v1/server/swagger → 200"         GET "${BASE_URL}/api/v1/server/swagger"   200
check_status "GET /graphql → 200"                       GET "${BASE_URL}/graphql"                 200
check_status "GET /api/v1/server/graphql → 200"         GET "${BASE_URL}/api/v1/server/graphql"   200

# ── Static / SEO endpoints ───────────────────────────────────────────────────
info "--- Static endpoints ---"
check_status "GET /robots.txt → 200"                    GET "${BASE_URL}/robots.txt"               200
check_status "GET /.well-known/security.txt → 200"      GET "${BASE_URL}/.well-known/security.txt" 200

# ── Security headers ─────────────────────────────────────────────────────────
info "--- Security headers ---"
HEADERS=$(curl -sI "${BASE_URL}/" | tr '[:upper:]' '[:lower:]')
printf '%s' "${HEADERS}" | grep -q "x-content-type-options" || \
    fail "Missing X-Content-Type-Options header"
pass "X-Content-Type-Options present"
printf '%s' "${HEADERS}" | grep -q "x-frame-options\|content-security-policy" || \
    fail "Missing X-Frame-Options or Content-Security-Policy header"
pass "Framing protection header present"

# ── --status flag ─────────────────────────────────────────────────────────────
docker exec "${CONTAINER_NAME}" /usr/local/bin/pastebin --status >/dev/null || \
    fail "--status exited non-zero"
pass "--status flag: ok"

# ── Final result ─────────────────────────────────────────────────────────────
if [[ "${ERRORS}" -gt 0 ]]; then
    printf "\n[RESULT] %d test(s) FAILED\n" "${ERRORS}" >&2
    exit 1
else
    info "All Docker integration tests passed."
fi
