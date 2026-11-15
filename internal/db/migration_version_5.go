package db

import "database/sql"

func migration_version_5(tx *sql.Tx) error {
	return ensureColumn(tx, "app_settings", "terminal_theme", "TEXT NOT NULL DEFAULT 'default-dark'")
}
