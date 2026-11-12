#!/usr/bin/env bash
# Accept a Conventional Commits title: type(scope)!: description
# https://www.conventionalcommits.org/en/v1.0.0/
set -euo pipefail

pattern='^(feat|fix|docs|style|refactor|perf|test|build|chore|ci|revert)(\([A-Za-z0-9._/-]+\))?!?: [^ ].+'

check_title() {
  local title="$1"
  [[ "$title" =~ $pattern ]]
}

self_test() {
  local ok fail=0 title
  local -a good=(
    'feat: add release publishing'
    'fix: accept Windows paths'
    'docs: update the library screenshot'
    'chore(main): release 0.1.0'
    'feat!: drop the old registry'
    'ci(release): check titles'
    'revert: undo the title check'
  )
  local -a bad=(
    'Update the README'
    'feat:'
    'feat: '
    'feat:missing space'
    'Feature: add a thing'
    'WIP'
  )
  for title in "${good[@]}"; do
    if check_title "$title"; then
      echo "ok  $title"
    else
      echo "FAIL (should pass) $title" >&2
      fail=1
    fi
  done
  for title in "${bad[@]}"; do
    if check_title "$title"; then
      echo "FAIL (should reject) $title" >&2
      fail=1
    else
      echo "ok  rejected: $title"
    fi
  done
  return "$fail"
}

if [[ "${1:-}" == "--self-test" ]]; then
  self_test
  exit $?
fi

title="${1:-${TITLE:-}}"
if check_title "$title"; then
  echo "title ok: $title"
  exit 0
fi
echo "Pull request title is not a Conventional Commit:" >&2
echo "  $title" >&2
echo "Use type(scope)!: description. Types: feat, fix, docs, style, refactor, perf, test, build, chore, ci, revert." >&2
echo "Example: docs: update the library screenshot" >&2
exit 1
