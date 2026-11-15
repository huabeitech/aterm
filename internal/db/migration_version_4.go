package db

import "database/sql"

func migration_version_4(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS app_metadata (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);`)
	return err
}
