package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"ssh-connect/internal/service"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return store
}

func TestHostCRUD(t *testing.T) {
	store := openTestStore(t)

	created, err := store.CreateHost(service.HostInput{
		Alias:        "prod-api",
		HostName:     "10.0.0.12",
		User:         "ubuntu",
		Port:         22,
		IdentityFile: "~/.ssh/prod.pem",
		ProxyJump:    "bastion",
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateHost() ID is empty")
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("ListHosts() len = %d, want 1", len(hosts))
	}
	if hosts[0].Alias != "prod-api" {
		t.Fatalf("Alias = %q, want prod-api", hosts[0].Alias)
	}

	updated, err := store.UpdateHost(created.ID, service.HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.13",
		User:     "root",
		Port:     2200,
	})
	if err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}
	if updated.HostName != "10.0.0.13" {
		t.Fatalf("HostName = %q, want 10.0.0.13", updated.HostName)
	}

	if err := store.DeleteHost(created.ID); err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	hosts, err = store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() after delete error = %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("ListHosts() len after delete = %d, want 0", len(hosts))
	}
}

func TestCreateHostRejectsDuplicateAlias(t *testing.T) {
	store := openTestStore(t)
	input := service.HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		Port:     22,
	}

	if _, err := store.CreateHost(input); err != nil {
		t.Fatalf("CreateHost() first error = %v", err)
	}
	if _, err := store.CreateHost(input); !errors.Is(err, ErrDuplicateAlias) {
		t.Fatalf("CreateHost() duplicate error = %v, want ErrDuplicateAlias", err)
	}
}

func TestMoveHostsAppendsToTargetDirectoryOrder(t *testing.T) {
	store := openTestStore(t)

	dirA, err := store.CreateDirectory(service.DirectoryInput{Name: "Directory A"})
	if err != nil {
		t.Fatalf("CreateDirectory(A) error = %v", err)
	}
	dirB, err := store.CreateDirectory(service.DirectoryInput{Name: "Directory B"})
	if err != nil {
		t.Fatalf("CreateDirectory(B) error = %v", err)
	}
	_, err = store.CreateHost(service.HostInput{Alias: "target-existing", HostName: "10.0.1.1", Port: 22, DirectoryID: dirB.ID})
	if err != nil {
		t.Fatalf("CreateHost(target) error = %v", err)
	}
	source, err := store.CreateHost(service.HostInput{Alias: "source", HostName: "10.0.0.1", Port: 22, DirectoryID: dirA.ID})
	if err != nil {
		t.Fatalf("CreateHost(source) error = %v", err)
	}

	moved, err := store.MoveHosts([]string{source.ID}, dirB.ID)
	if err != nil {
		t.Fatalf("MoveHosts() error = %v", err)
	}
	if len(moved) != 1 || moved[0].DirectoryID != dirB.ID || moved[0].SortOrders[hostSortScopeKey(service.HostSortScopeDirectory, dirB.ID)] != 1 {
		t.Fatalf("MoveHosts() = %#v, want target directory sort order 1", moved)
	}
}

