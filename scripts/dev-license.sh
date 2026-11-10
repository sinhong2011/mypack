#!/usr/bin/env bash
# Issue a development Pro license so dev builds have no managed-app limit.
# Writes .env (gitignored) for `make run` and activates the license for the
# locally built app. The token is signed by the dev key in dist/keys/ and is
# only accepted by builds that trust that key.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

tok="$(go run ./scripts/issuelicense -email dev@mypack.local 2>/dev/null | tail -1)"
pub="$(cat dist/keys/license.pub)"
printf 'MYPACK_LICENSE=%s\nMYPACK_LICENSE_PUBKEY=%s\n' "$tok" "$pub" > .env
MYPACK_LICENSE_PUBKEY="$pub" go run ./cmd/mypack-cli license activate "$tok" >/dev/null
echo "dev license: written to .env and activated for this user"
