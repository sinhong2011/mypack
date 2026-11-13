#!/usr/bin/env bash
# Stages the host-platform runtime so `mygo build` ships it inside the
# installer. On first launch MyPack copies it into the runtime cache; the
# remote registry.json is fetched only for a version or platform that is
# not already there.
#
# Windows and Linux: resources/runtimes/*.tar.gz plus registry.json.
# macOS: extract the archive into resources/darwin-<arch>/bin/mypack-runtime
# (MyGo platform dirs). `mygo build` signs that Mach-O with the app, so
# Apple notarization passes. The .tar.gz is not shipped: the notary service
# unpacks gzip and would reject the unsigned nested binary.
#
#   scripts/bundle-runtime.sh darwin/universal
#   scripts/bundle-runtime.sh windows/amd64
#   scripts/bundle-runtime.sh --self-test
#
# PLATFORM (os/arch) comes from the argument, $PLATFORM, or `go env`.
# RUNTIME_VERSION defaults to pkg/config DefaultRuntimeVersion.
# Sources, in order: dist/registry, the "runtimes" GitHub Release, a local
# scripts/release-runtimes.sh build of just the needed targets.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DEST="${BUNDLE_DEST:-$ROOT/resources/runtimes}"
REPO="${GITHUB_REPOSITORY:-sinhong2011/mypack}"
DIST="${DIST:-$ROOT/dist}"
TAG="${RUNTIMES_TAG:-runtimes}"

keys_for_platform() {
  case "$1" in
    darwin/universal) echo "darwin-arm64 darwin-amd64" ;;
    darwin/arm64) echo "darwin-arm64" ;;
    darwin/amd64) echo "darwin-amd64" ;;
    windows/amd64) echo "windows-amd64" ;;
    windows/arm64) echo "windows-arm64" ;;
    linux/amd64) echo "linux-amd64" ;;
    linux/arm64) echo "linux-arm64" ;;
    *)
      echo "bundle-runtime: unknown platform $1 (want os/arch)" >&2
      return 1
      ;;
  esac
}

runtime_version() {
  if [ -n "${RUNTIME_VERSION:-}" ]; then
    printf '%s\n' "$RUNTIME_VERSION"
    return
  fi
  sed -n 's/^const DefaultRuntimeVersion = "\([^"]*\)".*/\1/p' "$ROOT/pkg/config/config.go"
}

archive_name() {
  printf 'mypack-runtime-%s-%s.tar.gz\n' "$1" "$2"
}

have_archive() {
  [ -f "$1" ] && [ -s "$1" ]
}

copy_registry() {
  local src="$1"
  if [ -f "$src/registry.json" ] && [ -f "$src/registry.json.sig" ]; then
    cp "$src/registry.json" "$src/registry.json.sig" "$DEST/"
    return 0
  fi
  return 1
}

# gh release download takes one positional (the tag). A filename is --pattern,
# not a second argument; a second argument is "accepts at most 1 arg(s)".
download_github() {
  local ver="$1" key="$2" dest="$3"
  local name url
  name="$(archive_name "$ver" "$key")"
  url="https://github.com/${REPO}/releases/download/${TAG}/$name"
  if command -v gh >/dev/null 2>&1 && gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
    if gh release download "$TAG" --repo "$REPO" --dir "$DEST" --clobber --pattern "$name"; then
      return 0
    fi
  fi
  curl -fsSL -o "$dest" "$url"
}

download_registry() {
  if command -v gh >/dev/null 2>&1 && gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
    if gh release download "$TAG" --repo "$REPO" --dir "$DEST" --clobber --pattern "registry.json" --pattern "registry.json.sig"; then
      return 0
    fi
  fi
  curl -fsSL -o "$DEST/registry.json" "https://github.com/${REPO}/releases/download/${TAG}/registry.json"
  curl -fsSL -o "$DEST/registry.json.sig" "https://github.com/${REPO}/releases/download/${TAG}/registry.json.sig"
}

