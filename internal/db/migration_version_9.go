package db

import "database/sql"

// An absent alias is NULL so the UNIQUE constraint only applies to assigned aliases.
const hostSchema = `
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  directory_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  alias TEXT UNIQUE,
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
`

func migration_version_9(tx *sql.Tx) error {
	_, err := tx.Exec(`
ALTER TABLE hosts RENAME TO old_hosts;
` + hostSchema + `
INSERT INTO hosts (
  id, directory_id, name, alias, host_name, user, port, identity_file, password_auth, proxy_jump,
  jump_host_id, forward_agent, compression, server_alive_interval, server_alive_count_max,
  connect_timeout, remark, favorite, last_used_at, created_at, updated_at
)
SELECT id, directory_id, COALESCE(NULLIF(alias, ''), host_name), NULLIF(alias, ''), host_name,
       user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
       compression, server_alive_interval, server_alive_count_max, connect_timeout, remark,
       favorite, last_used_at, created_at, updated_at
FROM old_hosts;
DROP TABLE old_hosts;
` + pluginHostDeleteTrigger)
	return err
}
