#!/usr/bin/env bash
# @@License : WTFPL
# Integration test runner — auto-detects incus or docker (incus preferred).
# Delegates to the runtime-specific script so there is a single source of
# truth per runtime (tests/incus.sh, tests/docker.sh).
# Usage: ./tests/run_tests.sh [extra args passed through to the runner]
set -euo pipefail

RUN_TESTS_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"

__log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
__die() { __log "ERROR: $*" >&2; exit 1; }

# Runtime detection — incus is preferred per PART 28; docker is the fallback.
if command -v incus >/dev/null 2>&1; then
    RUN_TESTS_RUNTIME="incus"
elif command -v docker >/dev/null 2>&1; then
    RUN_TESTS_RUNTIME="docker"
else
    __die "Neither incus nor docker found. Install one to run integration tests."
fi

__log "Runtime: ${RUN_TESTS_RUNTIME}"

case "${RUN_TESTS_RUNTIME}" in
    incus)  exec bash "${RUN_TESTS_SCRIPT_DIR}/incus.sh" "$@" ;;
    docker) exec bash "${RUN_TESTS_SCRIPT_DIR}/docker.sh" "$@" ;;
esac
