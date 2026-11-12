#!/usr/bin/env bash
# Stores notarization credentials in the temporary signing keychain.
# See docs/releasing.md. The private key is never printed.
#
#   scripts/macos-notary-credentials.sh check
#   KEYCHAIN=/path/to/signing.keychain-db scripts/macos-notary-credentials.sh store
#   scripts/macos-notary-credentials.sh --self-test
#
# App Store Connect API key (preferred when any ASC_KEY_* secret is set):
#   Team key:       ASC_KEY_P8, ASC_KEY_ID, and ASC_ISSUER_ID (the Issuer ID
#                   UUID from App Store Connect, not the Developer Team ID).
#   Individual key: ASC_KEY_P8 and ASC_KEY_ID, with ASC_ISSUER_ID unset.
#                   notarytool returns 401 if --issuer is passed for an
#                   Individual key. This script does not invent an issuer.
# Apple ID fallback, only when no ASC_KEY_* secret is set:
#   APPLE_ID, APPLE_TEAM_ID, and APPLE_APP_PASSWORD, all three.
# A partial set fails and names the missing secret. It does not fall back.
set +x
set -euo pipefail

profile="${NOTARY_PROFILE:-notary}"
MODE=""
keydir=""

cleanup_key() {
  # Only the directory this script created. Never a caller-supplied path.
  if [[ "${keydir:-}" == *"/asc-key."* ]]; then
    rm -rf "$keydir"
  fi
  keydir=""
}
trap cleanup_key EXIT

select_mode() {
  local p8="${ASC_KEY_P8:-}"
  local id="${ASC_KEY_ID:-}"
  local issuer="${ASC_ISSUER_ID:-}"
  local apple="${APPLE_ID:-}"
  local team="${APPLE_TEAM_ID:-}"
  local pass="${APPLE_APP_PASSWORD:-}"
  local p8_set=0 id_set=0 issuer_set=0 apple_set=0 team_set=0 pass_set=0
  local missing

  if [[ -n "$p8" ]]; then p8_set=1; fi
  if [[ -n "$id" ]]; then id_set=1; fi
  if [[ -n "$issuer" ]]; then issuer_set=1; fi
  if [[ -n "$apple" ]]; then apple_set=1; fi
  if [[ -n "$team" ]]; then team_set=1; fi
  if [[ -n "$pass" ]]; then pass_set=1; fi

  if [[ "$p8_set" == 1 || "$id_set" == 1 || "$issuer_set" == 1 ]]; then
    if [[ "$p8_set" == 1 && "$id_set" == 1 && "$issuer_set" == 1 ]]; then
      MODE=team
      return 0
    fi
    if [[ "$p8_set" == 1 && "$id_set" == 1 && "$issuer_set" == 0 ]]; then
      MODE=individual
      return 0
    fi
    missing=""
    if [[ "$p8_set" == 0 ]]; then missing="ASC_KEY_P8"; fi
    if [[ "$id_set" == 0 ]]; then
      if [[ -n "$missing" ]]; then missing="$missing ASC_KEY_ID"; else missing="ASC_KEY_ID"; fi
    fi
    if [[ "$issuer_set" == 1 ]]; then
      echo "::error::App Store Connect Team API key credentials are incomplete: missing ${missing}. Set ASC_KEY_P8, ASC_KEY_ID, and ASC_ISSUER_ID. ASC_ISSUER_ID is the Issuer ID from App Store Connect (Users and Access, Integrations), not the Developer Team ID. This workflow does not invent an issuer." >&2
    else
      echo "::error::App Store Connect API key credentials are incomplete: missing ${missing}. A Team key needs ASC_KEY_P8, ASC_KEY_ID, and ASC_ISSUER_ID. An Individual key needs ASC_KEY_P8 and ASC_KEY_ID with ASC_ISSUER_ID unset. This workflow does not invent an issuer." >&2
    fi
    return 1
  fi

  if [[ "$apple_set" == 1 && "$team_set" == 1 && "$pass_set" == 1 ]]; then
    MODE="apple-id"
    return 0
  fi
  if [[ "$apple_set" == 1 || "$team_set" == 1 || "$pass_set" == 1 ]]; then
    missing=""
    if [[ "$apple_set" == 0 ]]; then missing="APPLE_ID"; fi
    if [[ "$team_set" == 0 ]]; then
      if [[ -n "$missing" ]]; then missing="$missing APPLE_TEAM_ID"; else missing="APPLE_TEAM_ID"; fi
    fi
    if [[ "$pass_set" == 0 ]]; then
      if [[ -n "$missing" ]]; then missing="$missing APPLE_APP_PASSWORD"; else missing="APPLE_APP_PASSWORD"; fi
    fi
    echo "::error::Apple ID notarization credentials are incomplete: missing ${missing}. Set APPLE_ID, APPLE_TEAM_ID, and APPLE_APP_PASSWORD, or configure an App Store Connect API key. This workflow does not invent an issuer." >&2
    return 1
  fi

  echo "::error::No notarization credentials are set. Set a Team App Store Connect API key (ASC_KEY_P8, ASC_KEY_ID, and ASC_ISSUER_ID) or an Individual key (ASC_KEY_P8 and ASC_KEY_ID, with ASC_ISSUER_ID unset), or complete Apple ID credentials (APPLE_ID, APPLE_TEAM_ID, and APPLE_APP_PASSWORD)." >&2
  return 1
}

