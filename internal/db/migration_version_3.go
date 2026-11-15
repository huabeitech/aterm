package db

import (
	"database/sql"
	"fmt"
)

func migration_version_3(tx *sql.Tx) error {
	present, err := migrationColumnExists(tx, "app_settings", "export_openssh_config")
	if err != nil || !present {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE app_settings DROP COLUMN export_openssh_config`)
	return err
}

func migrationColumnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
