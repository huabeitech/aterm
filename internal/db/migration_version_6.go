package db

import "database/sql"

func migration_version_6(tx *sql.Tx) error {
	return ensureColumn(tx, "app_settings", "terminal_word_navigation", "TEXT NOT NULL DEFAULT 'shell'")
}