report_mode() {
  case "$MODE" in
    team)
      echo "notary-credentials: team"
      ;;
    individual)
      # Creating API Keys: Individual keys are not able to use notarytool.
      # notarytool(1): --issuer is required for a Team key and must be omitted
      # for an Individual key (passing it returns 401). Do not invent one.
      echo "::warning::ASC_ISSUER_ID is unset, so this is an Individual App Store Connect API key and --issuer is omitted. notarytool returns 401 Unauthorized if --issuer is passed for an Individual key. Apple's Creating API Keys documentation says Individual keys are not able to use notarytool. A Team key sets ASC_ISSUER_ID to the Issuer ID near the top of Users and Access, Integrations. This workflow does not invent an issuer."
      echo "notary-credentials: individual"
      ;;
    apple-id)
      echo "notary-credentials: apple-id"
      ;;
    *)
      echo "::error::internal: notarization mode was not selected" >&2
      return 1
      ;;
  esac
}

run_notary() {
  if [[ -n "${NOTARYTOOL:-}" ]]; then
    "${NOTARYTOOL}" notarytool "$@"
  else
    xcrun notarytool "$@"
  fi
}

write_private_key() {
  local parent
  parent="${NOTARY_KEY_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}}"
  # keydir stays in this shell so the EXIT trap can remove it. A command
  # substitution would drop the assignment and leave the private key behind.
  keydir="$(mktemp -d "${parent}/asc-key.XXXXXX")"
  KEYFILE="${keydir}/AuthKey.p8"
  # Restrictive from the moment the file exists. The secret is the raw PEM text.
  (
    umask 077
    printf '%s' "$ASC_KEY_P8" >"$KEYFILE"
    if [[ "$ASC_KEY_P8" != *$'\n' ]]; then
      printf '\n' >>"$KEYFILE"
    fi
  )
  chmod 600 "$KEYFILE"
  chmod 700 "$keydir"
}

# Drop the secrets from this process before notarytool runs. The workflow
# shell still has them; the child should only see the arguments it needs.
clear_secret_env() {
  unset ASC_KEY_P8 ASC_KEY_ID ASC_ISSUER_ID APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD
}

store_api_key() {
  local key_id issuer
  write_private_key
  key_id="$ASC_KEY_ID"
  issuer="${ASC_ISSUER_ID:-}"
  clear_secret_env
  if [[ "$MODE" == team ]]; then
    run_notary store-credentials "$profile" --keychain "$KEYCHAIN" \
      --key "$KEYFILE" --key-id "$key_id" --issuer "$issuer"
  else
    run_notary store-credentials "$profile" --keychain "$KEYCHAIN" \
      --key "$KEYFILE" --key-id "$key_id"
  fi
  echo "Stored notary credentials (${MODE}) in profile ${profile}."
}

store_apple_id() {
  local apple team_id pass
  apple="$APPLE_ID"
  team_id="$APPLE_TEAM_ID"
  pass="$APPLE_APP_PASSWORD"
  clear_secret_env
  run_notary store-credentials "$profile" --keychain "$KEYCHAIN" \
    --apple-id "$apple" --team-id "$team_id" --password "$pass"
  echo "Stored notary credentials (${MODE}) in profile ${profile}."
}

cmd_check() {
  select_mode
  report_mode
}

