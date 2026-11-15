#!/usr/bin/env bash
set -euo pipefail

APP_BUNDLE="${APP_BUNDLE:-build/bin/aTerm.app}"
PKG_PATH="${PKG_PATH:-build/bin/aTerm.pkg}"
ENTITLEMENTS="${ENTITLEMENTS:-build/darwin/entitlements.appstore.plist}"
SIGNING_DIR="${SIGNING_DIR:-$HOME/.config/aterm/signing}"
PROFILE_PATH="${PROVISIONING_PROFILE:-}"
KEYCHAIN="${KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}"
SIGNING_KEY="${SIGNING_KEY:-$SIGNING_DIR/SSH_Config_Manager_MAS.key}"
P12_PASSWORD="${P12_PASSWORD:-$(openssl rand -hex 24)}"
WWDR_CERT="${WWDR_CERT:-$SIGNING_DIR/AppleWWDRCAG3.cer}"
OPEN_TRANSPORTER=0
SKIP_BUILD=0
SKIP_PROFILE=0

usage() {
  cat <<'EOF'
Usage:
  scripts/mas-package.sh [options]

Build, sign, and package aTerm for Mac App Store upload.

Options:
  --skip-build          Use the existing .app bundle instead of running wails3 task mas:build.
  --skip-profile        Do not embed a provisioning profile.
  --open-transporter    Open the generated .pkg in Transporter after packaging.
  -h, --help            Show this help.

Environment overrides:
  APPSTORE_IDENTITY       App signing identity.
                          Example: 3rd Party Mac Developer Application: Your Team (TEAMID)
  INSTALLER_IDENTITY      Installer package signing identity.
                          Example: 3rd Party Mac Developer Installer: Your Team (TEAMID)
  PROVISIONING_PROFILE    Path to the Mac App Store provisioning profile.
  KEYCHAIN                Default: ~/Library/Keychains/login.keychain-db
  SIGNING_DIR             Default: ~/.config/aterm/signing
  SIGNING_KEY             Default: $SIGNING_DIR/SSH_Config_Manager_MAS.key
  P12_PASSWORD            Optional password for temporary .p12 imports; random by default.
  WWDR_CERT               Default: $SIGNING_DIR/AppleWWDRCAG3.cer
  APP_BUNDLE              Default: build/bin/aTerm.app
  PKG_PATH                Default: build/bin/aTerm.pkg
  ENTITLEMENTS            Default: build/darwin/entitlements.appstore.plist

Local signing material:
  If SIGNING_KEY exists, the script imports it into KEYCHAIN before signing.
  If SIGNING_DIR/*.cer files exist, the script imports them into KEYCHAIN.
  If SIGNING_KEY and SIGNING_DIR/*.cer files exist, the script also creates
  temporary .p12 files and imports them so Keychain can form signing identities.

Provisioning profile lookup order:
  1. PROVISIONING_PROFILE environment variable
  2. $SIGNING_DIR/embedded.provisionprofile

After success, upload the generated .pkg with Transporter.
EOF
}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

while (($# > 0)); do
  case "$1" in
    --skip-build)
      SKIP_BUILD=1
      ;;
    --skip-profile)
      SKIP_PROFILE=1
      ;;
    --open-transporter)
      OPEN_TRANSPORTER=1
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

require_tool() {
  command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"
}

matching_identity() {
  local pattern="$1"
  local requested="${2:-}"
  local cert_path="${3:-}"
  local identities fingerprint
  identities="$(security find-identity -v "$KEYCHAIN" 2>/dev/null)"

  # Prefer the certificate supplied with this project's signing material.
  if [[ -z "$requested" && -f "$cert_path" ]]; then
    fingerprint="$(openssl x509 -inform DER -in "$cert_path" -noout -fingerprint -sha1 | sed 's/.*=//;s/://g')"
    requested="$fingerprint"
  fi

  printf '%s\n' "$identities" | awk -v pattern="$pattern" -v requested="$requested" '
    /^[[:space:]]*[0-9]+\)/ {
      hash = $2
      name = $0
      sub(/^[^"]*"/, "", name)
      sub(/"[[:space:]]*$/, "", name)
      if (name ~ pattern && (requested == "" || name == requested || toupper(hash) == toupper(requested))) {
        if (!seen[hash]++) { selected = hash; count++ }
      }
    }
    END { if (count == 1) print selected; else exit 1 }
  '
}

require_identity() {
  local value="$1"
  local env_name="$2"
  local pattern="$3"
  local cert_path="$4"
  local detected
  detected="$(matching_identity "$pattern" "$value" "$cert_path" || true)"
  if [[ -n "$detected" ]]; then
    printf '%s' "$detected"
    return
  fi
  printf 'Error: cannot uniquely resolve %s in %s. Set it to a valid certificate SHA-1 fingerprint, or provide the matching certificate at %s.\n' "$env_name" "$KEYCHAIN" "$cert_path" >&2
  exit 1
}

find_profile() {
  if [[ -n "$PROFILE_PATH" ]]; then
    [[ -f "$PROFILE_PATH" ]] || die "PROVISIONING_PROFILE does not exist: $PROFILE_PATH"
    printf '%s' "$PROFILE_PATH"
    return
  fi

  local candidate
  for candidate in "$SIGNING_DIR/embedded.provisionprofile"; do
    if [[ -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return
    fi
  done

  return 1
}

sync_bundle_metadata() {
  local info_plist="$APP_BUNDLE/Contents/Info.plist"
  log "Syncing bundle metadata from build/config.yml..."
  scripts/sync-build-metadata.py --plist "$info_plist"
}

increment_release_metadata() {
  local release_metadata
  release_metadata="$(scripts/increment-release-metadata.py)"
  log "Incremented build/config.yml info.version and info.buildNumber to $release_metadata"
}

clean_package_outputs() {
  log "Cleaning previous package output: $PKG_PATH"
  rm -f "$PKG_PATH"

  if [[ "$SKIP_BUILD" -eq 0 ]]; then
    log "Cleaning previous app bundle output: $APP_BUNDLE"
    rm -rf "$APP_BUNDLE"
  fi
}

plist_set_or_add_string() {
  local plist_path="$1"
  local key="$2"
  local value="$3"

  if /usr/libexec/PlistBuddy -c "Print :$key" "$plist_path" >/dev/null 2>&1; then
    /usr/libexec/PlistBuddy -c "Set :$key $value" "$plist_path"
  else
    /usr/libexec/PlistBuddy -c "Add :$key string $value" "$plist_path"
  fi
}

build_signing_entitlements() {
  local output_path="$1"
  cp "$ENTITLEMENTS" "$output_path"

  if [[ -z "${PROFILE_RESOLVED:-}" ]]; then
    return
  fi

  local profile_plist
  profile_plist="$TMP_DIR/profile.plist"
  security cms -D -i "$PROFILE_RESOLVED" >"$profile_plist"

  local app_identifier
  local team_identifier
  app_identifier="$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.application-identifier' "$profile_plist")"
  team_identifier="$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.developer.team-identifier' "$profile_plist")"

  plist_set_or_add_string "$output_path" "com.apple.application-identifier" "$app_identifier"
  plist_set_or_add_string "$output_path" "com.apple.developer.team-identifier" "$team_identifier"
}

require_tool security
require_tool codesign
require_tool productbuild
require_tool wails3
require_tool openssl

[[ -f "$ENTITLEMENTS" ]] || die "missing entitlements file: $ENTITLEMENTS"

umask 077
TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [[ -f "$SIGNING_KEY" ]]; then
  log "Importing signing private key into Keychain..."
  security import "$SIGNING_KEY" \
    -k "$KEYCHAIN" \
    -T /usr/bin/codesign \
    -T /usr/bin/productbuild >/dev/null 2>&1 || true
fi

if [[ ! -f "$WWDR_CERT" ]]; then
  if command -v curl >/dev/null 2>&1; then
    log "Downloading Apple WWDR G3 intermediate certificate..."
    mkdir -p "$(dirname "$WWDR_CERT")"
    curl -fsSL -o "$WWDR_CERT" https://www.apple.com/certificateauthority/AppleWWDRCAG3.cer
  else
    log "curl is not available; skipping Apple WWDR G3 download."
  fi
fi

if [[ -f "$WWDR_CERT" ]]; then
  log "Importing Apple WWDR intermediate certificate into Keychain..."
  security add-certificates -k "$KEYCHAIN" "$WWDR_CERT" >/dev/null 2>&1 || true
fi

while IFS= read -r -d '' cert_file; do
  log "Importing certificate into Keychain: $cert_file"
  security import "$cert_file" -k "$KEYCHAIN" >/dev/null 2>&1 || true

  if [[ -f "$SIGNING_KEY" && "$cert_file" != "$WWDR_CERT" ]]; then
    cert_base="$(basename "$cert_file")"
    cert_pem="$TMP_DIR/${cert_base}.pem"
    cert_p12="$TMP_DIR/${cert_base}.p12"
    if openssl x509 -inform DER -in "$cert_file" -out "$cert_pem" >/dev/null 2>&1; then
      if openssl pkcs12 -legacy -export \
        -inkey "$SIGNING_KEY" \
        -in "$cert_pem" \
        -out "$cert_p12" \
        -passout "pass:${P12_PASSWORD}" >/dev/null 2>&1; then
        log "Importing signing identity into Keychain: $cert_file"
        security import "$cert_p12" \
          -k "$KEYCHAIN" \
          -P "$P12_PASSWORD" \
          -T /usr/bin/codesign \
          -T /usr/bin/productbuild >/dev/null 2>&1 || true
      fi
    fi
  fi
done < <(find "$SIGNING_DIR" -maxdepth 1 -type f \( -name '*.cer' -o -name '*.cert' \) -print0 2>/dev/null)

APPSTORE_IDENTITY_RESOLVED="$(
  require_identity \
    "${APPSTORE_IDENTITY:-}" \
    "APPSTORE_IDENTITY" \
    '^(3rd Party Mac Developer Application:|Apple Distribution:)' \
    "$SIGNING_DIR/distribution.cer"
)"

INSTALLER_IDENTITY_RESOLVED="$(
  require_identity \
    "${INSTALLER_IDENTITY:-}" \
    "INSTALLER_IDENTITY" \
    '^(3rd Party Mac Developer Installer:|Mac Installer Distribution:)' \
    "$SIGNING_DIR/mac_installer.cer"
)"

if [[ "$SKIP_PROFILE" -eq 0 ]]; then
  PROFILE_RESOLVED="$(find_profile || true)"
  if [[ -z "$PROFILE_RESOLVED" ]]; then
    cat >&2 <<'EOF'
Error: no Mac App Store provisioning profile was found.

Download a Mac App Store Distribution provisioning profile for bundle id
pro.huabei.sshconfig, then either:

  1. Put it at ~/.config/aterm/signing/embedded.provisionprofile
     (or $SIGNING_DIR/embedded.provisionprofile when SIGNING_DIR is set)
  2. Or run with PROVISIONING_PROFILE=/path/to/profile.provisionprofile

Use --skip-profile only for local packaging experiments.
EOF
    exit 1
  fi
fi

clean_package_outputs
increment_release_metadata

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  log "Building app bundle..."
  wails3 task build EXTRA_TAGS=mas
else
  log "Using existing app bundle..."
fi

[[ -d "$APP_BUNDLE" ]] || die "missing app bundle: $APP_BUNDLE"
sync_bundle_metadata

if [[ "$SKIP_PROFILE" -eq 0 ]]; then
  log "Embedding provisioning profile: $PROFILE_RESOLVED"
  cp "$PROFILE_RESOLVED" "$APP_BUNDLE/Contents/embedded.provisionprofile"
else
  log "Skipping provisioning profile embedding."
fi

log "Signing app bundle..."
SIGNING_ENTITLEMENTS="$TMP_DIR/entitlements.mas.resolved.plist"
build_signing_entitlements "$SIGNING_ENTITLEMENTS"

codesign \
  --force \
  --options runtime \
  --timestamp \
  --entitlements "$SIGNING_ENTITLEMENTS" \
  --keychain "$KEYCHAIN" \
  --sign "$APPSTORE_IDENTITY_RESOLVED" \
  "$APP_BUNDLE"

log "Verifying app signature..."
codesign --verify --deep --strict --verbose=4 "$APP_BUNDLE"
if [[ -x scripts/mas-validate.sh ]]; then
  log "Validating Mac App Store metadata..."
  APP_BUNDLE="$APP_BUNDLE" PROVISIONING_PROFILE="${PROFILE_RESOLVED:-$PROFILE_PATH}" scripts/mas-validate.sh
fi

log "Building signed installer package..."
productbuild \
  --component "$APP_BUNDLE" /Applications \
  --keychain "$KEYCHAIN" \
  --sign "$INSTALLER_IDENTITY_RESOLVED" \
  "$PKG_PATH"

log "Verifying installer package..."
pkgutil --check-signature "$PKG_PATH"

log "Done: $PKG_PATH"
log "Upload this .pkg with Transporter."

if [[ "$OPEN_TRANSPORTER" -eq 1 ]]; then
  log "Opening Transporter..."
  open -a Transporter "$PKG_PATH"
fi
