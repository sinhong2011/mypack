#!/usr/bin/env bash
# Classify a git diff --name-status listing.
# Prints need_go, need_catalog, need_companion, need_docs, need_release_check.
# Unknown paths set need_go (fail safe: run the app tests).
# Empty input also fails safe.
set -euo pipefail

class_path() {
  local p="$1"
  case "$p" in
    README.md|README.*.md) echo docs ;;
    docs|docs/*) echo docs ;;
    ARCHITECTURE.md|CONTRIBUTING.md|SECURITY.md|CODE_OF_CONDUCT.md|TRADEMARKS.md|NOTICE) echo docs ;;
    .github/pull_request_template.md|.github/ISSUE_TEMPLATE|.github/ISSUE_TEMPLATE/*) echo docs ;;
    catalog/discover.json) echo catalog ;;
    internal/i18n|internal/i18n/*) echo go ;;
    apps/browser-companion|apps/browser-companion/*) echo companion ;;
    *.go|go.mod|go.sum|go.work|go.work.sum) echo go ;;
    Makefile|scripts|scripts/*) echo go ;;
    assets|assets/*|internal/desktop/assets|internal/desktop/assets/*) echo go ;;
    .github/workflows|.github/workflows/*|.github/ci|.github/ci/*) echo ci ;;
    .goreleaser.yaml|.goreleaser.yml|release-please-config.json|.release-please-manifest.json) echo ci ;;
    *) echo unknown ;;
  esac
}

strip_quotes() {
  local s="$1"
  if [[ "$s" == \"*\" && "$s" == *\" ]]; then
    s="${s#\"}"
    s="${s%\"}"
  fi
  printf '%s' "$s"
}

note() {
  case "$1" in
    docs) need_docs=true ;;
    catalog) need_catalog=true ;;
    companion) need_companion=true ;;
    go) need_go=true ;;
    ci) need_go=true; need_release_check=true ;;
    unknown) need_go=true ;;
  esac
}

classify_stream() {
  need_go=false
  need_catalog=false
  need_companion=false
  need_docs=false
  need_release_check=false
  local saw=false status p1 p2
  while IFS=$'\t' read -r status p1 p2 || [[ -n "${status:-}" ]]; do
    [[ -z "${status:-}" || "$status" == \#* ]] && continue
    saw=true
    p1="$(strip_quotes "$p1")"
    case "$status" in
      R*|C*)
        p2="$(strip_quotes "$p2")"
        note "$(class_path "$p1")"
        note "$(class_path "$p2")"
        ;;
      *)
        note "$(class_path "$p1")"
        ;;
    esac
  done
  if [[ "$saw" == false ]]; then
    note unknown
  fi
  printf 'need_go=%s\n' "$need_go"
  printf 'need_catalog=%s\n' "$need_catalog"
  printf 'need_companion=%s\n' "$need_companion"
  printf 'need_docs=%s\n' "$need_docs"
  printf 'need_release_check=%s\n' "$need_release_check"
}

self_test() {
  local root fail=0 fixture expect got key
  root="$(cd "$(dirname "$0")" && pwd)"
  shopt -s nullglob
  local fixtures=("$root"/fixtures/*.diff)
  if [[ ${#fixtures[@]} -eq 0 ]]; then
    echo "no fixtures in $root/fixtures" >&2
    return 1
  fi
  for fixture in "${fixtures[@]}"; do
    expect="${fixture%.diff}.expect"
    if [[ ! -f "$expect" ]]; then
      echo "missing $expect" >&2
      fail=1
      continue
    fi
    got="$(classify_stream < "$fixture")"
    if [[ "$got" != "$(cat "$expect")" ]]; then
      echo "FAIL ${fixture#"$root"/fixtures/}" >&2
      echo "  got:" >&2
      printf '%s\n' "$got" | sed 's/^/    /' >&2
      echo "  want:" >&2
      sed 's/^/    /' "$expect" >&2
      fail=1
    else
      echo "ok ${fixture#"$root"/fixtures/}"
    fi
  done
  if [[ "$fail" -ne 0 ]]; then
    return 1
  fi
}

if [[ "${1:-}" == "--self-test" ]]; then
  self_test
else
  classify_stream
fi
