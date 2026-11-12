#!/usr/bin/env bash
# The gate must pass a docs-only run and fail when a selected job fails or is cancelled.
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
gate="$root/gate.sh"
export CHANGES DOCS CATALOG COMPANION APP RELEASE

run() {
  if bash "$gate"; then
    echo "ok ${CHANGES} docs=${DOCS} app=${APP}"
  else
    echo "FAIL ${CHANGES} docs=${DOCS} app=${APP}" >&2
    exit 1
  fi
}
run_fail() {
  if bash "$gate"; then
    echo "FAIL (expected nonzero) ${CHANGES} app=${APP} docs=${DOCS}" >&2
    exit 1
  else
    echo "ok (rejected) ${CHANGES} app=${APP} docs=${DOCS}"
  fi
}

CHANGES=success DOCS=success CATALOG=skipped COMPANION=skipped APP=skipped RELEASE=skipped run
CHANGES=success DOCS=skipped CATALOG=success COMPANION=skipped APP=skipped RELEASE=skipped run
CHANGES=success DOCS=skipped CATALOG=skipped COMPANION=success APP=skipped RELEASE=skipped run
CHANGES=success DOCS=skipped CATALOG=skipped COMPANION=skipped APP=success RELEASE=success run
CHANGES=failure DOCS=skipped CATALOG=skipped COMPANION=skipped APP=skipped RELEASE=skipped run_fail
CHANGES=success DOCS=skipped CATALOG=skipped COMPANION=skipped APP=failure RELEASE=skipped run_fail
CHANGES=success DOCS=skipped CATALOG=skipped COMPANION=skipped APP=cancelled RELEASE=skipped run_fail
CHANGES=skipped DOCS=skipped CATALOG=skipped COMPANION=skipped APP=skipped RELEASE=skipped run_fail
CHANGES=success DOCS=failure CATALOG=skipped COMPANION=skipped APP=skipped RELEASE=skipped run_fail
echo "gate tests passed"