# Confirm archive bytes match registry.json before we extract them into the
# app. python3 is on the macOS and Ubuntu runners and on developer Macs.
verify_archive() {
  local archive="$1" key="$2" version="$3" registry="$4"
  python3 - "$archive" "$key" "$version" "$registry" <<'PY'
import hashlib, json, os, sys

archive, key, version, registry = sys.argv[1:]
os_name, _, arch = key.partition("-")
with open(registry, encoding="utf-8") as f:
    man = json.load(f)
want = None
for e in man.get("runtimes") or []:
    if e.get("version") == version and e.get("os") == os_name and e.get("arch") == arch:
        want = e
        break
if want is None:
    sys.exit(f"registry.json has no {version} {key}")
h = hashlib.sha256()
with open(archive, "rb") as f:
    for chunk in iter(lambda: f.read(1 << 20), b""):
        h.update(chunk)
got, size = h.hexdigest(), os.path.getsize(archive)
if got.lower() != str(want.get("sha256", "")).lower() or size != int(want["size"]):
    sys.exit(
        f"{os.path.basename(archive)} sha256/size {got} {size} "
        f"!= registry {want.get('sha256')} {want.get('size')}"
    )
print(f"bundle-runtime: verified {os.path.basename(archive)}")
PY
}

# Unpack mypack-runtime into the MyGo platform directory so `mygo build`
# signs the Mach-O. Do not leave the .tar.gz in resources/: Apple's notary
# service unpacks gzip and rejects the unsigned nested binary.
extract_darwin() {
  local archive="$1" key="$2"
  local dest="$ROOT/resources/${key}/bin"
  local tmp exe
  mkdir -p "$dest"
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/mypack-runtime.XXXXXX")"
  tar -xzf "$archive" -C "$tmp"
  exe="$tmp/mypack-runtime"
  if [ ! -f "$exe" ]; then
    echo "bundle-runtime: $archive has no mypack-runtime" >&2
    rm -rf "$tmp"
    return 1
  fi
  mv "$exe" "$dest/mypack-runtime"
  chmod 755 "$dest/mypack-runtime"
  rm -rf "$tmp"
  rm -f "$archive"
  echo "bundle-runtime: $key → resources/${key}/bin/mypack-runtime"
}

self_test() {
  local fail=0
  want="darwin-arm64 darwin-amd64"
  got="$(keys_for_platform darwin/universal)"
  [ "$got" = "$want" ] || { echo "universal keys: got '$got' want '$want'"; fail=1; }
  got="$(keys_for_platform windows/amd64)"
  [ "$got" = "windows-amd64" ] || { echo "windows keys: got '$got'"; fail=1; }
  ver="$(runtime_version)"
  [ -n "$ver" ] || { echo "DefaultRuntimeVersion missing"; fail=1; }
  echo "$ver" | grep -Eq '^[0-9]+(\.[0-9]+){0,3}$' || { echo "bad version $ver"; fail=1; }
  if ! grep -q 'bundle-runtime.sh' "$ROOT/Makefile"; then
    echo "Makefile does not call bundle-runtime.sh"; fail=1
  fi
  if ! grep -q 'bundle-runtime.sh' "$ROOT/.github/workflows/release.yml"; then
    echo "release.yml does not mention bundle-runtime.sh"; fail=1
  fi
  if grep -E 'gh release download' "$ROOT/scripts/bundle-runtime.sh" | grep -v -- '--pattern' | grep -q .; then
    echo "gh release download must pass --pattern, not a filename argument"; fail=1
  fi
  if ! grep -q 'resources/${key}/bin' "$ROOT/scripts/bundle-runtime.sh"; then
    echo "darwin helper extract (MyGo platform dir) is missing"; fail=1
  fi
  if [ "$fail" = 0 ]; then
    echo "bundle-runtime.sh: examples pass"
  fi
  return "$fail"
}

usage() {
  sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
  --self-test) self_test; exit ;;
  -h|--help) usage; exit 2 ;;
esac

