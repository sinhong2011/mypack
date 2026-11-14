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
#   TIERS          runtime tiers to build (default "full lite"). The lite
#                  tier is -tags mypack_lite (runtime/app/lite.go) and ships as
#                  mypack-runtime-<version>-<os>-<arch>-lite.tar.gz
#   CAPABILITIES   comma separated capabilities advertised in the manifest
#   EXPIRES_DAYS   manifest validity (default 365)
#   GOTOOLCHAIN    default "auto" (Go >= 1.27.3 is downloaded on demand)
#   KEYS           signing key directory (default <DIST>/keys)
#   REGISTRY_FLAGS extra signregistry registry flags; the Runtimes workflow
#                  passes "-flat -merge" for the GitHub Release (docs/releasing.md)
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
TIERS="${TIERS:-full lite}"
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
KEYS="${KEYS:-$DIST/keys}"
OUT="$REG/$VERSION"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/mypack-runtimes.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$OUT" "$KEYS" "$DIST/tools"

echo "==> building signing helper"
go build -trimpath -o "$DIST/tools/signregistry" ./scripts/signregistry
SIGN="$DIST/tools/signregistry"
"$SIGN" keygen -keys "$KEYS" >/dev/null

# suffix_for <tier>: the key/archive suffix of a tier ("" or "-lite").
suffix_for() {
  case "$1" in
    full) echo "" ;;
    lite) echo "-lite" ;;
    *)
      echo "unknown tier $1 (want full or lite)" >&2
      return 1
      ;;
  esac
}

built=""
for t in $TARGETS; do
  t="${t%-lite}" # a lite key builds its platform (both tiers, per TIERS)
  case " $built " in *" $t "*) continue ;; esac
  goos="${t%%-*}"
  goarch="${t#*-}"
  exe="mypack-runtime"
  [ "$goos" = "windows" ] && exe="mypack-runtime.exe"
  LDFLAGS="-s -w -X github.com/egoist/mygo.production=1"
  [ "$goos" = "windows" ] && LDFLAGS="$LDFLAGS -H=windowsgui"
  for tier in $TIERS; do
    sfx="$(suffix_for "$tier")"
    tags="mygo_noinspector"
    [ "$tier" = "lite" ] && tags="$tags mypack_lite"
    mkdir -p "$WORK/$t$sfx"
    echo "==> building $t ($tier)"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
      -tags "$tags" \
      -ldflags "$LDFLAGS" \
      -o "$WORK/$t$sfx/$exe" "$RUNTIME_PKG"
    "$SIGN" pack -bin "$WORK/$t$sfx/$exe" -key "$t$sfx" -out "$OUT/mypack-runtime-$VERSION-$t$sfx.tar.gz"
  done
  built="$built $t"
done

# macOS universal binaries, only when lipo is available and both slices were built
for tier in $TIERS; do
  sfx="$(suffix_for "$tier")"
  if command -v lipo >/dev/null 2>&1 && [ -f "$WORK/darwin-arm64$sfx/mypack-runtime" ] && [ -f "$WORK/darwin-amd64$sfx/mypack-runtime" ]; then
    echo "==> creating darwin-universal$sfx"
    mkdir -p "$WORK/darwin-universal$sfx"
    lipo -create -output "$WORK/darwin-universal$sfx/mypack-runtime" \
      "$WORK/darwin-arm64$sfx/mypack-runtime" "$WORK/darwin-amd64$sfx/mypack-runtime"
    "$SIGN" pack -bin "$WORK/darwin-universal$sfx/mypack-runtime" -key "darwin-universal$sfx" \
      -out "$OUT/mypack-runtime-$VERSION-darwin-universal$sfx.tar.gz"
  else
    echo "==> skipping darwin-universal$sfx (needs lipo and both darwin builds)"
  fi
done

echo "==> signing registry"
PUB="$("$SIGN" registry -dir "$REG" -keys "$KEYS" -capabilities "$CAPABILITIES" -expires-days "$EXPIRES_DAYS" ${REGISTRY_FLAGS:-})"

echo
echo "Registry:   $REG/registry.json"
echo "Public key: $PUB"
echo "Use with:   MYPACK_REGISTRY=file://$REG/registry.json MYPACK_DEV_TRUST=1"
