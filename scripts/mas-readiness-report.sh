#!/usr/bin/env bash
set -euo pipefail

APP_BUNDLE="${APP_BUNDLE:-build/bin/aTerm.app}"
PKG_PATH="${PKG_PATH:-build/bin/aTerm.pkg}"
SCREENSHOT_DIR="${SCREENSHOT_DIR:-docs/app-store/screenshots}"
PLISTBUDDY="${PLISTBUDDY:-/usr/libexec/PlistBuddy}"

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

require_tool() {
  command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"
}

status_for_url() {
  local url="$1"
  curl -L -s -o /dev/null -w '%{http_code}' "$url"
}

screenshot_dimensions() {
  local file="$1"
  sips -g pixelWidth -g pixelHeight "$file" 2>/dev/null \
    | awk '/pixelWidth/{w=$2}/pixelHeight/{h=$2} END{print w "x" h}'
}

require_tool curl
require_tool sips
require_tool codesign
require_tool pkgutil
require_tool lipo

[[ -d "$APP_BUNDLE" ]] || die "missing app bundle: $APP_BUNDLE"
[[ -f "$PKG_PATH" ]] || die "missing package: $PKG_PATH"
[[ -d "$SCREENSHOT_DIR" ]] || die "missing screenshot directory: $SCREENSHOT_DIR"

INFO_PLIST="$APP_BUNDLE/Contents/Info.plist"
EXECUTABLE_NAME="$("$PLISTBUDDY" -c 'Print :CFBundleExecutable' "$INFO_PLIST")"
EXECUTABLE_PATH="$APP_BUNDLE/Contents/MacOS/$EXECUTABLE_NAME"

printf '# aTerm App Store Readiness Report\n\n'
printf 'Generated: %s UTC\n\n' "$(date -u '+%Y-%m-%d %H:%M:%S')"

printf '## Bundle\n\n'
printf -- '- Bundle ID: `%s`\n' "$("$PLISTBUDDY" -c 'Print :CFBundleIdentifier' "$INFO_PLIST")"
printf -- '- Version: `%s`\n' "$("$PLISTBUDDY" -c 'Print :CFBundleShortVersionString' "$INFO_PLIST")"
printf -- '- Build: `%s`\n' "$("$PLISTBUDDY" -c 'Print :CFBundleVersion' "$INFO_PLIST")"
printf -- '- Minimum macOS: `%s`\n' "$("$PLISTBUDDY" -c 'Print :LSMinimumSystemVersion' "$INFO_PLIST")"
printf -- '- Category: `%s`\n' "$("$PLISTBUDDY" -c 'Print :LSApplicationCategoryType' "$INFO_PLIST")"
printf -- '- Non-exempt encryption: `%s`\n' "$("$PLISTBUDDY" -c 'Print :ITSAppUsesNonExemptEncryption' "$INFO_PLIST")"
printf -- '- Architectures: `%s`\n\n' "$(lipo -archs "$EXECUTABLE_PATH")"

printf '## Package\n\n'
printf -- '- Path: `%s`\n' "$PKG_PATH"
printf -- '- Size: `%s`\n' "$(du -h "$PKG_PATH" | awk '{print $1}')"
printf -- '- Signature:\n\n'
pkgutil --check-signature "$PKG_PATH" | sed 's/^/  /'
printf '\n'

printf '## Legal URLs\n\n'
for url in \
  'https://huabei.pro/privacy/ssh-config-manager/' \
  'https://huabei.pro/terms/ssh-config-manager/' \
  'https://huabei.pro/support/ssh-config-manager/'
do
  printf -- '- `%s` -> `%s`\n' "$url" "$(status_for_url "$url")"
done
printf '\n'

printf '## Screenshots\n\n'
while IFS= read -r -d '' file; do
  printf -- '- `%s` -> `%s`\n' "$file" "$(screenshot_dimensions "$file")"
done < <(find "$SCREENSHOT_DIR" -maxdepth 1 -type f -name '*.png' -print0 | sort -z)
printf '\n'

printf '## Submission Files\n\n'
for file in \
  'docs/app-store/connect-submission-packet.md' \
  'docs/app-store-submission-plan.md' \
  'docs/app-store-readiness.md'
do
  if [[ -f "$file" ]]; then
    printf -- '- `%s` present\n' "$file"
  else
    printf -- '- `%s` missing\n' "$file"
  fi
done
