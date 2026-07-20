#!/usr/bin/env bash
# @@License : WTFPL
# tests/docker.sh — Docker integration tests for pastebin
# Builds the local image, brings it up via the mandated
# docker/docker-compose.test.yml (copied to a temp dir, never run from the
# project tree), and exercises the API with content negotiation and endpoint
# coverage per PART 28 requirements.
# Usage: bash tests/docker.sh [--keep]
# Options:
#   --keep   Do not tear down the stack / temp dir after the test run

set -euo pipefail

# ── Variables ────────────────────────────────────────────────────────────────
DOCKER_SCRIPT_NAME="$(basename -- "$0")"
DOCKER_PROJECT_DIR="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
DOCKER_PROJECT_ORG="apimgr"
DOCKER_PROJECT_NAME="pastebin"
# Local build tag — the temp compose file is rewritten to use this image with
# pull_policy:never so we exercise the current source, not a published image.
DOCKER_IMAGE_NAME="${DOCKER_PROJECT_NAME}:test-$$"
DOCKER_COMPOSE_PROJECT="${DOCKER_PROJECT_NAME}-test-$$"
# Fixed by docker/docker-compose.test.yml (Test context → 64581:80).
DOCKER_CONTAINER_NAME="${DOCKER_PROJECT_NAME}-test"
DOCKER_PORT="64581"
DOCKER_BASE_URL="http://127.0.0.1:${DOCKER_PORT}"
DOCKER_TEMP_DIR=""
DOCKER_KEEP=false
DOCKER_ERRORS=0

# ── Helpers ──────────────────────────────────────────────────────────────────
__pass() { printf "[PASS] %s\n" "$*"; }
__fail() { printf "[FAIL] %s\n" "$*" >&2; DOCKER_ERRORS=$((DOCKER_ERRORS + 1)); }
__info() { printf "[INFO] %s\n" "$*"; }

__check_status() {
    local desc="$1" method="$2" url="$3" expected="$4"
    shift 4
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" -X "${method}" "$@" "${url}")
    if [[ "${code}" == "${expected}" ]]; then
        __pass "${desc} → ${code}"
    else
        __fail "${desc} → expected ${expected}, got ${code}"
    fi
}

__check_body() {
    local desc="$1" url="$2" pattern="$3"
    shift 3
    local body
    body=$(curl -sf "$@" "${url}" 2>/dev/null || true)
    if printf '%s' "${body}" | grep -qi -- "${pattern}"; then
        __pass "${desc}"
    else
        __fail "${desc} — pattern '${pattern}' not found in: ${body}"
    fi
}

__cleanup() {
    if [[ "${DOCKER_KEEP}" == "false" ]]; then
        if [[ -n "${DOCKER_TEMP_DIR}" && -f "${DOCKER_TEMP_DIR}/docker-compose.yml" ]]; then
            __info "Tearing down compose project ${DOCKER_COMPOSE_PROJECT}"
            (cd "${DOCKER_TEMP_DIR}" && docker compose -p "${DOCKER_COMPOSE_PROJECT}" down -v 2>/dev/null) || true
        fi
        docker rmi -f "${DOCKER_IMAGE_NAME}" 2>/dev/null || true
        [[ -n "${DOCKER_TEMP_DIR}" && -d "${DOCKER_TEMP_DIR}" ]] && rm -rf "${DOCKER_TEMP_DIR}"
    else
        __info "Keeping compose project ${DOCKER_COMPOSE_PROJECT} and temp dir ${DOCKER_TEMP_DIR} (--keep flag set)"
    fi
}
trap __cleanup EXIT

# ── Argument parsing ─────────────────────────────────────────────────────────
for arg in "$@"; do
    case "${arg}" in
        --keep) DOCKER_KEEP=true ;;
        *) printf "%s: unknown argument: %s\n" "${DOCKER_SCRIPT_NAME}" "${arg}" >&2; exit 2 ;;
    esac
done

# ── Build test image ─────────────────────────────────────────────────────────
__info "Building test image ${DOCKER_IMAGE_NAME}"
docker build \
    --file "${DOCKER_PROJECT_DIR}/docker/Dockerfile" \
    --tag "${DOCKER_IMAGE_NAME}" \
    "${DOCKER_PROJECT_DIR}"