PLATFORM_ARG="${1:-${PLATFORM:-}}"
if [ -z "$PLATFORM_ARG" ]; then
  PLATFORM_ARG="$(go env GOOS)/$(go env GOARCH)"
fi
# One os/arch per invocation so each installer only carries its runtime.
PLATFORM_ARG="${PLATFORM_ARG%%,*}"

VERSION="$(runtime_version)"
if ! [[ "$VERSION" =~ ^[0-9]+(\.[0-9]+){0,3}$ ]]; then
  echo "bundle-runtime: RUNTIME_VERSION must look like 1.0.8 (got '$VERSION')" >&2
  exit 2
fi
KEYS="$(keys_for_platform "$PLATFORM_ARG")"

mkdir -p "$DEST"
rm -f "$DEST"/mypack-runtime-*.tar.gz "$DEST"/registry.json "$DEST"/registry.json.sig "$DEST"/bundled.json
if [[ "$PLATFORM_ARG" == darwin/* ]]; then
  rm -rf "$ROOT/resources/darwin-arm64" "$ROOT/resources/darwin-amd64" "$ROOT/resources/darwin"
fi

missing=""
for key in $KEYS; do
  dest="$DEST/$(archive_name "$VERSION" "$key")"
  if have_archive "$DIST/registry/$VERSION/$(archive_name "$VERSION" "$key")"; then
    cp "$DIST/registry/$VERSION/$(archive_name "$VERSION" "$key")" "$dest"
    echo "bundle-runtime: $key from dist/registry"
    continue
  fi
  if download_github "$VERSION" "$key" "$dest" && have_archive "$dest"; then
    echo "bundle-runtime: $key from GitHub Release $TAG"
    continue
  fi
  rm -f "$dest"
  missing="$missing $key"
done

if [ -n "$missing" ]; then
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "bundle-runtime: missing$missing from GitHub Release $TAG (no local rebuild in CI; it would sign registry.json with a new key no build trusts)" >&2
    exit 1
  fi
  echo "bundle-runtime: building$missing locally"
  VERSION="$VERSION" TARGETS="$missing" DIST="$DIST" "$ROOT/scripts/release-runtimes.sh"
  for key in $missing; do
    src="$DIST/registry/$VERSION/$(archive_name "$VERSION" "$key")"
    have_archive "$src" || { echo "bundle-runtime: failed to produce $src" >&2; exit 1; }
    cp "$src" "$DEST/$(archive_name "$VERSION" "$key")"
  done
fi

copy_registry "$DIST/registry" || download_registry || {
  echo "bundle-runtime: registry.json is missing (dist/registry and GitHub Release $TAG)" >&2
  exit 1
}
[ -s "$DEST/registry.json" ] && [ -s "$DEST/registry.json.sig" ] || {
  echo "bundle-runtime: registry.json or registry.json.sig is empty" >&2
  exit 1
}

for key in $KEYS; do
  have_archive "$DEST/$(archive_name "$VERSION" "$key")" || {
    echo "bundle-runtime: missing $(archive_name "$VERSION" "$key")" >&2
    exit 1
  }
done

if [[ "$PLATFORM_ARG" == darwin/* ]]; then
  for key in $KEYS; do
    archive="$DEST/$(archive_name "$VERSION" "$key")"
    verify_archive "$archive" "$key" "$VERSION" "$DEST/registry.json"
    extract_darwin "$archive" "$key"
  done
  # First launch seeds this Mach-O (MyGo-signed in the installer) into the cache.
  printf '{"version":"%s","keys":[%s]}\n' "$VERSION" "$(
    first=1
    for key in $KEYS; do
      if [ "$first" = 1 ]; then first=0; else printf ','; fi
      printf '"%s"' "$key"
    done
  )" >"$DEST/bundled.json"
  echo "bundle-runtime: $PLATFORM_ARG → helper $KEYS ($VERSION) + registry.json"
else
  echo "bundle-runtime: $PLATFORM_ARG →$KEYS ($VERSION) in $DEST"
fi
