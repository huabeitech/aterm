#!/usr/bin/env bash
set -euo pipefail

PKG_PATH="${PKG_PATH:-build/bin/aTerm.pkg}"
TRANSPORTER="${TRANSPORTER:-/Applications/Transporter.app/Contents/itms/bin/iTMSTransporter}"
LOG_DIR="${LOG_DIR:-build/transporter-logs}"
MODE="upload"

usage() {
  cat <<'EOF'
Usage:
  scripts/mas-transporter.sh [--verify|--upload]

Upload or verify the Mac App Store .pkg with Apple's iTMSTransporter CLI.

Options:
  --verify      Run Transporter verification only.
  --upload      Upload the .pkg to App Store Connect. This is the default.
  -h, --help    Show this help.

Environment:
  PKG_PATH            Default: build/bin/aTerm.pkg
  TRANSPORTER         Default: /Applications/Transporter.app/Contents/itms/bin/iTMSTransporter
  LOG_DIR             Default: build/transporter-logs

Authentication, choose one:

  App Store Connect API Key, recommended:
    ASC_API_KEY       API Key ID.
    ASC_API_ISSUER    Issuer ID.
    ASC_API_KEY_TYPE  Optional: team or individual. Default: team.

    The private key must be named AuthKey_<ASC_API_KEY>.p8 for a team key
    or ApiKey_<ASC_API_KEY>.p8 for an individual key, in one of:
      ./private_keys
      ~/private_keys
      ~/.private_keys
      ~/.appstoreconnect/private_keys

  Apple ID, interactive:
    APPLE_ID          Apple ID email. Transporter will prompt for password.
    ASC_PROVIDER      Optional provider short name if your account has multiple providers.

Examples:
  ASC_API_KEY=ABC123DEFG ASC_API_ISSUER=00000000-0000-0000-0000-000000000000 \
    scripts/mas-transporter.sh --upload

  APPLE_ID=name@example.com scripts/mas-transporter.sh --verify
EOF
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

while (($# > 0)); do
  case "$1" in
    --verify)
      MODE="verify"
      ;;
    --upload)
      MODE="upload"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
  shift
done

[[ -f "$PKG_PATH" ]] || die "missing package: $PKG_PATH"
[[ -x "$TRANSPORTER" ]] || die "missing executable Transporter CLI: $TRANSPORTER"

AUTH_ARGS=()
if [[ -n "${ASC_API_KEY:-}" || -n "${ASC_API_ISSUER:-}" ]]; then
  [[ -n "${ASC_API_KEY:-}" ]] || die "ASC_API_KEY is required when using API key authentication"
  [[ -n "${ASC_API_ISSUER:-}" ]] || die "ASC_API_ISSUER is required when using API key authentication"
  AUTH_ARGS=(-apiKey "$ASC_API_KEY" -apiIssuer "$ASC_API_ISSUER" -apiKeyType "${ASC_API_KEY_TYPE:-team}")
elif [[ -n "${APPLE_ID:-}" ]]; then
  AUTH_ARGS=(-u "$APPLE_ID" -p)
  if [[ -n "${ASC_PROVIDER:-}" ]]; then
    AUTH_ARGS+=(-asc_provider "$ASC_PROVIDER")
  fi
else
  cat >&2 <<'EOF'
Error: missing Transporter authentication.

Use App Store Connect API Key authentication:
  ASC_API_KEY=... ASC_API_ISSUER=... scripts/mas-transporter.sh --upload

Or interactive Apple ID authentication:
  APPLE_ID=name@example.com scripts/mas-transporter.sh --upload
EOF
  exit 1
fi

mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/${MODE}-$(date -u +%Y%m%dT%H%M%SZ).log"

if [[ "$MODE" == "verify" ]]; then
  "$TRANSPORTER" \
    -m verify \
    -f "$PKG_PATH" \
    "${AUTH_ARGS[@]}" \
    -v informational \
    -o "$LOG_FILE"
else
  "$TRANSPORTER" \
    -m upload \
    -assetFile "$PKG_PATH" \
    "${AUTH_ARGS[@]}" \
    -distribution AppStore \
    -t HTTP \
    -throughput \
    -v informational \
    -o "$LOG_FILE"
fi

printf 'Transporter %s log: %s\n' "$MODE" "$LOG_FILE"