# ── Prepare compose stack in a temp dir (PART 28: never run from project tree) ─
mkdir -p "${TMPDIR:-/tmp}/${DOCKER_PROJECT_ORG}"
DOCKER_TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/${DOCKER_PROJECT_ORG}/${DOCKER_PROJECT_NAME}-XXXXXX")"
mkdir -p "${DOCKER_TEMP_DIR}/volumes/config" "${DOCKER_TEMP_DIR}/volumes/data"
cp "${DOCKER_PROJECT_DIR}/docker/docker-compose.test.yml" "${DOCKER_TEMP_DIR}/docker-compose.yml"
# Point the stack at the locally built image and never pull a published one.
sed -i \
    -e "s|image: ghcr.io/apimgr/pastebin:latest|image: ${DOCKER_IMAGE_NAME}|" \
    -e "s|pull_policy: always|pull_policy: never|" \
    "${DOCKER_TEMP_DIR}/docker-compose.yml"

# ── Start stack ──────────────────────────────────────────────────────────────
__info "Starting compose project ${DOCKER_COMPOSE_PROJECT} on port ${DOCKER_PORT}"
(cd "${DOCKER_TEMP_DIR}" && docker compose -p "${DOCKER_COMPOSE_PROJECT}" up --detach)

# Wait for server to become ready (up to 30 s)
__info "Waiting for server to start..."
for ((i = 1; i <= 30; i++)); do
    if curl --silent --fail "${DOCKER_BASE_URL}/server/healthz" >/dev/null 2>&1; then
        break
    fi
    sleep 1
    if [[ "${i}" -eq 30 ]]; then
        (cd "${DOCKER_TEMP_DIR}" && docker compose -p "${DOCKER_COMPOSE_PROJECT}" logs) >&2 || true
        __fail "Server did not start within 30 seconds"
        exit 1
    fi
done
__pass "Server is up"

# ── Health endpoints ──────────────────────────────────────────────────────────
__check_status "GET /server/healthz"        GET "${DOCKER_BASE_URL}/server/healthz"        200
__check_status "GET /api/v1/server/healthz" GET "${DOCKER_BASE_URL}/api/v1/server/healthz" 200

# Healthz must include required fields
__check_body "healthz has version field" "${DOCKER_BASE_URL}/api/v1/server/healthz" "version"
__check_body "healthz has status field"  "${DOCKER_BASE_URL}/api/v1/server/healthz" "ok\|status\|healthy"

# ── Content negotiation: frontend routes ─────────────────────────────────────
__info "--- Content negotiation: frontend routes ---"
# Home page — browser gets HTML, plain-text client gets text
__check_body "/ Accept:text/html → HTML"  "${DOCKER_BASE_URL}/" "<!DOCTYPE html\|<html" \
    --header "Accept: text/html"
__check_status "/ Accept:text/plain → 200" GET "${DOCKER_BASE_URL}/" 200 \
    --header "Accept: text/plain"
__check_status "GET /recent → 200"         GET "${DOCKER_BASE_URL}/recent" 200
__check_status "GET /create → 200"         GET "${DOCKER_BASE_URL}/create" 200

# ── Content negotiation: API routes ─────────────────────────────────────────
__info "--- Content negotiation: API routes ---"
__check_body   "GET /api/v1/pastes → JSON"  "${DOCKER_BASE_URL}/api/v1/pastes" '"ok"' \
    --header "Accept: application/json"
__check_status "GET /api/v1/pastes → 200"   GET "${DOCKER_BASE_URL}/api/v1/pastes" 200
# Plain-text negotiation on the same route
__check_status "GET /api/v1/pastes Accept:text/plain → 200" GET "${DOCKER_BASE_URL}/api/v1/pastes" 200 \
    --header "Accept: text/plain"
# .txt extension alias
__check_status "GET /api/v1/pastes.txt → 200" GET "${DOCKER_BASE_URL}/api/v1/pastes.txt" 200

# ── Create paste: multiple request formats ───────────────────────────────────
__info "--- Paste creation ---"

# JSON body
DOCKER_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"hello from docker test","language":"text"}' \
    "${DOCKER_BASE_URL}/api/v1/pastes") || __fail "Create paste (JSON): request failed"
