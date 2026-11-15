#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEMO_HOME="${1:-${REPO_ROOT}/build/appstore-demo-home}"
APP_SUPPORT_DIR="${DEMO_HOME}/Library/Application Support/TermSSH"
DB_PATH="${APP_SUPPORT_DIR}/app.db"
SSH_DIR="${DEMO_HOME}/.ssh"
NOW="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

mkdir -p "${APP_SUPPORT_DIR}" "${SSH_DIR}/config.d"
rm -f "${DB_PATH}"

sqlite3 "${DB_PATH}" <<SQL
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  directory_id TEXT NOT NULL DEFAULT '',
  alias TEXT NOT NULL UNIQUE,
  host_name TEXT NOT NULL,
  user TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 22,
  identity_file TEXT NOT NULL DEFAULT '',
  password_auth INTEGER NOT NULL DEFAULT 0,
  proxy_jump TEXT NOT NULL DEFAULT '',
  jump_host_id TEXT NOT NULL DEFAULT '',
  forward_agent INTEGER NOT NULL DEFAULT 0,
  compression INTEGER NOT NULL DEFAULT 0,
  server_alive_interval INTEGER NOT NULL DEFAULT 0,
  server_alive_count_max INTEGER NOT NULL DEFAULT 0,
  connect_timeout INTEGER NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  favorite INTEGER NOT NULL DEFAULT 0,
  last_used_at DATETIME NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE host_sort_orders (
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  host_id TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (scope_type, scope_id, host_id)
);

CREATE TABLE directories (
  id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX directories_parent_name_unique
ON directories(parent_id, name);

CREATE TABLE app_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  theme TEXT NOT NULL DEFAULT 'system',
  terminal_theme TEXT NOT NULL DEFAULT 'default-dark',
  default_home TEXT NOT NULL DEFAULT 'hosts',
  command_palette_shortcut TEXT NOT NULL DEFAULT 'meta+k',
  new_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+t',
  close_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+w',
  previous_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+arrowleft',
  next_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+arrowright',
  move_tab_left_shortcut TEXT NOT NULL DEFAULT 'meta+shift+arrowleft',
  move_tab_right_shortcut TEXT NOT NULL DEFAULT 'meta+shift+arrowright',
  terminal_font_size INTEGER NOT NULL DEFAULT 13,
  split_vertical_shortcut TEXT NOT NULL DEFAULT 'meta+d',
  split_horizontal_shortcut TEXT NOT NULL DEFAULT 'meta+shift+d',
  ssh_directory_path TEXT NOT NULL DEFAULT '',
  ssh_directory_bookmark TEXT NOT NULL DEFAULT '',
  ssh_directory_bookmark_created_at DATETIME NOT NULL DEFAULT '',
  sync_directory_path TEXT NOT NULL DEFAULT '',
  sync_directory_bookmark TEXT NOT NULL DEFAULT '',
  sync_directory_bookmark_created_at DATETIME NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL
);

CREATE TABLE migrations (
  version INTEGER PRIMARY KEY,
  applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO migrations (version) VALUES (1), (2);

INSERT INTO directories (id, parent_id, name, sort_order, created_at, updated_at) VALUES
  ('dir-production', '', 'Production', 10, '${NOW}', '${NOW}'),
  ('dir-staging', '', 'Staging', 20, '${NOW}', '${NOW}'),
  ('dir-infra', '', 'Infrastructure', 30, '${NOW}', '${NOW}'),
  ('dir-production-api', 'dir-production', 'API', 10, '${NOW}', '${NOW}');

INSERT INTO hosts (
  id, directory_id, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id,
  forward_agent, compression, server_alive_interval, server_alive_count_max,
  connect_timeout, remark, favorite, last_used_at, created_at, updated_at
) VALUES
  ('host-prod-api-01', 'dir-production-api', 'prod-api-01', 'api-01.example.internal', 'deploy', 22, '~/.ssh/id_ed25519', 0, 'jump-host', 'host-jump', 1, 1, 30, 3, 10, 'Primary production API node', 1, '${NOW}', '${NOW}', '${NOW}'),
  ('host-prod-api-02', 'dir-production-api', 'prod-api-02', 'api-02.example.internal', 'deploy', 22, '~/.ssh/id_ed25519', 0, 'jump-host', 'host-jump', 1, 1, 30, 3, 10, 'Secondary production API node', 0, '', '${NOW}', '${NOW}'),
  ('host-staging-db', 'dir-staging', 'staging-db', 'db.staging.example.internal', 'admin', 2222, '~/.ssh/staging_key', 0, '', '', 0, 1, 30, 3, 8, 'Staging database access', 1, '${NOW}', '${NOW}', '${NOW}'),
  ('host-jump', 'dir-infra', 'jump-host', 'jump.example.internal', 'ops', 22, '~/.ssh/id_ed25519', 0, '', '', 0, 1, 60, 3, 10, 'Shared jump host', 1, '${NOW}', '${NOW}', '${NOW}'),
  ('host-cache', 'dir-infra', 'cache-node', 'cache.example.internal', 'ops', 22, '~/.ssh/id_ed25519', 0, 'jump-host', 'host-jump', 0, 0, 30, 3, 10, 'Cache maintenance node', 0, '', '${NOW}', '${NOW}');

INSERT INTO host_sort_orders (scope_type, scope_id, host_id, sort_order, created_at, updated_at) VALUES
  ('directory', 'dir-production-api', 'host-prod-api-01', 10, '${NOW}', '${NOW}'),
  ('directory', 'dir-production-api', 'host-prod-api-02', 20, '${NOW}', '${NOW}'),
  ('directory', 'dir-staging', 'host-staging-db', 10, '${NOW}', '${NOW}'),
  ('directory', 'dir-infra', 'host-jump', 10, '${NOW}', '${NOW}'),
  ('directory', 'dir-infra', 'host-cache', 20, '${NOW}', '${NOW}');

INSERT INTO app_settings (
  id, theme, terminal_theme, default_home, ssh_directory_path,
  ssh_directory_bookmark, ssh_directory_bookmark_created_at, updated_at
) VALUES (
  1, 'system', 'default-dark', 'hosts', '${SSH_DIR}', '', '', '${NOW}'
);
SQL

cat > "${SSH_DIR}/config" <<'EOF'
# User managed SSH config.

Host github.com
    HostName github.com
    User git
EOF

cat > "${SSH_DIR}/config.d/ssh-connect.generated.conf" <<'EOF'
# Generated by aTerm

Host prod-api-01
    HostName api-01.example.internal
    User deploy
EOF

echo "Prepared App Store demo HOME: ${DEMO_HOME}"
echo "Demo database: ${DB_PATH}"
echo
echo "Launch the packaged app with:"
echo "  HOME='${DEMO_HOME}' '${REPO_ROOT}/build/bin/aTerm.app/Contents/MacOS/aTerm'"
