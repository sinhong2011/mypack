#!/usr/bin/env bash
# Conventional Commits title, and a matching branch name when the branch
# starts with a type. https://www.conventionalcommits.org/en/v1.0.0/
set -euo pipefail

types='feat|fix|docs|style|refactor|perf|test|build|chore|ci|revert'
title_pattern="^(${types})(\\([A-Za-z0-9._/-]+\\))?!?: [^ ].+"

check_title() {
  local title="$1"
  [[ "$title" =~ $title_pattern ]]
}

title_type() {
  local title="$1"
  [[ "$title" =~ $title_pattern ]] || return 1
  printf '%s' "${BASH_REMATCH[1]}"
}

# A type/slug branch must use the same type as the title.
# cursor/, dependabot/, renovate/, and release-please-- branches are left alone.
check_branch() {
  local branch="$1" ttype="$2"
  local first="${branch%%/*}"
  case "$first" in
    cursor|dependabot|renovate) return 0 ;;
  esac
  if [[ "$branch" == release-please--* ]]; then
    return 0
  fi
  if [[ "$first" =~ ^($types)$ ]]; then
    [[ "$first" == "$ttype" ]]
    return
  fi
  return 0
}

self_test() {
  local fail=0
  expect_ok() {
    local title="$1" branch="$2"
    if check_title "$title" && check_branch "$branch" "$(title_type "$title")"; then
      echo "ok  $branch  $title"
    else
      echo "FAIL (should pass) $branch  $title" >&2
      fail=1
    fi
  }
  expect_bad_title() {
    local title="$1"
    if check_title "$title"; then
      echo "FAIL (should reject title) $title" >&2
      fail=1
    else
      echo "ok  rejected title: $title"
    fi
  }
  expect_bad_branch() {
    local title="$1" branch="$2"
    if check_title "$title" && check_branch "$branch" "$(title_type "$title")"; then
      echo "FAIL (should reject branch) $branch  $title" >&2
      fail=1
    else
      echo "ok  rejected branch: $branch  $title"
    fi
  }

  expect_ok 'docs: update the library screenshot' 'docs/update-library-screenshot'
  expect_ok 'feat: add release publishing' 'feat/release-publishing'
  expect_ok 'fix: accept Windows paths' 'fix/windows-paths'
  expect_ok 'chore(main): release 0.1.0' 'chore/release-0.1.0'
  expect_ok 'ci: check pull request titles' 'cursor/conventional-title-6f82'
  expect_ok 'chore: bump a dependency' 'dependabot/npm_and_yarn/left-pad-1.0.0'
  expect_ok 'chore(main): release 0.1.0' 'release-please--branches--main'
  expect_ok 'docs: update the library screenshot' 'wip-readme'

  expect_bad_title 'Update the README'
  expect_bad_title 'feat:'
  expect_bad_title 'feat: '
  expect_bad_title 'feat:missing space'
  expect_bad_title 'Feature: add a thing'
  expect_bad_branch 'feat: add release publishing' 'docs/update-library-screenshot'
  expect_bad_branch 'docs: update the library screenshot' 'fix/windows-paths'
  return "$fail"
}

if [[ "${1:-}" == "--self-test" ]]; then
  self_test
  exit $?
fi

title="${TITLE:-${1:-}}"
branch="${BRANCH:-${2:-}}"
if ! check_title "$title"; then
  echo "Pull request title is not a Conventional Commit:" >&2
  echo "  $title" >&2
  echo "Use type(scope)!: description. Types: feat, fix, docs, style, refactor, perf, test, build, chore, ci, revert." >&2
  echo "Example: docs: update the library screenshot" >&2
  exit 1
fi
ttype="$(title_type "$title")"
if ! check_branch "$branch" "$ttype"; then
  echo "Branch name prefers the same type as the title." >&2
  echo "  branch: $branch" >&2
  echo "  title:  $title" >&2
  echo "Use ${ttype}/short-description, or rename the branch. cursor/, dependabot/, renovate/, and release-please-- branches are left alone." >&2
  exit 1
fi
echo "title ok: $title"
echo "branch ok: $branch"
exit 0