DOCKER_PASTE_ID=$(printf '%s' "${DOCKER_RESPONSE}" | grep -o -- '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
DOCKER_OWNER_TOKEN=$(printf '%s' "${DOCKER_RESPONSE}" | grep -o -- '"owner_token":"[^"]*"' | cut -d'"' -f4)
[[ -n "${DOCKER_PASTE_ID}" ]] || __fail "Create paste (JSON): no ID in response: ${DOCKER_RESPONSE}"
__pass "Create paste (JSON): id=${DOCKER_PASTE_ID}"

# Raw body (curl pipe style)
DOCKER_RAW_RESPONSE=$(curl -sf \
    --header "Content-Type: text/plain" \
    --header "Accept: application/json" \
    --data "raw paste from curl" \
    "${DOCKER_BASE_URL}/api/v1/pastes") || __fail "Create paste (raw): request failed"
printf '%s' "${DOCKER_RAW_RESPONSE}" | grep -q -- '"id"' || \
    __fail "Create paste (raw): no id in response: ${DOCKER_RAW_RESPONSE}"
__pass "Create paste (raw body)"

# Form submission (browser style — should redirect)
DOCKER_FORM_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    --header "Content-Type: application/x-www-form-urlencoded" \
    --data "content=form+paste+test" \
    "${DOCKER_BASE_URL}/pastes")
[[ "${DOCKER_FORM_CODE}" =~ ^(201|303|302)$ ]] || \
    __fail "Create paste (form): expected 201/302/303, got ${DOCKER_FORM_CODE}"
__pass "Create paste (form): code=${DOCKER_FORM_CODE}"

# With expiry
DOCKER_EXP_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"expiring paste","expires_in":"1h"}' \
    "${DOCKER_BASE_URL}/api/v1/pastes") || __fail "Create paste (expiry): request failed"
printf '%s' "${DOCKER_EXP_RESPONSE}" | grep -q -- '"expires_at"' || \
    __fail "Create paste (expiry): expires_at missing in: ${DOCKER_EXP_RESPONSE}"
__pass "Create paste (with expiry)"

# With burn_after
DOCKER_BURN_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"burn after 2","burn_after":2}' \
    "${DOCKER_BASE_URL}/api/v1/pastes") || __fail "Create paste (burn_after): request failed"
printf '%s' "${DOCKER_BURN_RESPONSE}" | grep -q -- '"burn_after"' || \
    __fail "Create paste (burn_after): field missing in: ${DOCKER_BURN_RESPONSE}"
__pass "Create paste (burn_after=2)"

