package db

import "database/sql"

const dropHostFavoriteColumn = `ALTER TABLE hosts DROP COLUMN favorite;`

func migration_version_10(tx *sql.Tx) error {
	_, err := tx.Exec(`DELETE FROM host_sort_orders WHERE scope_type = 'favorites';` + dropHostFavoriteColumn)
	return err
}
