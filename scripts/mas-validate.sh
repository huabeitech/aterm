#!/usr/bin/env bash
set -euo pipefail

APP_BUNDLE="${APP_BUNDLE:-build/bin/aTerm.app}"
SIGNING_DIR="${SIGNING_DIR:-$HOME/.config/aterm/signing}"
PROFILE_PATH="${PROVISIONING_PROFILE:-$SIGNING_DIR/embedded.provisionprofile}"
PLISTBUDDY="${PLISTBUDDY:-/usr/libexec/PlistBuddy}"

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

[[ -d "$APP_BUNDLE" ]] || die "missing app bundle: $APP_BUNDLE"
[[ -f "$PROFILE_PATH" ]] || die "missing provisioning profile: $PROFILE_PATH"

INFO_PLIST="$APP_BUNDLE/Contents/Info.plist"
EXECUTABLE_NAME="$("$PLISTBUDDY" -c 'Print :CFBundleExecutable' "$INFO_PLIST")"
EXECUTABLE_PATH="$APP_BUNDLE/Contents/MacOS/$EXECUTABLE_NAME"
MIN_SYSTEM_VERSION="$("$PLISTBUDDY" -c 'Print :LSMinimumSystemVersion' "$INFO_PLIST")"
SHORT_VERSION="$("$PLISTBUDDY" -c 'Print :CFBundleShortVersionString' "$INFO_PLIST")"
BUNDLE_VERSION="$("$PLISTBUDDY" -c 'Print :CFBundleVersion' "$INFO_PLIST")"

PROFILE_PLIST="$(mktemp)"
SIGNED_ENTITLEMENTS="$(mktemp)"
cleanup() {
  rm -f "$PROFILE_PLIST" "$SIGNED_ENTITLEMENTS"
}
trap cleanup EXIT

security cms -D -i "$PROFILE_PATH" >"$PROFILE_PLIST"
codesign -d --entitlements :- "$APP_BUNDLE" >"$SIGNED_ENTITLEMENTS" 2>/dev/null

PROFILE_APP_ID="$("$PLISTBUDDY" -c 'Print :Entitlements:com.apple.application-identifier' "$PROFILE_PLIST")"
SIGNED_APP_ID="$("$PLISTBUDDY" -c 'Print :com.apple.application-identifier' "$SIGNED_ENTITLEMENTS" 2>/dev/null || true)"

if [[ "$SIGNED_APP_ID" != "$PROFILE_APP_ID" ]]; then
  die "signed application identifier '$SIGNED_APP_ID' does not match provisioning profile '$PROFILE_APP_ID'"
fi

PROFILE_TEAM_ID="$("$PLISTBUDDY" -c 'Print :Entitlements:com.apple.developer.team-identifier' "$PROFILE_PLIST")"
SIGNED_TEAM_ID="$("$PLISTBUDDY" -c 'Print :com.apple.developer.team-identifier' "$SIGNED_ENTITLEMENTS" 2>/dev/null || true)"

if [[ "$SIGNED_TEAM_ID" != "$PROFILE_TEAM_ID" ]]; then
  die "signed team identifier '$SIGNED_TEAM_ID' does not match provisioning profile '$PROFILE_TEAM_ID'"
fi

SIGNED_APP_SANDBOX="$("$PLISTBUDDY" -c 'Print :com.apple.security.app-sandbox' "$SIGNED_ENTITLEMENTS" 2>/dev/null || true)"
if [[ "$SIGNED_APP_SANDBOX" != "true" ]]; then
  die "signed app is missing com.apple.security.app-sandbox"
fi

SIGNED_NETWORK_CLIENT="$("$PLISTBUDDY" -c 'Print :com.apple.security.network.client' "$SIGNED_ENTITLEMENTS" 2>/dev/null || true)"
if [[ "$SIGNED_NETWORK_CLIENT" != "true" ]]; then
  die "signed app is missing com.apple.security.network.client"
fi

SIGNED_APP_SCOPE_BOOKMARKS="$("$PLISTBUDDY" -c 'Print :com.apple.security.files.bookmarks.app-scope' "$SIGNED_ENTITLEMENTS" 2>/dev/null || true)"
if [[ "$SIGNED_APP_SCOPE_BOOKMARKS" != "true" ]]; then
  die "signed app is missing com.apple.security.files.bookmarks.app-scope"
fi

SIGNED_APPLE_EVENTS="$("$PLISTBUDDY" -c 'Print :com.apple.security.automation.apple-events' "$SIGNED_ENTITLEMENTS" 2>/dev/null || true)"
if [[ -n "$SIGNED_APPLE_EVENTS" ]]; then
  die "signed MAS app should not include com.apple.security.automation.apple-events"
fi
APPLE_EVENTS_EXCEPTION_KEY="com.apple.security.temporary-exception.apple-events"
if "$PLISTBUDDY" -c "Print :$APPLE_EVENTS_EXCEPTION_KEY" "$SIGNED_ENTITLEMENTS" >/dev/null 2>&1; then
  die "signed MAS app should not include Apple Events temporary exception entitlement"
fi

ALLOWS_LOCAL_NETWORKING="$("$PLISTBUDDY" -c 'Print :NSAppTransportSecurity:NSAllowsLocalNetworking' "$INFO_PLIST" 2>/dev/null || true)"
if [[ "$ALLOWS_LOCAL_NETWORKING" != "true" ]]; then
  die "Info.plist is missing NSAppTransportSecurity:NSAllowsLocalNetworking"
fi

ARCHS="$(lipo -archs "$EXECUTABLE_PATH")"
if [[ "$ARCHS" != *x86_64* && "$MIN_SYSTEM_VERSION" < "12.0" ]]; then
  die "arm64-only build requires LSMinimumSystemVersion 12.0 or newer; current value is $MIN_SYSTEM_VERSION"
fi
printf 'MAS validation passed.\n'
printf 'Application identifier: %s\n' "$SIGNED_APP_ID"
printf 'Team identifier: %s\n' "$SIGNED_TEAM_ID"
printf 'Architectures: %s\n' "$ARCHS"
printf 'LSMinimumSystemVersion: %s\n' "$MIN_SYSTEM_VERSION"
printf 'CFBundleShortVersionString: %s\n' "$SHORT_VERSION"
printf 'CFBundleVersion: %s\n' "$BUNDLE_VERSION"
printf 'App Sandbox: %s\n' "$SIGNED_APP_SANDBOX"
printf 'Network client entitlement: %s\n' "$SIGNED_NETWORK_CLIENT"
printf 'App-scope bookmarks entitlement: %s\n' "$SIGNED_APP_SCOPE_BOOKMARKS"
printf 'Apple Events entitlement: not present\n'
printf 'Allows local networking: %s\n' "$ALLOWS_LOCAL_NETWORKING"