# Unlisted
DOCKER_UNLIST_RESPONSE=$(curl -sf \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":"unlisted paste","visibility":"unlisted"}' \
    "${DOCKER_BASE_URL}/api/v1/pastes") || __fail "Create paste (unlisted): request failed"
printf '%s' "${DOCKER_UNLIST_RESPONSE}" | grep -q -- '"visibility"' || \
    __fail "Create paste (unlisted): field missing"
__pass "Create paste (unlisted)"

# ── Retrieve paste ───────────────────────────────────────────────────────────
__info "--- Paste retrieval ---"
DOCKER_GET=$(curl -sf \
    --header "Accept: application/json" \
    "${DOCKER_BASE_URL}/api/v1/pastes/${DOCKER_PASTE_ID}") || __fail "Get paste: request failed"
printf '%s' "${DOCKER_GET}" | grep -q -- "hello from docker test" || \
    __fail "Get paste: content mismatch: ${DOCKER_GET}"
__pass "Get paste (JSON)"

# Raw text
DOCKER_RAW=$(curl -sf "${DOCKER_BASE_URL}/api/v1/pastes/${DOCKER_PASTE_ID}/raw") || __fail "Get raw paste: request failed"
[[ "${DOCKER_RAW}" == "hello from docker test" ]] || \
    __fail "Get raw paste: unexpected content: ${DOCKER_RAW}"
__pass "Get raw paste"

# List pastes
DOCKER_LIST=$(curl -sf --header "Accept: application/json" "${DOCKER_BASE_URL}/api/v1/pastes") || \
    __fail "List pastes: request failed"
printf '%s' "${DOCKER_LIST}" | grep -q -- "${DOCKER_PASTE_ID}" || \
    __fail "List pastes: paste ID not found"
__pass "List pastes"

# ── Delete paste ─────────────────────────────────────────────────────────────
__info "--- Paste deletion ---"

# Wrong token → 404
__check_status "Delete with wrong token → 404" DELETE \
    "${DOCKER_BASE_URL}/api/v1/pastes/${DOCKER_PASTE_ID}" 404 \
    --header "Authorization: Bearer tok_wrongwrongwrongwrongwrongwrongwx"

# No token → 401
__check_status "Delete with no token → 401" DELETE \
    "${DOCKER_BASE_URL}/api/v1/pastes/${DOCKER_PASTE_ID}" 401

# Correct owner token → 200
if [[ -n "${DOCKER_OWNER_TOKEN}" ]]; then
    DOCKER_DEL=$(curl -sf \
        --request DELETE \
        --header "Authorization: Bearer ${DOCKER_OWNER_TOKEN}" \
        "${DOCKER_BASE_URL}/api/v1/pastes/${DOCKER_PASTE_ID}") || __fail "Delete paste: request failed"
    printf '%s' "${DOCKER_DEL}" | grep -qi -- '"ok".*true\|deleted' || \
        __fail "Delete paste: unexpected response: ${DOCKER_DEL}"
    __pass "Delete paste (owner token)"

    # Verify it's gone
    __check_status "Deleted paste returns 404" GET \
        "${DOCKER_BASE_URL}/api/v1/pastes/${DOCKER_PASTE_ID}" 404
fi

# ── Error cases ──────────────────────────────────────────────────────────────
__info "--- Error cases ---"
__check_status "Get nonexistent paste → 404" GET "${DOCKER_BASE_URL}/api/v1/pastes/notexist" 404
__check_status "Empty content → 400" POST "${DOCKER_BASE_URL}/api/v1/pastes" 400 \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    --data '{"content":""}'

# ── Compat APIs ───────────────────────────────────────────────────────────────
__info "--- Compat API endpoints ---"
# pastebin.com compat
DOCKER_COMPAT_RESPONSE=$(curl -sf \
    --data "api_paste_code=compat+test&api_paste_private=0" \
    "${DOCKER_BASE_URL}/api/api_post.php") || __fail "pastebin.com compat POST: request failed"
printf '%s' "${DOCKER_COMPAT_RESPONSE}" | grep -qi -- "http\|paste\|error" || \
    __fail "pastebin.com compat POST: unexpected response: ${DOCKER_COMPAT_RESPONSE}"
__pass "pastebin.com compat POST /api/api_post.php"

__check_status "pastebin.com login stub → 200" POST "${DOCKER_BASE_URL}/api/api_login.php" 200 \
    --data "api_dev_key=x&api_user_name=x&api_user_password=x"

# lenpaste compat
__check_status "lenpaste /api/new → 200/201" POST "${DOCKER_BASE_URL}/api/new" 200 \
    --header "Content-Type: application/json" \
    --data '{"text":"lenpaste compat test"}'

__check_status "lenpaste /api/v1/getServerInfo → 200" GET "${DOCKER_BASE_URL}/api/v1/getServerInfo" 200

# ── Swagger / GraphQL endpoints ───────────────────────────────────────────────
__info "--- API documentation endpoints ---"
__check_status "GET /api/swagger → 200"             GET "${DOCKER_BASE_URL}/api/swagger"           200
__check_status "GET /api/v1/server/swagger → 200"   GET "${DOCKER_BASE_URL}/api/v1/server/swagger" 200
__check_status "GET /api/graphql → 200"             GET "${DOCKER_BASE_URL}/api/graphql"           200
__check_status "GET /api/v1/server/graphql → 200"   GET "${DOCKER_BASE_URL}/api/v1/server/graphql" 200
__check_status "GET /server/docs/graphql → 200"     GET "${DOCKER_BASE_URL}/server/docs/graphql"   200

# ── Static / SEO endpoints ───────────────────────────────────────────────────
__info "--- Static endpoints ---"
__check_status "GET /robots.txt → 200"   GET "${DOCKER_BASE_URL}/robots.txt"   200
__check_status "GET /security.txt → 200" GET "${DOCKER_BASE_URL}/security.txt" 200

# ── Security headers ─────────────────────────────────────────────────────────
__info "--- Security headers ---"
DOCKER_HEADERS=$(curl -sI "${DOCKER_BASE_URL}/" | tr '[:upper:]' '[:lower:]')
printf '%s' "${DOCKER_HEADERS}" | grep -q -- "x-content-type-options" || \
    __fail "Missing X-Content-Type-Options header"
__pass "X-Content-Type-Options present"
printf '%s' "${DOCKER_HEADERS}" | grep -q -- "x-frame-options\|content-security-policy" || \
    __fail "Missing X-Frame-Options or Content-Security-Policy header"
__pass "Framing protection header present"

# ── --status flag ─────────────────────────────────────────────────────────────
docker exec "${DOCKER_CONTAINER_NAME}" /usr/local/bin/pastebin --status >/dev/null || \
    __fail "--status exited non-zero"
__pass "--status flag: ok"

# ── Final result ─────────────────────────────────────────────────────────────
if [[ "${DOCKER_ERRORS}" -gt 0 ]]; then
    printf "\n[RESULT] %d test(s) FAILED\n" "${DOCKER_ERRORS}" >&2
    exit 1
else
    __info "All Docker integration tests passed."
fi
