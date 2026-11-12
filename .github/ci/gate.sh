#!/usr/bin/env bash
# Aggregate required jobs. success and skipped are fine.
# failure, cancelled, or anything else fails the gate.
# The change detector must have succeeded; a skipped detector is a failure.
set -euo pipefail

fail=0
report() {
  local name="$1" val="$2"
  case "$val" in
    success|skipped) ;;
    *)
      echo "$name=$val" >&2
      fail=1
      ;;
  esac
}

report changes "${CHANGES:-}"
report docs "${DOCS:-}"
report catalog "${CATALOG:-}"
report companion "${COMPANION:-}"
report app "${APP:-}"
report release "${RELEASE:-}"

if [[ "${CHANGES:-}" != "success" ]]; then
  echo "change detector must succeed (got ${CHANGES:-empty})" >&2
  fail=1
fi

exit "$fail"
