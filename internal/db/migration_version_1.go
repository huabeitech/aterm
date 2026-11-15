package db

import "database/sql"

func migration_version_1(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS hosts (
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
CREATE TABLE IF NOT EXISTS host_sort_orders (
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  host_id TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (scope_type, scope_id, host_id)
);
CREATE TABLE IF NOT EXISTS directories (
  id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_directories_parent_name ON directories(parent_id, name);
CREATE TABLE IF NOT EXISTS app_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  theme TEXT NOT NULL DEFAULT 'system',
	default_home TEXT NOT NULL DEFAULT 'hosts',
  command_palette_shortcut TEXT NOT NULL DEFAULT 'meta+k',
  new_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+t',
  close_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+w',
  previous_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+arrowleft',
  next_terminal_shortcut TEXT NOT NULL DEFAULT 'meta+arrowright',
  ssh_directory_path TEXT NOT NULL DEFAULT '',
  ssh_directory_bookmark TEXT NOT NULL DEFAULT '',
  ssh_directory_bookmark_created_at DATETIME NOT NULL DEFAULT '',
  export_openssh_config INTEGER NOT NULL DEFAULT 0,
  sync_directory_path TEXT NOT NULL DEFAULT '',
  sync_directory_bookmark TEXT NOT NULL DEFAULT '',
  sync_directory_bookmark_created_at DATETIME NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL
);`)
	return err
}
