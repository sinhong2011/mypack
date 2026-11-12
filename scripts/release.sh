#!/usr/bin/env bash
# Cuts a Mypack release by pushing its tag, as MyGo does: no release pull
# request and no version in the source. The Release workflow tests the
# tagged commit and publishes. See docs/releasing.md.
#
#   scripts/release.sh 0.2.0          # tag main as v0.2.0 and push the tag
#   scripts/release.sh 0.2.0-beta.1   # a pre-release (anything after "-")
#   scripts/release.sh --check-tag v0.2.0
#   scripts/release.sh --self-test
set -euo pipefail

semver='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

check_version() { [[ "$1" =~ $semver ]]; }

check_tag() { [[ "$1" == v* ]] && check_version "${1#v}"; }

self_test() {
  local ok=0
  for t in v0.1.0 v1.2.3 v0.2.0-beta.1 v10.0.0-rc.2; do
    check_tag "$t" || { echo "should pass: $t"; ok=1; }
  done
  for t in 0.1.0 v0.1 v0.1.0. vv0.1.0 v0.1.0+meta v0.1.0-; do
    if check_tag "$t"; then echo "should fail: $t"; ok=1; fi
  done
  [[ $ok == 0 ]] && echo "release.sh: examples pass"
  return $ok
}

# The remote that is the GitHub repository (this checkout may have others).
github_remote() {
  local r
  for r in $(git remote); do
    if git remote get-url "$r" | grep -q 'github\.com[:/]sinhong2011/mypack'; then
      echo "$r"
      return
    fi
  done
  echo "no remote points at github.com/sinhong2011/mypack" >&2
  return 1
}

case "${1:-}" in
  --self-test) self_test; exit ;;
  --check-tag)
    if ! check_tag "${2:-}"; then
      echo "tag '${2:-}' is not vX.Y.Z or vX.Y.Z-pre" >&2
      exit 1
    fi
    echo "tag ${2} is a release tag"
    exit ;;
  ""|-h|--help)
    sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
    exit 2 ;;
esac

version="${1#v}"
tag="v$version"
check_version "$version" || { echo "'$1' is not X.Y.Z or X.Y.Z-pre" >&2; exit 2; }

remote="$(github_remote)"
git fetch --quiet --tags "$remote" main

[[ -z "$(git status --porcelain --untracked-files=no)" ]] || { echo "commit or stash your changes first" >&2; exit 1; }
[[ "$(git rev-parse --abbrev-ref HEAD)" == main ]] || { echo "check out main first" >&2; exit 1; }
[[ "$(git rev-parse HEAD)" == "$(git rev-parse "$remote/main")" ]] || { echo "main is not $remote/main: pull or push first" >&2; exit 1; }
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  echo "$tag already exists" >&2
  exit 1
fi

last="$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || true)"
echo "Releasing $tag from $(git rev-parse --short HEAD)${last:+, after $last}:"
git log --oneline --no-merges ${last:+"$last"..HEAD} | sed 's/^/  /' | head -40
read -r -p "Push $tag to $remote? [y/N] " yes
[[ "$yes" == [yY] ]] || { echo "nothing pushed"; exit 1; }

git tag -a "$tag" -m "Mypack $version"
git push "$remote" "refs/tags/$tag"
echo "pushed $tag: https://github.com/sinhong2011/mypack/actions/workflows/release.yml"
