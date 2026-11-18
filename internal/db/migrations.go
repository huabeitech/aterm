package db

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	up      func(*sql.Tx) error
}

// Migrations are append-only. Once released, do not modify an existing
// migration; add a new version instead.
var migrations = []migration{
	{version: 1, up: migration_version_1},
	{version: 2, up: migration_version_2},
	{version: 3, up: migration_version_3},
	{version: 4, up: migration_version_4},
	{version: 5, up: migration_version_5},
	{version: 6, up: migration_version_6},
	{version: 7, up: migration_version_7},
	{version: 8, up: migration_version_8},
	{version: 9, up: migration_version_9},
}

// migrate applies only migrations that are not yet recorded as successful.
// A database created before migration tracking runs the initial migration once,
// then records it so subsequent launches avoid schema inspection.
func (s *Store) migrate() error {
	if err := validateMigrations(); err != nil {
		return err
	}
	if err := s.ensureMigrationsTable(); err != nil {
		return err
	}
	applied, err := s.appliedMigrationVersions()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if applied[migration.version] {
			continue
		}
		if err := s.applyMigration(migration); err != nil {
			return fmt.Errorf("apply migration %d: %w", migration.version, err)
		}
	}
	return nil
}

// ensureMigrationsTable bootstraps the migration ledger before it is queried.
// It intentionally lives outside versioned migrations: a new database has no
// migration history yet, so migration_version_1 cannot create the table in
// time for appliedMigrationVersions to read it.
func (s *Store) ensureMigrationsTable() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS migrations (
  version INTEGER PRIMARY KEY,
  applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`)
	return err
}

func validateMigrations() error {
	previousVersion := 0
	for _, migration := range migrations {
		if migration.version <= previousVersion {
			return fmt.Errorf("migration versions must be strictly increasing: %d after %d", migration.version, previousVersion)
		}
		if migration.up == nil {
			return fmt.Errorf("migration %d has no implementation", migration.version)
		}
		previousVersion = migration.version
	}
	return nil
}

func (s *Store) appliedMigrationVersions() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT version FROM migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func (s *Store) applyMigration(migration migration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := migration.up(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO migrations (version) VALUES (?)`, migration.version); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureColumn(tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}
