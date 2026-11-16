package db

import "database/sql"

const tunnelSchema = `
CREATE TABLE tunnels (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 host_id TEXT NOT NULL,
 local_address TEXT NOT NULL,
 local_port INTEGER NOT NULL CHECK(local_port BETWEEN 1 AND 65535),
 target_address TEXT NOT NULL,
 target_port INTEGER NOT NULL CHECK(target_port BETWEEN 1 AND 65535),
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE INDEX idx_tunnels_host ON tunnels(host_id);
CREATE TRIGGER tunnels_host_insert BEFORE INSERT ON tunnels
 WHEN NOT EXISTS(SELECT 1 FROM hosts WHERE id = NEW.host_id)
 BEGIN SELECT RAISE(ABORT, 'the SSH host no longer exists'); END;
CREATE TRIGGER tunnels_host_update BEFORE UPDATE OF host_id ON tunnels
 WHEN NOT EXISTS(SELECT 1 FROM hosts WHERE id = NEW.host_id)
 BEGIN SELECT RAISE(ABORT, 'the SSH host no longer exists'); END;
CREATE TRIGGER tunnels_host_delete BEFORE DELETE ON hosts
 WHEN EXISTS(SELECT 1 FROM tunnels WHERE host_id = OLD.id)
 BEGIN SELECT RAISE(ABORT, 'delete the tunnels associated with this host before deleting the host'); END;
`

func migration_version_7(tx *sql.Tx) error {
	_, err := tx.Exec(tunnelSchema)
	return err
}
