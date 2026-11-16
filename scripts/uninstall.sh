#!/usr/bin/env bash
# Remove local aTerm artifacts so the next launch behaves like a
# first-run test. It intentionally preserves iCloud sync data, user-managed
# SSH entries, and any unmarked Include directives in ~/.ssh/config.
set -euo pipefail

readonly APP_NAME="aTerm"
readonly BUNDLE_ID="pro.huabei.sshconfig"
readonly MANAGED_INCLUDE_COMMENT="# Added by aTerm"
readonly MANAGED_INCLUDE_LINE="Include ~/.ssh/config.d/*.conf"

usage() {
  cat <<'EOF'
Usage: scripts/uninstall.sh [--yes]

Uninstalls aTerm from /Applications and removes its local data for
the current macOS user. iCloud sync data is preserved. Use --yes to skip the
confirmation prompt.
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi
if [[ $# -gt 1 || ( $# -eq 1 && "${1:-}" != "--yes" ) ]]; then
  usage >&2
  exit 2
fi

if [[ "$(id -u)" -eq 0 ]]; then
  echo "Run this script as the macOS user whose test data should be removed, not as root." >&2
  exit 1
fi

if [[ "${1:-}" != "--yes" ]]; then
  read -r -p "Delete ${APP_NAME}, its local data, and managed SSH config for ${USER}? [y/N] " answer
  if [[ ! "$answer" =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
  fi
fi

remove_path() {
  local path="$1"
  if [[ -e "$path" || -L "$path" ]]; then
    rm -rf -- "$path"
    echo "Removed: $path"
  fi
}

remove_installed_app() {
  local app_path="/Applications/${APP_NAME}.app"
  [[ -e "$app_path" || -L "$app_path" ]] || return 0

  # Applications installed by macOS or the App Store are usually owned by
  # root. Keep the rest of this script in the invoking user's context, but
  # request administrator permission for this one protected path.
  if [[ -w "$app_path" && -w "/Applications" ]]; then
    rm -rf -- "$app_path"
  else
    echo "Administrator permission is required to remove: $app_path"
    sudo rm -rf -- "$app_path"
  fi
  echo "Removed: $app_path"
}

remove_managed_include_block() {
  local config_path="$1"
  [[ -f "$config_path" ]] || return 0

  local temporary_path
  temporary_path="$(mktemp "${config_path}.ssh-connect-uninstall.XXXXXX")"
  awk -v comment="$MANAGED_INCLUDE_COMMENT" -v include="$MANAGED_INCLUDE_LINE" '
    function trimmed(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }
    {
      current = trimmed($0)
      if (current == comment) {
        if (pending != "") print pending
        pending = $0
        next
      }
      if (pending != "") {
        if (current == include) {
          pending = ""
          changed = 1
          next
        }
        print pending
        pending = ""
      }
      print
    }
    END {
      if (pending != "") print pending
    }
  ' "$config_path" > "$temporary_path"

  if cmp -s "$config_path" "$temporary_path"; then
    rm -f -- "$temporary_path"
    return 0
  fi
  chmod --reference="$config_path" "$temporary_path" 2>/dev/null || chmod 600 "$temporary_path"
  mv -- "$temporary_path" "$config_path"
  echo "Removed the managed SSH config Include block from: $config_path"
}

if pgrep -x "${APP_NAME}" >/dev/null 2>&1; then
  echo "${APP_NAME} is still running. Quit it, then run this script again." >&2
  exit 1
fi

readonly HOME_DIR="$HOME"
readonly SSH_DIR="${HOME_DIR}/.ssh"
readonly SANDBOX_DATA_DIR="${HOME_DIR}/Library/Containers/${BUNDLE_ID}/Data"

remove_managed_include_block "${SSH_DIR}/config"
remove_path "${SSH_DIR}/config.d/ssh-connect.generated.conf"
for backup in "${SSH_DIR}"/config.ssh-connect.*.bak; do
  [[ -e "$backup" ]] || continue
  remove_path "$backup"
done

remove_installed_app
remove_path "${HOME_DIR}/Library/Application Support/${APP_NAME}"
# Do not remove the container root: macOS owns its
# .com.apple.containermanagerd.metadata.plist and rejects direct removal from
# a normal terminal. Removing these payload paths resets every application
# setting and data item while leaving only that OS-managed metadata behind.
remove_path "${SANDBOX_DATA_DIR}/Library/Application Support/${APP_NAME}"
remove_path "${SANDBOX_DATA_DIR}/Library/Caches/${BUNDLE_ID}"
remove_path "${SANDBOX_DATA_DIR}/Library/Preferences/${BUNDLE_ID}.plist"
remove_path "${SANDBOX_DATA_DIR}/Library/Saved Application State/${BUNDLE_ID}.savedState"
remove_path "${HOME_DIR}/Library/Caches/${BUNDLE_ID}"
remove_path "${HOME_DIR}/Library/Preferences/${BUNDLE_ID}.plist"
remove_path "${HOME_DIR}/Library/Saved Application State/${BUNDLE_ID}.savedState"

echo "${APP_NAME} has been removed. The next install will start with no app data."
