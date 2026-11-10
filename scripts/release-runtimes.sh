#!/usr/bin/env bash
# Cross-compile the mypack runtime for every platform, package it as
# dist/registry/<version>/mypack-runtime-<version>-<os>-<arch>.tar.gz and
# publish a signed registry manifest (dist/registry/registry.json + .sig).
#
# Usage:
#   scripts/release-runtimes.sh                 # VERSION=1.0.0, all targets
#   VERSION=1.1.0 scripts/release-runtimes.sh   # adds a version, keeps older ones
#   TARGETS="darwin-arm64 linux-amd64" scripts/release-runtimes.sh
#
# Environment:
#   VERSION        runtime version (default 1.0.0)
#   DIST           output root (default <repo>/dist)
#   TARGETS        space separated os-arch list (default: all six)
#   RUNTIME_PKG    Go package of the runtime (default ./runtime/cmd/mypack-runtime)
#   CAPABILITIES   comma separated capabilities advertised in the manifest
#   EXPIRES_DAYS   manifest validity (default 365)
#   GOTOOLCHAIN    default "auto" (Go >= 1.27.3 is downloaded on demand)
#
# The dev signing key is generated on first run in dist/keys/ (never commit it).
# To use the registry with a development Builder:
#   export MYPACK_REGISTRY=file://$PWD/dist/registry/registry.json
#   export MYPACK_DEV_TRUST=1        # trusts dist/keys/registry.pub
# or   export MYPACK_TRUSTED_KEY=<base64 public key printed by this script>
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-1.0.0}"
DIST="${DIST:-$ROOT/dist}"
RUNTIME_PKG="${RUNTIME_PKG:-./runtime/cmd/mypack-runtime}"
TARGETS="${TARGETS:-darwin-arm64 darwin-amd64 windows-amd64 windows-arm64 linux-amd64 linux-arm64}"
CAPABILITIES="${CAPABILITIES:-tray,globalShortcut,media,dragDrop,proxy,injection}"
EXPIRES_DAYS="${EXPIRES_DAYS:-365}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

if ! [[ "$VERSION" =~ ^[0-9]+(\.[0-9]+){0,3}$ ]]; then
  echo "VERSION must look like 1.0.0 (got '$VERSION')" >&2
  exit 2
fi
if [ ! -d "$RUNTIME_PKG" ] && [ ! -d "$ROOT/${RUNTIME_PKG#./}" ]; then
  echo "runtime package $RUNTIME_PKG does not exist yet" >&2
  exit 1
fi

REG="$DIST/registry"
KEYS="$DIST/keys"
OUT="$REG/$VERSION"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/mypack-runtimes.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$OUT" "$KEYS" "$DIST/tools"

echo "==> building signing helper"
go build -trimpath -o "$DIST/tools/signregistry" ./scripts/signregistry
SIGN="$DIST/tools/signregistry"
"$SIGN" keygen -keys "$KEYS" >/dev/null

built=""
for t in $TARGETS; do
  goos="${t%%-*}"
  goarch="${t#*-}"
  exe="mypack-runtime"
  [ "$goos" = "windows" ] && exe="mypack-runtime.exe"
  mkdir -p "$WORK/$t"
  echo "==> building $t"
  LDFLAGS="-s -w -X github.com/egoist/mygo.production=1"
  [ "$goos" = "windows" ] && LDFLAGS="$LDFLAGS -H=windowsgui"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
    -tags mygo_noinspector \
    -ldflags "$LDFLAGS" \
    -o "$WORK/$t/$exe" "$RUNTIME_PKG"
  "$SIGN" pack -bin "$WORK/$t/$exe" -key "$t" -out "$OUT/mypack-runtime-$VERSION-$t.tar.gz"
  built="$built $t"
done

# macOS universal binary, only when lipo is available and both slices were built
if command -v lipo >/dev/null 2>&1 && [ -f "$WORK/darwin-arm64/mypack-runtime" ] && [ -f "$WORK/darwin-amd64/mypack-runtime" ]; then
  echo "==> creating darwin-universal"
  mkdir -p "$WORK/darwin-universal"
  lipo -create -output "$WORK/darwin-universal/mypack-runtime" \
    "$WORK/darwin-arm64/mypack-runtime" "$WORK/darwin-amd64/mypack-runtime"
  "$SIGN" pack -bin "$WORK/darwin-universal/mypack-runtime" -key darwin-universal \
    -out "$OUT/mypack-runtime-$VERSION-darwin-universal.tar.gz"
else
  echo "==> skipping darwin-universal (needs lipo and both darwin builds)"
fi

echo "==> signing registry"
PUB="$("$SIGN" registry -dir "$REG" -keys "$KEYS" -capabilities "$CAPABILITIES" -expires-days "$EXPIRES_DAYS")"

echo
echo "Registry:   $REG/registry.json"
echo "Public key: $PUB"
echo "Use with:   MYPACK_REGISTRY=file://$REG/registry.json MYPACK_DEV_TRUST=1"
