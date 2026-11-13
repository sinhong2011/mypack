#!/usr/bin/env bash
# Cuts a MyPack release by pushing its tag, as MyGo does: no release pull
# request and no version in the source. The release notes are written into
# the annotated tag, and the Release workflow puts them at the top of the
# GitHub Release, above the generated list of pull requests. See
# docs/releasing.md.
#
#   scripts/release.sh 0.2.0                 # write the notes in $EDITOR
#   scripts/release.sh 0.2.0 --notes n.md    # take them from a file
#   scripts/release.sh 0.2.0-beta.1          # a pre-release (anything after "-")
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
    sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
    exit 2 ;;
esac

version="${1#v}"
tag="v$version"
check_version "$version" || { echo "'$1' is not X.Y.Z or X.Y.Z-pre" >&2; exit 2; }
notes_file=""
if [[ "${2:-}" == --notes ]]; then
  notes_file="${3:?--notes needs a file}"
  [[ -f "$notes_file" ]] || { echo "no file $notes_file" >&2; exit 2; }
fi

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
commits="$(git log --format='%s' --no-merges ${last:+"$last"..HEAD})"

# The tag's message is "MyPack X.Y.Z", a blank line, then the notes in
# Markdown. Lines starting with ";" are dropped, so "#" headings stay.
msg="$(mktemp -t mypack-release)"
trap 'rm -f "$msg"' EXIT
{
  echo "MyPack $version"
  echo
  if [[ -n "$notes_file" ]]; then
    cat "$notes_file"
  else
    cat <<NOTES
; Write the release notes for $tag below, in Markdown. They go at the top
; of the GitHub Release, above GitHub's list of merged pull requests.
; Lines starting with ";" are dropped. Leave it empty for the list alone.
; Save and close to continue.
;
; A shape that reads well (drop what is empty):
;
; ## Highlights
; - **Bold lead.** One sentence on what a user can do now.
; ## Breaking changes
; ## Fixes
;
; Commits since ${last:-the first commit}:
$(printf '%s\n' "$commits" | sed 's/^/;   /')

NOTES
  fi
} > "$msg"
if [[ -z "$notes_file" ]]; then
  "$(git var GIT_EDITOR)" "$msg"
fi
# Keep what git will keep, to show it.
notes="$(grep -v '^;' "$msg" | tail -n +3)"

echo "Releasing $tag from $(git rev-parse --short HEAD)${last:+, after $last}."
if [[ -n "$(printf '%s' "$notes" | tr -d '[:space:]')" ]]; then
  echo "Notes:"
  printf '%s\n' "$notes" | sed 's/^/  | /'
else
  echo "No notes: the Release lists the merged pull requests only."
fi
read -r -p "Push $tag to $remote? [y/N] " yes
[[ "$yes" == [yY] ]] || { echo "nothing pushed"; exit 1; }

git -c core.commentChar=';' tag -a "$tag" --cleanup=strip -F "$msg"
git push "$remote" "refs/tags/$tag"
echo "pushed $tag: https://github.com/sinhong2011/mypack/actions/workflows/release.yml"