cmd_store() {
  select_mode
  if [[ -z "${KEYCHAIN:-}" ]]; then
    echo "::error::KEYCHAIN is not set; notary credentials are stored in the temporary signing keychain." >&2
    return 1
  fi
  report_mode
  case "$MODE" in
    team | individual) store_api_key ;;
    apple-id) store_apple_id ;;
  esac
}

self_test() {
  local root script fail
  local p8 key_id issuer apple team pass
  local work record keydir_parent
  root="$(cd "$(dirname "$0")/.." && pwd)"
  script="${root}/scripts/macos-notary-credentials.sh"
  fail=0
  key_id="KEYIDTEST01"
  issuer="57246542-96fe-1a63-e053-0824d011072a"
  apple="notary-user@example.test"
  team="TEAMIDTEST"
  pass="app-specific-password-sentinel"

  ok() { echo "ok  $*"; }
  bad() { echo "FAIL $*" >&2; fail=1; }

  assert_clean() {
    local name="$1" text="$2"
    local secret
    for secret in "$p8" "bm90LWEtcmVhbC1rZXk=" "$key_id" "$issuer" "$apple" "$team" "$pass"; do
      if [[ "$text" == *"$secret"* ]]; then
        bad "$name printed a credential"
        return
      fi
    done
  }

  work="$(mktemp -d "${TMPDIR:-/tmp}/notary-self-test.XXXXXX")"
  record="${work}/record"
  keydir_parent="${work}/keys"
  mkdir -p "$record" "$keydir_parent"
  # Keep the PEM's trailing newline; command substitution would strip it.
  printf '%s\n' "-----BEGIN PRIVATE KEY-----" "bm90LWEtcmVhbC1rZXk=" "-----END PRIVATE KEY-----" >"${work}/p8.pem"
  p8="$(cat "${work}/p8.pem"; printf x)"
  p8="${p8%x}"
  cat >"${work}/notarytool" <<'MOCK'
#!/usr/bin/env bash
set +x
set -euo pipefail
out="${NOTARY_RECORD:?}"
if [[ -n "${ASC_KEY_P8:-}${ASC_KEY_ID:-}${ASC_ISSUER_ID:-}${APPLE_ID:-}${APPLE_TEAM_ID:-}${APPLE_APP_PASSWORD:-}" ]]; then
  printf 'leak\n' >"${out}/env-leak"
fi
printf '%s\0' "$@" >"${out}/args"
args=("$@")
i=0
while [[ "$i" -lt "${#args[@]}" ]]; do
  if [[ "${args[$i]}" == --key ]]; then
    key="${args[$((i + 1))]}"
    cp "$key" "${out}/key.p8"
    if stat -c %a "$key" >/dev/null 2>&1; then
      stat -c %a "$key" >"${out}/key.mode"
      stat -c %a "$(dirname "$key")" >"${out}/dir.mode"
    else
      stat -f %Lp "$key" >"${out}/key.mode"
      stat -f %Lp "$(dirname "$key")" >"${out}/dir.mode"
    fi
  fi
  i=$((i + 1))
done
if [[ "${NOTARY_FAIL:-}" == 1 ]]; then
  echo "mock notarytool rejected the credentials" >&2
  exit 1
fi
MOCK
  chmod 700 "${work}/notarytool"

  reset_record() {
    rm -rf "$record"
    mkdir -p "$record"
  }
  has_arg() {
    tr '\0' '\n' <"${record}/args" | grep -qxF -e "$1"
  }
  value_after() {
    tr '\0' '\n' <"${record}/args" | awk -v flag="$1" 'prev==flag { print; exit } { prev=$0 }'
  }
  run_cli() {
    local sub="$1"
    cli_rc=0
    cli_out="$(
      ASC_KEY_P8="${ASC_KEY_P8-}" \
        ASC_KEY_ID="${ASC_KEY_ID-}" \
        ASC_ISSUER_ID="${ASC_ISSUER_ID-}" \
        APPLE_ID="${APPLE_ID-}" \
        APPLE_TEAM_ID="${APPLE_TEAM_ID-}" \
        APPLE_APP_PASSWORD="${APPLE_APP_PASSWORD-}" \
        KEYCHAIN="${KEYCHAIN-}" \
        NOTARY_PROFILE="${NOTARY_PROFILE-}" \
        NOTARYTOOL="${NOTARYTOOL-}" \
        NOTARY_KEY_DIR="${NOTARY_KEY_DIR-}" \
        NOTARY_RECORD="${NOTARY_RECORD-}" \
        NOTARY_FAIL="${NOTARY_FAIL-}" \
        bash "$script" "$sub" 2>&1
    )" || cli_rc=$?
  }
  clear_env() {
    unset ASC_KEY_P8 ASC_KEY_ID ASC_ISSUER_ID APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD
    unset KEYCHAIN NOTARY_PROFILE NOTARY_FAIL
    NOTARYTOOL="${work}/notarytool"
    NOTARY_KEY_DIR="$keydir_parent"
    NOTARY_RECORD="$record"
  }
  no_key_left() {
    local name="$1" leftover
    leftover="$(find "$keydir_parent" -mindepth 1 -print)"
    if [[ -n "$leftover" ]]; then
      bad "$name left a private key file behind"
      rm -rf "${keydir_parent:?}/"*
    fi
    no_env_leak "$name"
  }
  no_env_leak() {
    if [[ -f "${record}/env-leak" ]]; then
      bad "${1:-store} exposed credentials to notarytool's environment"
    fi
  }

  clear_env
  ASC_KEY_P8="$p8"
  ASC_KEY_ID="$key_id"
  ASC_ISSUER_ID="$issuer"
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  run_cli check
  if [[ "$cli_rc" == 0 && "$cli_out" == *"notary-credentials: team"* ]]; then
    ok "check team key"
  else
    bad "check team key rc=$cli_rc out=$cli_out"
  fi
  assert_clean "check team" "$cli_out"
  if [[ -f "${record}/args" ]]; then bad "check invoked notarytool"; else ok "check does not call notarytool"; fi

  clear_env
  ASC_KEY_P8="$p8"
  ASC_KEY_ID="$key_id"
  APPLE_TEAM_ID="$team"
  run_cli check
  if [[ "$cli_rc" == 0 && "$cli_out" == *"notary-credentials: individual"* && "$cli_out" == *"does not invent an issuer"* && "$cli_out" == *"not able to use notarytool"* ]]; then
    ok "check individual key omits issuer"
  else
    bad "check individual key rc=$cli_rc out=$cli_out"
  fi
  assert_clean "check individual" "$cli_out"

  clear_env
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  run_cli check
  if [[ "$cli_rc" == 0 && "$cli_out" == *"notary-credentials: apple-id"* ]]; then
    ok "check apple id fallback"
  else
    bad "check apple id rc=$cli_rc out=$cli_out"
  fi
  assert_clean "check apple id" "$cli_out"

  clear_env
  ASC_KEY_P8="$p8"
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  run_cli check
  if [[ "$cli_rc" != 0 && "$cli_out" == *"missing ASC_KEY_ID"* && "$cli_out" != *"notary-credentials:"* ]]; then
    ok "partial API key does not fall back"
  else
    bad "partial API key rc=$cli_rc out=$cli_out"
  fi
  assert_clean "partial API key" "$cli_out"

  clear_env
  ASC_ISSUER_ID="$issuer"
  run_cli check
  if [[ "$cli_rc" != 0 && "$cli_out" == *"missing ASC_KEY_P8 ASC_KEY_ID"* && "$cli_out" == *"does not invent an issuer"* ]]; then
    ok "issuer alone is an incomplete team key"
  else
    bad "issuer alone rc=$cli_rc out=$cli_out"
  fi
  assert_clean "issuer alone" "$cli_out"

  clear_env
  ASC_KEY_ID="$key_id"
  ASC_ISSUER_ID="$issuer"
  run_cli check
  if [[ "$cli_rc" != 0 && "$cli_out" == *"missing ASC_KEY_P8"* ]]; then
    ok "team key missing the private key"
  else
    bad "missing p8 rc=$cli_rc out=$cli_out"
  fi
  assert_clean "missing p8" "$cli_out"

  clear_env
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  run_cli check
  if [[ "$cli_rc" != 0 && "$cli_out" == *"missing APPLE_APP_PASSWORD"* ]]; then
    ok "partial Apple ID fails"
  else
    bad "partial Apple ID rc=$cli_rc out=$cli_out"
  fi
  assert_clean "partial Apple ID" "$cli_out"

  clear_env
  run_cli check
  if [[ "$cli_rc" != 0 && "$cli_out" == *"No notarization credentials"* && "$cli_out" == *"ASC_KEY_P8"* && "$cli_out" == *"APPLE_ID"* ]]; then
    ok "no credentials fails"
  else
    bad "no credentials rc=$cli_rc out=$cli_out"
  fi

  clear_env
  ASC_KEY_P8=""
  ASC_KEY_ID=""
  ASC_ISSUER_ID=""
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  run_cli check
  if [[ "$cli_rc" == 0 && "$cli_out" == *"notary-credentials: apple-id"* ]]; then
    ok "empty API key secrets use Apple ID"
  else
    bad "empty API secrets rc=$cli_rc out=$cli_out"
  fi
  assert_clean "empty API secrets" "$cli_out"

  # store: team key wins over a complete Apple ID, writes raw PEM mode 600, deletes it.
  clear_env
  reset_record
  ASC_KEY_P8="$p8"
  ASC_KEY_ID="$key_id"
  ASC_ISSUER_ID="$issuer"
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  KEYCHAIN="${work}/signing.keychain-db"
  run_cli store
  if [[ "$cli_rc" == 0 && "$cli_out" == *"notary-credentials: team"* && "$cli_out" == *"Stored notary credentials (team) in profile notary."* ]]; then
    ok "store team key"
  else
    bad "store team key rc=$cli_rc out=$cli_out"
  fi
  assert_clean "store team" "$cli_out"
  if has_arg --key && has_arg --key-id && has_arg --issuer && ! has_arg --apple-id && ! has_arg --password; then
    ok "team key uses --key --key-id --issuer"
  else
    bad "team key arguments were wrong"
  fi
  if [[ "$(value_after --key-id)" == "$key_id" && "$(value_after --issuer)" == "$issuer" && "$(value_after --keychain)" == "${work}/signing.keychain-db" ]]; then
    ok "team key passes the provided issuer"
  else
    bad "team key issuer or keychain was wrong"
  fi
  if [[ "$(tr '\0' '\n' <"${record}/args")" == *"TEAMIDTEST"* ]]; then
    bad "team key command included the Developer Team ID"
  else
    ok "team key does not substitute the Developer Team ID"
  fi
  if [[ "$(cat "${record}/key.mode" 2>/dev/null || true)" == 600 && "$(cat "${record}/dir.mode" 2>/dev/null || true)" == 700 ]]; then
    ok "private key file is mode 600"
  else
    bad "private key mode was not 600"
  fi
  if cmp -s "${work}/p8.pem" "${record}/key.p8"; then
    ok "private key is stored as raw PEM"
  else
    bad "private key bytes were changed"
  fi
  no_key_left "store team success"

  # No trailing newline in the secret: the file still ends with one newline.
  clear_env
  reset_record
  ASC_KEY_P8="${p8%$'\n'}"
  ASC_KEY_ID="$key_id"
  ASC_ISSUER_ID="$issuer"
  KEYCHAIN="${work}/signing.keychain-db"
  run_cli store
  if [[ "$cli_rc" == 0 ]] && cmp -s "${work}/p8.pem" "${record}/key.p8"; then
    ok "private key gains a trailing newline"
  else
    bad "trailing newline rc=$cli_rc"
  fi
  assert_clean "trailing newline" "$cli_out"
  no_key_left "store newline"

  # Individual key: omit --issuer even if a Developer Team ID is present.
  clear_env
  reset_record
  ASC_KEY_P8="$p8"
  ASC_KEY_ID="$key_id"
  APPLE_TEAM_ID="$team"
  APPLE_ID="$apple"
  KEYCHAIN="${work}/signing.keychain-db"
  run_cli store
  if [[ "$cli_rc" == 0 ]] && has_arg --key && has_arg --key-id && ! has_arg --issuer && ! has_arg --apple-id; then
    ok "individual key omits --issuer"
  else
    bad "individual store rc=$cli_rc"
  fi
  if [[ "$(tr '\0' '\n' <"${record}/args")" == *"$team"* || "$(tr '\0' '\n' <"${record}/args")" == *"$apple"* ]]; then
    bad "individual key command invented an issuer or used Apple ID"
  else
    ok "individual key does not invent an issuer"
  fi
  assert_clean "store individual" "$cli_out"
  no_key_left "store individual"

  # notarytool failure still deletes the key file and fails the script.
  clear_env
  reset_record
  ASC_KEY_P8="$p8"
  ASC_KEY_ID="$key_id"
  ASC_ISSUER_ID="$issuer"
  KEYCHAIN="${work}/signing.keychain-db"
  NOTARY_FAIL=1
  run_cli store
  if [[ "$cli_rc" != 0 ]]; then
    ok "notarytool failure fails the script"
  else
    bad "notarytool failure was ignored"
  fi
  assert_clean "notarytool failure" "$cli_out"
  no_key_left "store failure"
  if [[ ! -f "${record}/key.p8" ]]; then
    bad "notarytool was not given the key file"
  fi

  # Apple ID fallback, and a missing keychain, do not write a key file.
  clear_env
  reset_record
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  KEYCHAIN="${work}/signing.keychain-db"
  NOTARY_PROFILE=notary
  run_cli store
  if [[ "$cli_rc" == 0 ]] && has_arg --apple-id && has_arg --team-id && has_arg --password && ! has_arg --key; then
    ok "apple id store-credentials"
  else
    bad "apple id store rc=$cli_rc"
  fi
  if [[ "$(value_after --apple-id)" == "$apple" && "$(value_after --team-id)" == "$team" && "$(value_after --password)" == "$pass" && "$(value_after store-credentials)" == notary ]]; then
    ok "apple id arguments match the profile"
  else
    bad "apple id arguments were wrong"
  fi
  assert_clean "store apple id" "$cli_out"
  no_key_left "store apple id"

  clear_env
  reset_record
  ASC_KEY_P8="$p8"
  ASC_KEY_ID="$key_id"
  ASC_ISSUER_ID="$issuer"
  run_cli store
  if [[ "$cli_rc" != 0 && "$cli_out" == *"KEYCHAIN is not set"* && ! -f "${record}/args" ]]; then
    ok "store without a keychain does not call notarytool"
  else
    bad "missing keychain rc=$cli_rc out=$cli_out"
  fi
  assert_clean "missing keychain" "$cli_out"
  no_key_left "missing keychain"

  clear_env
  reset_record
  ASC_KEY_P8="$p8"
  APPLE_ID="$apple"
  APPLE_TEAM_ID="$team"
  APPLE_APP_PASSWORD="$pass"
  KEYCHAIN="${work}/signing.keychain-db"
  run_cli store
  if [[ "$cli_rc" != 0 && ! -f "${record}/args" ]]; then
    ok "incomplete API key does not call notarytool"
  else
    bad "incomplete store called notarytool rc=$cli_rc"
  fi
  no_key_left "incomplete store"

  # Static: workflow wiring and the release docs.
  local workflow="${root}/.github/workflows/release.yml"
  local docs="${root}/docs/releasing.md"
  local needle
  for needle in \
    "secrets.ASC_KEY_P8" \
    "secrets.ASC_KEY_ID" \
    "secrets.ASC_ISSUER_ID" \
    "secrets.APPLE_ID" \
    "secrets.APPLE_TEAM_ID" \
    "secrets.APPLE_APP_PASSWORD" \
    "secrets.MACOS_CERTIFICATE" \
    "secrets.MYGO_UPDATER_PRIVATE_KEY" \
    "macos-notary-credentials.sh check" \
    "macos-notary-credentials.sh store" \
    "Developer ID Application" \
    "signingIdentity" \
    "UPLOAD=1"; do
    if ! grep -qF -- "$needle" "$workflow"; then
      bad "release.yml missing ${needle}"
    fi
  done
  impl="$(sed '/^self_test()/,$d' "$script")"
  if [[ "$impl" == *base64* ]]; then
    bad "credential script must not base64 the API key"
  fi
  for needle in \
    "ASC_KEY_P8" \
    "ASC_KEY_ID" \
    "ASC_ISSUER_ID" \
    "not base64" \
    "does not invent an issuer" \
    "Individual keys are not able to use notarytool" \
    "removed on success and on failure" \
    "mode 600" \
    "APPLE_APP_PASSWORD"; do
    if ! grep -qF -- "$needle" "$docs"; then
      bad "releasing.md missing ${needle}"
    fi
  done
  if [[ "$fail" == 0 ]]; then
    ok "workflow and docs name the credentials"
  fi

  rm -rf "$work"
  if [[ "$fail" == 0 ]]; then
    echo "macos-notary-credentials.sh: examples pass"
  fi
  return "$fail"
}

usage() {
  sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
  check) cmd_check ;;
  store) cmd_store ;;
  --self-test) self_test ;;
  -h | --help | "") usage; exit 2 ;;
  *)
    echo "unknown command: $1" >&2
    usage >&2
    exit 2
    ;;
esac