func TestHostFavoritePersistsAcrossCreateListAndUpdate(t *testing.T) {
	store := openTestStore(t)

	created, err := store.CreateHost(service.HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		Port:     22,
		Favorite: true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if !created.Favorite {
		t.Fatal("CreateHost() Favorite = false, want true")
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 || !hosts[0].Favorite {
		t.Fatalf("ListHosts()[0].Favorite = %v, want true", hosts[0].Favorite)
	}

	updated, err := store.UpdateHost(created.ID, service.HostInput{
		Alias:    created.Alias,
		HostName: created.HostName,
		Port:     created.Port,
		Favorite: false,
	})
	if err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}
	if updated.Favorite {
		t.Fatal("UpdateHost() Favorite = true, want false")
	}
}

func TestOpenMigratesHostFavoriteColumn(t *testing.T) {
	t.Skip("No upgrade path is supported before the first release.")
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = raw.Exec(`
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  directory_id TEXT NOT NULL DEFAULT '',
  alias TEXT NOT NULL UNIQUE,
  host_name TEXT NOT NULL,
  user TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 22,
  identity_file TEXT NOT NULL DEFAULT '',
  proxy_jump TEXT NOT NULL DEFAULT '',
  forward_agent INTEGER NOT NULL DEFAULT 0,
  compression INTEGER NOT NULL DEFAULT 0,
  server_alive_interval INTEGER NOT NULL DEFAULT 0,
  server_alive_count_max INTEGER NOT NULL DEFAULT 0,
  connect_timeout INTEGER NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
INSERT INTO hosts (id, alias, host_name, created_at, updated_at)
VALUES ('host-1', 'prod-api', '10.0.0.12', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
`)
	if err != nil {
		t.Fatalf("seed old hosts schema error = %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close() error = %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() migrated store error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("ListHosts() len = %d, want 1", len(hosts))
	}
	if hosts[0].Favorite {
		t.Fatal("migrated Favorite = true, want default false")
	}
}

func TestMarkHostUsedPersistsLastUsedAt(t *testing.T) {
	store := openTestStore(t)

	created, err := store.CreateHost(service.HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		Port:     22,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	usedAt := "2026-01-02T03:04:05Z"
	updated, err := store.MarkHostUsed(created.ID, usedAt)
	if err != nil {
		t.Fatalf("MarkHostUsed() error = %v", err)
	}
	if updated.LastUsedAt != usedAt {
		t.Fatalf("MarkHostUsed() LastUsedAt = %q, want %q", updated.LastUsedAt, usedAt)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 || hosts[0].LastUsedAt != usedAt {
		t.Fatalf("ListHosts()[0].LastUsedAt = %q, want %q", hosts[0].LastUsedAt, usedAt)
	}
}

func TestOpenMigratesHostLastUsedAtColumn(t *testing.T) {
	t.Skip("No upgrade path is supported before the first release.")
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = raw.Exec(`
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  directory_id TEXT NOT NULL DEFAULT '',
  alias TEXT NOT NULL UNIQUE,
  host_name TEXT NOT NULL,
  user TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 22,
  identity_file TEXT NOT NULL DEFAULT '',
  proxy_jump TEXT NOT NULL DEFAULT '',
  forward_agent INTEGER NOT NULL DEFAULT 0,
  compression INTEGER NOT NULL DEFAULT 0,
  server_alive_interval INTEGER NOT NULL DEFAULT 0,
  server_alive_count_max INTEGER NOT NULL DEFAULT 0,
  connect_timeout INTEGER NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  favorite INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
INSERT INTO hosts (id, alias, host_name, created_at, updated_at)
VALUES ('host-1', 'prod-api', '10.0.0.12', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
`)
	if err != nil {
		t.Fatalf("seed old hosts schema error = %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close() error = %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() migrated store error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("ListHosts() len = %d, want 1", len(hosts))
	}
	if hosts[0].LastUsedAt != "" {
		t.Fatalf("migrated LastUsedAt = %q, want empty", hosts[0].LastUsedAt)
	}
}

func TestCountHosts(t *testing.T) {
	store := openTestStore(t)

	count, err := store.CountHosts()
	if err != nil {
		t.Fatalf("CountHosts() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountHosts() = %d, want 0", count)
	}
}

func TestSettingsPersistAcrossOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defaults, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() default error = %v", err)
	}
	expectedDefaults := service.DefaultAppSettings()
	if defaults != expectedDefaults {
		t.Fatalf("GetSettings() default = %+v, want %+v", defaults, expectedDefaults)
	}

	updated, err := store.UpdateSettings(service.AppSettingsInput{
		CommandPaletteShortcut: "meta+shift+p",
		DefaultHome:            "local-terminal",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if updated.CommandPaletteShortcut != "meta+shift+p" || updated.DefaultHome != "local-terminal" {
		t.Fatalf("UpdateSettings() = %+v, want updated command palette shortcut and default home", updated)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open() reopened error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close() reopened error = %v", err)
		}
	})
	persisted, err := reopened.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() persisted error = %v", err)
	}
	if persisted != updated {
		t.Fatalf("persisted settings = %+v, want %+v", persisted, updated)
	}
}

func TestOpenMigratesRemovedConnectionSettingsColumns(t *testing.T) {
	t.Skip("No upgrade path is supported before the first release.")
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = raw.Exec(`
CREATE TABLE app_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  terminal_tool TEXT NOT NULL,
  open_mode TEXT NOT NULL,
  auto_render_before_connect INTEGER NOT NULL DEFAULT 1,
  auto_validate_before_connect INTEGER NOT NULL DEFAULT 1,
  ssh_directory_path TEXT NOT NULL DEFAULT '',
  ssh_directory_bookmark TEXT NOT NULL DEFAULT '',
  ssh_directory_bookmark_created_at DATETIME NOT NULL DEFAULT '',
  sync_directory_path TEXT NOT NULL DEFAULT '',
  sync_directory_bookmark TEXT NOT NULL DEFAULT '',
  sync_directory_bookmark_created_at DATETIME NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL
);
INSERT INTO app_settings (
  id, terminal_tool, open_mode, auto_render_before_connect, auto_validate_before_connect,
  ssh_directory_path, ssh_directory_bookmark, ssh_directory_bookmark_created_at,
  sync_directory_path, sync_directory_bookmark, sync_directory_bookmark_created_at, updated_at
) VALUES (
  1, 'iterm2', 'window', 1, 1,
  '/Users/example/.ssh', 'ssh-bookmark', '2026-07-03T00:00:00Z',
  '/Users/example/Library/Mobile Documents/com~apple~CloudDocs', 'sync-bookmark', '2026-07-08T00:00:00Z',
  '2026-07-08T00:00:00Z'
);
`)
	if err != nil {
		t.Fatalf("seed old app_settings schema error = %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close() error = %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() migrated store error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	for _, column := range []string{"terminal_tool", "open_mode", "auto_render_before_connect", "auto_validate_before_connect"} {
		hasColumn, err := schemaHasColumn(store, "app_settings", column)
		if err != nil {
			t.Fatalf("hasColumn(%s) error = %v", column, err)
		}
		if hasColumn {
			t.Fatalf("app_settings.%s column still exists after migration", column)
		}
	}
	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if settings.SSHDirectoryPath != "/Users/example/.ssh" || settings.SSHDirectoryBookmark != "ssh-bookmark" {
		t.Fatalf("settings = %+v, want preserved SSH directory authorization", settings)
	}
	if settings.SyncDirectoryBookmark != "sync-bookmark" {
		t.Fatalf("settings = %+v, want preserved sync directory authorization", settings)
	}
}

func schemaHasColumn(store *Store, tableName, columnName string) (bool, error) {
	rows, err := store.db.Query(`PRAGMA table_info(` + quoteIdentifier(tableName) + `);`)
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
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

func TestSettingsPersistSSHDirectoryAuthorization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	updated, err := store.UpdateSettings(service.AppSettingsInput{
		SSHDirectoryPath:              "/Users/example/.ssh",
		SSHDirectoryBookmark:          "bookmark-data",
		SSHDirectoryBookmarkCreatedAt: "2026-07-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open() reopened error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close() reopened error = %v", err)
		}
	})
	persisted, err := reopened.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() persisted error = %v", err)
	}
	if persisted.SSHDirectoryPath != updated.SSHDirectoryPath ||
		persisted.SSHDirectoryBookmark != updated.SSHDirectoryBookmark ||
		persisted.SSHDirectoryBookmarkCreatedAt != updated.SSHDirectoryBookmarkCreatedAt {
		t.Fatalf("persisted SSH authorization = %+v, want %+v", persisted, updated)
	}
}

