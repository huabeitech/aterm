package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenCreatesInitialSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	for _, table := range []string{"migrations", "hosts", "directories", "host_sort_orders", "app_settings", "app_metadata"} {
		var name string
		if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("initial schema missing %s: %v", table, err)
		}
	}
	assertAppliedMigrationVersion(t, store, 1)
	assertAppliedMigrationVersion(t, store, 2)
	assertAppliedMigrationVersion(t, store, 3)
	assertAppliedMigrationVersion(t, store, 4)
	assertAppliedMigrationVersion(t, store, 5)
	if present, err := schemaHasColumn(store, "app_settings", "terminal_theme"); err != nil {
		t.Fatalf("inspect app_settings.terminal_theme: %v", err)
	} else if !present {
		t.Fatal("app_settings.terminal_theme missing after migration 5")
	}
	if present, err := schemaHasColumn(store, "app_settings", "export_openssh_config"); err != nil {
		t.Fatalf("inspect removed export setting: %v", err)
	} else if present {
		t.Fatal("app_settings.export_openssh_config still exists after migration 3")
	}
}

func TestOpenBootstrapsMigrationLedgerBeforeReadingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open new database: %v", err)
	}
	defer store.Close()

	assertAppliedMigrationVersion(t, store, 1)
	assertAppliedMigrationVersion(t, store, 2)
	assertAppliedMigrationVersion(t, store, 3)
	assertAppliedMigrationVersion(t, store, 4)
	assertAppliedMigrationVersion(t, store, 5)
}

func TestAppMetadataPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.SetAppMetadata("password-migration", "1"); err != nil {
		t.Fatalf("SetAppMetadata() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	value, err := reopened.GetAppMetadata("password-migration")
	if err != nil || value != "1" {
		t.Fatalf("GetAppMetadata() = %q, %v, want 1", value, err)
	}
}

func TestOpenMigratesAndRecordsExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	seedVersionOneSchema(t, raw)
	if _, err := raw.Exec(`
DROP TABLE directories;
INSERT INTO hosts (id, alias, host_name, created_at, updated_at)
VALUES ('test-id', 'old-alias', 'test.example', '2026-01-01', '2026-01-01');
`); err != nil {
		t.Fatalf("create existing hosts table: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	assertAppliedMigrationVersion(t, store, 1)
	assertAppliedMigrationVersion(t, store, 2)
	assertAppliedMigrationVersion(t, store, 3)
	assertAppliedMigrationVersion(t, store, 4)
	assertAppliedMigrationVersion(t, store, 5)
	var directoriesTable string
	if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'directories'`).Scan(&directoriesTable); err != nil {
		t.Fatalf("initial migration should create directories: %v", err)
	}
	host, err := store.GetHost("test-id")
	if err != nil || host.Name != "old-alias" || host.Alias != "old-alias" {
		t.Fatalf("legacy host after migration: %+v, %v", host, err)
	}
}

func seedVersionOneSchema(t *testing.T, raw *sql.DB) {
	t.Helper()
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := migration_version_1(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAppliesSecondMigrationToVersionOneDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	seedVersionOneSchema(t, raw)
	_, err = raw.Exec(`
CREATE TABLE migrations (version INTEGER PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
INSERT INTO migrations (version) VALUES (1);
`)
	if err != nil {
		t.Fatalf("seed version one database: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	assertAppliedMigrationVersion(t, store, 2)
	assertAppliedMigrationVersion(t, store, 3)
	assertAppliedMigrationVersion(t, store, 4)
	assertAppliedMigrationVersion(t, store, 5)
	for _, column := range []string{
		"move_tab_left_shortcut",
		"move_tab_right_shortcut",
		"terminal_font_size",
		"split_vertical_shortcut",
		"split_horizontal_shortcut",
		"terminal_theme",
	} {
		present, err := schemaHasColumn(store, "app_settings", column)
		if err != nil {
			t.Fatalf("inspect app_settings.%s: %v", column, err)
		}
		if !present {
			t.Fatalf("app_settings.%s missing after migration 2", column)
		}
	}
}

func TestOpenMigratesVersionTwoSettingsToVersionThreeWithoutDataLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	tx, err := raw.Begin()
	if err != nil {
		t.Fatalf("begin version two seed: %v", err)
	}
	if _, err := tx.Exec(`CREATE TABLE migrations (version INTEGER PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("create migration ledger: %v", err)
	}
	if err := migration_version_1(tx); err != nil {
		t.Fatalf("apply migration 1: %v", err)
	}
	if err := migration_version_2(tx); err != nil {
		t.Fatalf("apply migration 2: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO migrations (version) VALUES (1), (2)`); err != nil {
		t.Fatalf("record version two migrations: %v", err)
	}
	if _, err := tx.Exec(`
INSERT INTO app_settings (
  id, theme, default_home, command_palette_shortcut, new_terminal_shortcut,
  close_terminal_shortcut, previous_terminal_shortcut, next_terminal_shortcut,
  move_tab_left_shortcut, move_tab_right_shortcut, terminal_font_size,
  split_vertical_shortcut, split_horizontal_shortcut,
  ssh_directory_path, ssh_directory_bookmark, ssh_directory_bookmark_created_at,
  export_openssh_config,
  sync_directory_path, sync_directory_bookmark, sync_directory_bookmark_created_at,
  updated_at
) VALUES (
  1, 'dark', 'local-terminal', 'meta+p', 'meta+n',
  'meta+q', 'meta+shift+[', 'meta+shift+]',
  'meta+alt+left', 'meta+alt+right', 17,
  'meta+1', 'meta+2',
  '/Users/test/.ssh', 'ssh-bookmark', '2026-07-01T01:02:03Z',
  1,
  '/Users/test/iCloud', 'sync-bookmark', '2026-07-02T04:05:06Z',
  '2026-07-03T07:08:09Z'
)`); err != nil {
		t.Fatalf("insert version two settings: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit version two seed: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close version two seed: %v", err)
	}

	assertVersionThreeSettings := func(store *Store) {
		t.Helper()
		settings, err := store.GetSettings()
		if err != nil {
			t.Fatalf("GetSettings() after migration: %v", err)
		}
		if settings.Theme != "dark" || settings.TerminalTheme != "default-dark" || settings.DefaultHome != "local-terminal" || settings.TerminalFontSize != 17 {
			t.Fatalf("appearance settings changed during migration: %+v", settings)
		}
		if settings.CommandPaletteShortcut != "meta+p" || settings.NewTerminalShortcut != "meta+n" ||
			settings.CloseTerminalShortcut != "meta+q" || settings.PreviousTerminalShortcut != "meta+shift+[" ||
			settings.NextTerminalShortcut != "meta+shift+]" || settings.MoveTabLeftShortcut != "meta+alt+left" ||
			settings.MoveTabRightShortcut != "meta+alt+right" || settings.SplitVerticalShortcut != "meta+1" ||
			settings.SplitHorizontalShortcut != "meta+2" {
			t.Fatalf("shortcut settings changed during migration: %+v", settings)
		}
		if settings.SSHDirectoryPath != "/Users/test/.ssh" || settings.SSHDirectoryBookmark != "ssh-bookmark" ||
			settings.SSHDirectoryBookmarkCreatedAt != "2026-07-01T01:02:03Z" {
			t.Fatalf("SSH directory authorization changed during migration: %+v", settings)
		}
		if settings.SyncDirectoryPath != "/Users/test/iCloud" || settings.SyncDirectoryBookmark != "sync-bookmark" ||
			settings.SyncDirectoryBookmarkCreatedAt != "2026-07-02T04:05:06Z" {
			t.Fatalf("sync directory authorization changed during migration: %+v", settings)
		}
		assertAppliedMigrationVersion(t, store, 3)
		assertAppliedMigrationVersion(t, store, 4)
		assertAppliedMigrationVersion(t, store, 5)
		if present, err := schemaHasColumn(store, "app_settings", "export_openssh_config"); err != nil {
			t.Fatalf("inspect removed export setting: %v", err)
		} else if present {
			t.Fatal("app_settings.export_openssh_config still exists")
		}
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open version two database: %v", err)
	}
	assertVersionThreeSettings(store)
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated database: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	defer reopened.Close()
	assertVersionThreeSettings(reopened)
}

func assertAppliedMigrationVersion(t *testing.T, store *Store, version int) {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM migrations WHERE version = ?`, version).Scan(&count); err != nil {
		t.Fatalf("query migration %d: %v", version, err)
	}
	if count != 1 {
		t.Fatalf("migration %d count = %d, want 1", version, count)
	}
}
