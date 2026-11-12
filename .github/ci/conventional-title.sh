#!/usr/bin/env bash
# Conventional Commits for the pull request title, the branch name, and
# each commit subject. https://www.conventionalcommits.org/en/v1.0.0/
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
# Commit subjects use the same title pattern. They do not have to share the
# branch type: a feature pull request may contain a fix.
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

# Subjects, one per line. Blank lines are ignored.
check_subjects() {
  local bad=0 subject saw=false
  while IFS= read -r subject || [[ -n "$subject" ]]; do
    [[ -z "$subject" ]] && continue
    saw=true
    if check_title "$subject"; then
      echo "commit ok: $subject"
    else
      echo "Commit subject is not a Conventional Commit:" >&2
      echo "  $subject" >&2
      bad=1
    fi
  done
  if [[ "$saw" == false ]]; then
    echo "No commit subjects to check." >&2
    return 1
  fi
  return "$bad"
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
  if printf '%s\n' 'ci: check pull request titles' 'fix: accept Windows paths' | check_subjects >/dev/null; then
    echo "ok  commit subjects"
  else
    echo "FAIL (should pass subjects)" >&2
    fail=1
  fi
  if printf '%s\n' 'ci: check pull request titles' 'Update the README' | check_subjects >/dev/null; then
    echo "FAIL (should reject a non-conventional subject)" >&2
    fail=1
  else
    echo "ok  rejected commit subject"
  fi
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
if [[ -n "${RANGE_BASE:-}" && -n "${RANGE_HEAD:-}" ]]; then
  git log --no-merges --format=%s "${RANGE_BASE}..${RANGE_HEAD}" | check_subjects
fi