func TestExportHostConfigSnapshotExcludesSettingsAndIncludesHostUsage(t *testing.T) {
	store := openTestStore(t)
	directory, err := store.CreateDirectory(service.DirectoryInput{Name: "Production"})
	if err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	host, err := store.CreateHost(service.HostInput{
		Alias:       "prod-api",
		HostName:    "10.0.0.12",
		User:        "ubuntu",
		Port:        22,
		DirectoryID: directory.ID,
		Favorite:    true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if _, err := store.MarkHostUsed(host.ID, "2026-07-02T12:00:00Z"); err != nil {
		t.Fatalf("MarkHostUsed() error = %v", err)
	}
	if _, err := store.UpdateSettings(service.AppSettingsInput{}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	snapshotPath := filepath.Join(t.TempDir(), "app.db")
	info, err := store.ExportHostConfigSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("ExportHostConfigSnapshot() error = %v", err)
	}
	if info.HostCount != 1 || info.DirectoryCount != 1 {
		t.Fatalf("snapshot info = %+v, want 1 host and 1 directory", info)
	}

	raw, err := sql.Open("sqlite", snapshotPath)
	if err != nil {
		t.Fatalf("sql.Open(snapshot) error = %v", err)
	}
	defer raw.Close()
	var appSettingsName string
	err = raw.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'app_settings';`).Scan(&appSettingsName)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("snapshot app_settings table err = %v, name = %q, want no table", err, appSettingsName)
	}
	var favorite int
	var lastUsedAt string
	if err := raw.QueryRow(`SELECT favorite, last_used_at FROM hosts WHERE alias = 'prod-api';`).Scan(&favorite, &lastUsedAt); err != nil {
		t.Fatalf("snapshot host state query error = %v", err)
	}
	if favorite != 1 {
		t.Fatalf("snapshot favorite = %d, want 1", favorite)
	}
	if lastUsedAt != "2026-07-02T12:00:00Z" {
		t.Fatalf("snapshot last_used_at = %q, want synced usage time", lastUsedAt)
	}
	var sortOrderCount int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM host_sort_orders WHERE host_id = ?;`, host.ID).Scan(&sortOrderCount); err != nil {
		t.Fatalf("snapshot host_sort_orders query error = %v", err)
	}
	if sortOrderCount == 0 {
		t.Fatal("snapshot host_sort_orders count = 0, want synced sort orders")
	}
}

func TestImportHostConfigSnapshotReplacesHostsAndDirectoriesOnly(t *testing.T) {
	source := openTestStore(t)
	directory, err := source.CreateDirectory(service.DirectoryInput{Name: "Cloud"})
	if err != nil {
		t.Fatalf("source CreateDirectory() error = %v", err)
	}
	host, err := source.CreateHost(service.HostInput{
		Alias:       "cloud-api",
		HostName:    "10.0.0.20",
		Port:        22,
		DirectoryID: directory.ID,
		Favorite:    true,
	})
	if err != nil {
		t.Fatalf("source CreateHost() error = %v", err)
	}
	if _, err := source.MarkHostUsed(host.ID, "2026-07-02T12:00:00Z"); err != nil {
		t.Fatalf("source MarkHostUsed() error = %v", err)
	}
	snapshotPath := filepath.Join(t.TempDir(), "cloud.db")
	if _, err := source.ExportHostConfigSnapshot(snapshotPath); err != nil {
		t.Fatalf("source ExportHostConfigSnapshot() error = %v", err)
	}

	target := openTestStore(t)
	if _, err := target.UpdateSettings(service.AppSettingsInput{}); err != nil {
		t.Fatalf("target UpdateSettings() error = %v", err)
	}
	if _, err := target.CreateHost(service.HostInput{
		Alias:    "local-only",
		HostName: "127.0.0.1",
		Port:     22,
	}); err != nil {
		t.Fatalf("target CreateHost() error = %v", err)
	}

	info, err := target.ImportHostConfigSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("ImportHostConfigSnapshot() error = %v", err)
	}
	if info.HostCount != 1 || info.DirectoryCount != 1 {
		t.Fatalf("import info = %+v, want 1 host and 1 directory", info)
	}
	hosts, err := target.ListHosts()
	if err != nil {
		t.Fatalf("target ListHosts() error = %v", err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "cloud-api" {
		t.Fatalf("target hosts = %+v, want only cloud-api", hosts)
	}
	if !hosts[0].Favorite {
		t.Fatal("imported Favorite = false, want true")
	}
	if hosts[0].LastUsedAt != "2026-07-02T12:00:00Z" {
		t.Fatalf("imported LastUsedAt = %q, want synced usage time", hosts[0].LastUsedAt)
	}
}

func TestListHostsEmptyReturnsEmptySlice(t *testing.T) {
	store := openTestStore(t)

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if hosts == nil {
		t.Fatal("ListHosts() = nil, want empty slice")
	}
	if len(hosts) != 0 {
		t.Fatalf("ListHosts() len = %d, want 0", len(hosts))
	}
}

func TestDirectoryCRUDAndHostAssignment(t *testing.T) {
	store := openTestStore(t)

	directory, err := store.CreateDirectory(service.DirectoryInput{Name: "Production"})
	if err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	if directory.ID == "" {
		t.Fatal("CreateDirectory() ID is empty")
	}
	if directory.Name != "Production" {
		t.Fatalf("Directory Name = %q, want Production", directory.Name)
	}

	host, err := store.CreateHost(service.HostInput{
		Alias:       "prod-api",
		HostName:    "10.0.0.12",
		Port:        22,
		DirectoryID: directory.ID,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if host.DirectoryID != directory.ID {
		t.Fatalf("Host DirectoryID = %q, want %q", host.DirectoryID, directory.ID)
	}

	updated, err := store.UpdateDirectory(directory.ID, service.DirectoryInput{Name: "Production API"})
	if err != nil {
		t.Fatalf("UpdateDirectory() error = %v", err)
	}
	if updated.Name != "Production API" {
		t.Fatalf("Updated directory Name = %q, want Production API", updated.Name)
	}

	directories, err := store.ListDirectories()
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if len(directories) != 1 {
		t.Fatalf("ListDirectories() len = %d, want 1", len(directories))
	}
	if directories[0].Name != "Production API" {
		t.Fatalf("ListDirectories()[0].Name = %q, want Production API", directories[0].Name)
	}
}

func TestDeleteDirectoryClearsAssignedHosts(t *testing.T) {
	store := openTestStore(t)

	directory, err := store.CreateDirectory(service.DirectoryInput{Name: "Archive"})
	if err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	host, err := store.CreateHost(service.HostInput{
		Alias:       "legacy-db",
		HostName:    "10.9.2.8",
		Port:        22,
		DirectoryID: directory.ID,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if err := store.DeleteDirectory(directory.ID); err != nil {
		t.Fatalf("DeleteDirectory() error = %v", err)
	}
	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("ListHosts() len = %d, want 1", len(hosts))
	}
	if hosts[0].ID != host.ID {
		t.Fatalf("Host ID = %q, want %q", hosts[0].ID, host.ID)
	}
	if hosts[0].DirectoryID != "" {
		t.Fatalf("Host DirectoryID after delete = %q, want empty", hosts[0].DirectoryID)
	}
}

func TestCreateDirectoryRejectsDuplicateNameInSameParent(t *testing.T) {
	store := openTestStore(t)
	input := service.DirectoryInput{Name: "Production"}

	if _, err := store.CreateDirectory(input); err != nil {
		t.Fatalf("CreateDirectory() first error = %v", err)
	}
	if _, err := store.CreateDirectory(input); !errors.Is(err, ErrDuplicateDirectoryName) {
		t.Fatalf("CreateDirectory() duplicate error = %v, want ErrDuplicateDirectoryName", err)
	}
}

func TestCreateDirectoryAllowsSameNameInDifferentParents(t *testing.T) {
	store := openTestStore(t)

	rootA, err := store.CreateDirectory(service.DirectoryInput{Name: "Root A"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootA error = %v", err)
	}
	rootB, err := store.CreateDirectory(service.DirectoryInput{Name: "Root B"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootB error = %v", err)
	}

	if _, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootA.ID, Name: "API"}); err != nil {
		t.Fatalf("CreateDirectory() rootA/API error = %v", err)
	}
	if _, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootB.ID, Name: "API"}); err != nil {
		t.Fatalf("CreateDirectory() rootB/API error = %v", err)
	}
	if _, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootA.ID, Name: "API"}); !errors.Is(err, ErrDuplicateDirectoryName) {
		t.Fatalf("CreateDirectory() duplicate sibling error = %v, want ErrDuplicateDirectoryName", err)
	}
}

func TestOpenMigratesDirectoryNameUniquenessToParentScope(t *testing.T) {
	t.Skip("No upgrade path is supported before the first release.")
	path := filepath.Join(t.TempDir(), "app.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = raw.Exec(`
CREATE TABLE directories (
  id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL UNIQUE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
INSERT INTO directories (id, parent_id, name, sort_order, created_at, updated_at)
VALUES
  ('root-a', '', 'Root A', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
  ('api-a', 'root-a', 'API', 0, '2026-01-01T00:00:01Z', '2026-01-01T00:00:01Z');
`)
	if err != nil {
		t.Fatalf("seed old directories schema error = %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close() error = %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() migrated store error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	rootB, err := store.CreateDirectory(service.DirectoryInput{Name: "Root B"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootB error = %v", err)
	}
	if _, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootB.ID, Name: "API"}); err != nil {
		t.Fatalf("CreateDirectory() same name in different parent after migration error = %v", err)
	}
	if _, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootB.ID, Name: "API"}); !errors.Is(err, ErrDuplicateDirectoryName) {
		t.Fatalf("CreateDirectory() duplicate sibling after migration error = %v, want ErrDuplicateDirectoryName", err)
	}
}

func TestMoveDirectoryRejectsDuplicateNameInTargetParent(t *testing.T) {
	store := openTestStore(t)

	rootA, err := store.CreateDirectory(service.DirectoryInput{Name: "Root A"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootA error = %v", err)
	}
	rootB, err := store.CreateDirectory(service.DirectoryInput{Name: "Root B"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootB error = %v", err)
	}
	childA, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootA.ID, Name: "API"})
	if err != nil {
		t.Fatalf("CreateDirectory() childA error = %v", err)
	}
	if _, err := store.CreateDirectory(service.DirectoryInput{ParentID: rootB.ID, Name: "API"}); err != nil {
		t.Fatalf("CreateDirectory() childB error = %v", err)
	}

	if _, err := store.MoveDirectory(childA.ID, service.DirectoryMoveInput{ParentID: rootB.ID}); !errors.Is(err, ErrDuplicateDirectoryName) {
		t.Fatalf("MoveDirectory() duplicate target error = %v, want ErrDuplicateDirectoryName", err)
	}
}

func TestListDirectoriesDefaultsToCreationOrder(t *testing.T) {
	store := openTestStore(t)

	first, err := store.CreateDirectory(service.DirectoryInput{Name: "First"})
	if err != nil {
		t.Fatalf("CreateDirectory() first error = %v", err)
	}
	second, err := store.CreateDirectory(service.DirectoryInput{Name: "Second"})
	if err != nil {
		t.Fatalf("CreateDirectory() second error = %v", err)
	}
	third, err := store.CreateDirectory(service.DirectoryInput{Name: "Third"})
	if err != nil {
		t.Fatalf("CreateDirectory() third error = %v", err)
	}

	directories, err := store.ListDirectories()
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	got := []string{directories[0].ID, directories[1].ID, directories[2].ID}
	want := []string{first.ID, second.ID, third.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListDirectories()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMoveDirectoryReordersSiblingsAndUpdatesParent(t *testing.T) {
	store := openTestStore(t)

	rootA, err := store.CreateDirectory(service.DirectoryInput{Name: "Root A"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootA error = %v", err)
	}
	rootB, err := store.CreateDirectory(service.DirectoryInput{Name: "Root B"})
	if err != nil {
		t.Fatalf("CreateDirectory() rootB error = %v", err)
	}
	child, err := store.CreateDirectory(service.DirectoryInput{Name: "Child", ParentID: rootA.ID})
	if err != nil {
		t.Fatalf("CreateDirectory() child error = %v", err)
	}

	moved, err := store.MoveDirectory(child.ID, service.DirectoryMoveInput{ParentID: rootB.ID, SortOrder: 0})
	if err != nil {
		t.Fatalf("MoveDirectory() child error = %v", err)
	}
	if moved.ParentID != rootB.ID {
		t.Fatalf("Moved ParentID = %q, want %q", moved.ParentID, rootB.ID)
	}
	if moved.SortOrder != 0 {
		t.Fatalf("Moved SortOrder = %d, want 0", moved.SortOrder)
	}

	movedRoot, err := store.MoveDirectory(rootB.ID, service.DirectoryMoveInput{ParentID: "", SortOrder: 0})
	if err != nil {
		t.Fatalf("MoveDirectory() rootB error = %v", err)
	}
	if movedRoot.ParentID != "" {
		t.Fatalf("Root ParentID = %q, want empty", movedRoot.ParentID)
	}

	directories, err := store.ListDirectories()
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	positions := map[string]service.Directory{}
	for _, directory := range directories {
		positions[directory.ID] = directory
	}
	if positions[rootB.ID].SortOrder != 0 {
		t.Fatalf("Root B SortOrder = %d, want 0", positions[rootB.ID].SortOrder)
	}
	if positions[rootA.ID].SortOrder != 1 {
		t.Fatalf("Root A SortOrder = %d, want 1", positions[rootA.ID].SortOrder)
	}
}

func TestMoveDirectoryRejectsDescendantParent(t *testing.T) {
	store := openTestStore(t)

	parent, err := store.CreateDirectory(service.DirectoryInput{Name: "Parent"})
	if err != nil {
		t.Fatalf("CreateDirectory() parent error = %v", err)
	}
	child, err := store.CreateDirectory(service.DirectoryInput{Name: "Child", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("CreateDirectory() child error = %v", err)
	}

	if _, err := store.MoveDirectory(parent.ID, service.DirectoryMoveInput{ParentID: child.ID, SortOrder: 0}); !errors.Is(err, service.ErrDirectoryParentCycle) {
		t.Fatalf("MoveDirectory() error = %v, want ErrDirectoryParentCycle", err)
	}
}

func TestDeleteDirectoryClearsDescendantHosts(t *testing.T) {
	store := openTestStore(t)

	parent, err := store.CreateDirectory(service.DirectoryInput{Name: "Parent"})
	if err != nil {
		t.Fatalf("CreateDirectory() parent error = %v", err)
	}
	child, err := store.CreateDirectory(service.DirectoryInput{Name: "Child", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("CreateDirectory() child error = %v", err)
	}
	host, err := store.CreateHost(service.HostInput{
		Alias:       "child-host",
		HostName:    "10.0.0.10",
		Port:        22,
		DirectoryID: child.ID,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if err := store.DeleteDirectory(parent.ID); err != nil {
		t.Fatalf("DeleteDirectory() error = %v", err)
	}
	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 || hosts[0].ID != host.ID {
		t.Fatalf("ListHosts() = %#v, want host %q", hosts, host.ID)
	}
	if hosts[0].DirectoryID != "" {
		t.Fatalf("Host DirectoryID after parent delete = %q, want empty", hosts[0].DirectoryID)
	}
	directories, err := store.ListDirectories()
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if len(directories) != 0 {
		t.Fatalf("ListDirectories() len after parent delete = %d, want 0", len(directories))
	}
}
