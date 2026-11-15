package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ssh-connect/internal/domain"
)

func (s *Store) listHostSortOrders() ([]domain.HostSortOrder, error) {
	rows, err := s.db.Query(`
SELECT scope_type, scope_id, host_id, sort_order, created_at, updated_at
FROM host_sort_orders
ORDER BY scope_type ASC, scope_id ASC, sort_order ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sortOrders := make([]domain.HostSortOrder, 0)
	for rows.Next() {
		sortOrder, err := scanHostSortOrder(rows)
		if err != nil {
			return nil, err
		}
		sortOrders = append(sortOrders, sortOrder)
	}
	return sortOrders, rows.Err()
}

func (s *Store) ExportHostConfigSnapshot(path string) (domain.SyncSnapshotInfo, error) {
	hosts, err := s.ListHosts()
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	hosts = stripDeviceLocalHostFields(hosts)
	directories, err := s.ListDirectories()
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	sortOrders, err := s.listHostSortOrders()
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	return writeHostConfigSnapshot(path, hosts, directories, sortOrders)
}

// BuildHostConfigSnapshot writes an isolated, existing-schema snapshot without
// modifying the live library or stripping legacy device-local identity paths.
func (s *Store) BuildHostConfigSnapshot(path string, hosts []domain.Host, directories []domain.Directory, sortOrders []domain.HostSortOrder) (domain.SyncSnapshotInfo, error) {
	return writeHostConfigSnapshot(path, hosts, directories, sortOrders)
}
func writeHostConfigSnapshot(path string, hosts []domain.Host, directories []domain.Directory, sortOrders []domain.HostSortOrder) (domain.SyncSnapshotInfo, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return domain.SyncSnapshotInfo{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	raw, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	if err := createHostConfigTables(raw); err != nil {
		_ = raw.Close()
		return domain.SyncSnapshotInfo{}, err
	}
	tx, err := raw.Begin()
	if err != nil {
		_ = raw.Close()
		return domain.SyncSnapshotInfo{}, err
	}
	if err := insertDirectories(tx, directories); err != nil {
		_ = tx.Rollback()
		_ = raw.Close()
		return domain.SyncSnapshotInfo{}, err
	}
	if err := insertHosts(tx, hosts); err != nil {
		_ = tx.Rollback()
		_ = raw.Close()
		return domain.SyncSnapshotInfo{}, err
	}
	if err := insertHostSortOrders(tx, sortOrders); err != nil {
		_ = tx.Rollback()
		_ = raw.Close()
		return domain.SyncSnapshotInfo{}, err
	}
	if err := tx.Commit(); err != nil {
		_ = raw.Close()
		return domain.SyncSnapshotInfo{}, err
	}
	if err := raw.Close(); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	cleanup = false
	return domain.SyncSnapshotInfo{
		HostCount:      len(hosts),
		DirectoryCount: len(directories),
		UpdatedAt:      latestHostConfigUpdatedAt(hosts, directories, sortOrders),
	}, nil
}

type hostConfigSnapshot struct {
	hosts       []domain.Host
	directories []domain.Directory
	sortOrders  []domain.HostSortOrder
}

func readHostConfigSnapshot(path string) (hostConfigSnapshot, error) {
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		return hostConfigSnapshot{}, err
	}
	defer raw.Close()
	if err := validateHostConfigSnapshot(raw); err != nil {
		return hostConfigSnapshot{}, err
	}
	directories, err := readSnapshotDirectories(raw)
	if err != nil {
		return hostConfigSnapshot{}, err
	}
	hosts, err := readSnapshotHosts(raw)
	if err != nil {
		return hostConfigSnapshot{}, err
	}
	sortOrders, err := readSnapshotHostSortOrders(raw, hosts)
	if err != nil {
		return hostConfigSnapshot{}, err
	}

	return hostConfigSnapshot{hosts: hosts, directories: directories, sortOrders: sortOrders}, nil
}

// ValidateHostConfigSnapshot parses and validates without mutating live tables.
func (s *Store) ValidateHostConfigSnapshot(path string) (domain.SyncSnapshotInfo, error) {
	snapshot, err := readHostConfigSnapshot(path)
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	return domain.SyncSnapshotInfo{HostCount: len(snapshot.hosts), DirectoryCount: len(snapshot.directories), UpdatedAt: latestHostConfigUpdatedAt(snapshot.hosts, snapshot.directories, snapshot.sortOrders)}, nil
}
func (s *Store) ImportHostConfigSnapshot(path string) (domain.SyncSnapshotInfo, error) {
	return s.ImportHostConfigSnapshotWithMetadata(path, nil)
}

// ImportHostConfigSnapshotWithMetadata commits library and restored preferences
// together. An error preserves both rather than leaving a partial restore.
func (s *Store) ImportHostConfigSnapshotWithMetadata(path string, metadata map[string]string) (domain.SyncSnapshotInfo, error) {
	snapshot, err := readHostConfigSnapshot(path)
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	hosts, directories, sortOrders := snapshot.hosts, snapshot.directories, snapshot.sortOrders
	tx, err := s.db.Begin()
	if err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(`DELETE FROM host_sort_orders; DELETE FROM hosts; DELETE FROM directories;`); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	if err = insertDirectories(tx, directories); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	if err = insertHosts(tx, hosts); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	if err = insertHostSortOrders(tx, sortOrders); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	for key, value := range metadata {
		if _, err = tx.Exec(`INSERT INTO app_metadata(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, key, value, formatTime(time.Now())); err != nil {
			return domain.SyncSnapshotInfo{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.SyncSnapshotInfo{}, err
	}
	return domain.SyncSnapshotInfo{
		HostCount:      len(hosts),
		DirectoryCount: len(directories),
		UpdatedAt:      latestHostConfigUpdatedAt(hosts, directories, sortOrders),
	}, nil
}

type hostScanner interface {
	Scan(dest ...any) error
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type sqlReadWriter interface {
	sqlExecer
	QueryRow(query string, args ...any) *sql.Row
}

func createHostConfigTables(raw *sql.DB) error {
	_, err := raw.Exec(`
CREATE TABLE directories (
  id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX idx_directories_parent_name ON directories(parent_id, name);
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  directory_id TEXT NOT NULL DEFAULT '',
  alias TEXT NOT NULL UNIQUE,
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
CREATE TABLE host_sort_orders (
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  host_id TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (scope_type, scope_id, host_id)
);
`)
	return err
}

func validateHostConfigSnapshot(raw *sql.DB) error {
	for _, table := range []string{"hosts", "directories"} {
		var name string
		if err := raw.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?;`, table).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("snapshot missing %s table", table)
			}
			return err
		}
	}
	return nil
}

func readSnapshotHosts(raw *sql.DB) ([]domain.Host, error) {
	rows, err := raw.Query(`
SELECT id, directory_id, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
       compression, server_alive_interval, server_alive_count_max, connect_timeout,
       remark, favorite, last_used_at, created_at, updated_at
FROM hosts
ORDER BY created_at ASC, alias ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hosts := make([]domain.Host, 0)
	for rows.Next() {
		host, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hosts, nil
}

func readSnapshotDirectories(raw *sql.DB) ([]domain.Directory, error) {
	rows, err := raw.Query(`
SELECT id, parent_id, name, sort_order, created_at, updated_at
FROM directories
ORDER BY parent_id ASC, sort_order ASC, created_at ASC, name ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	directories := make([]domain.Directory, 0)
	for rows.Next() {
		directory, err := scanDirectory(rows)
		if err != nil {
			return nil, err
		}
		directories = append(directories, directory)
	}
	return directories, rows.Err()
}

func readSnapshotHostSortOrders(raw *sql.DB, hosts []domain.Host) ([]domain.HostSortOrder, error) {
	if exists, err := tableExists(raw, "host_sort_orders"); err != nil {
		return nil, err
	} else if !exists {
		return deriveHostSortOrders(hosts), nil
	}
	rows, err := raw.Query(`
SELECT scope_type, scope_id, host_id, sort_order, created_at, updated_at
FROM host_sort_orders
ORDER BY scope_type ASC, scope_id ASC, sort_order ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sortOrders := make([]domain.HostSortOrder, 0)
	for rows.Next() {
		sortOrder, err := scanHostSortOrder(rows)
		if err != nil {
			return nil, err
		}
		sortOrders = append(sortOrders, sortOrder)
	}
	return sortOrders, rows.Err()
}

func tableExists(raw *sql.DB, table string) (bool, error) {
	var name string
	err := raw.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?;`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func insertDirectories(exec sqlExecer, directories []domain.Directory) error {
	for _, directory := range directories {
		if _, err := exec.Exec(`
INSERT INTO directories (id, parent_id, name, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);
`, directory.ID, directory.ParentID, directory.Name, directory.SortOrder, directory.CreatedAt, directory.UpdatedAt); err != nil {
			if isUniqueDirectoryNameError(err) {
				return ErrDuplicateDirectoryName
			}
			return err
		}
	}
	return nil
}

func insertHosts(exec sqlExecer, hosts []domain.Host) error {
	for _, host := range hosts {
		if _, err := exec.Exec(`
INSERT INTO hosts (
  id, directory_id, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
  compression, server_alive_interval, server_alive_count_max, connect_timeout,
  remark, favorite, last_used_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`,
			host.ID, host.DirectoryID, host.Alias, host.HostName, host.User, host.Port, host.IdentityFile, boolToInt(host.PasswordAuth), host.ProxyJump, host.JumpHostID,
			boolToInt(host.ForwardAgent), boolToInt(host.Compression), host.ServerAliveInterval,
			host.ServerAliveCountMax, host.ConnectTimeout, host.Remark, boolToInt(host.Favorite),
			host.LastUsedAt, host.CreatedAt, host.UpdatedAt,
		); err != nil {
			if isUniqueAliasError(err) {
				return ErrDuplicateAlias
			}
			return err
		}
	}
	return nil
}

func stripDeviceLocalHostFields(hosts []domain.Host) []domain.Host {
	sanitized := make([]domain.Host, len(hosts))
	copy(sanitized, hosts)
	for i := range sanitized {
		if !strings.HasPrefix(sanitized[i].IdentityFile, "aterm-key://") {
			sanitized[i].IdentityFile = ""
		}
	}
	return sanitized
}

func insertHostSortOrders(exec sqlExecer, sortOrders []domain.HostSortOrder) error {
	for _, sortOrder := range sortOrders {
		scopeType, scopeID := normalizeHostSortScope(sortOrder.ScopeType, sortOrder.ScopeID)
		if _, err := exec.Exec(`
INSERT INTO host_sort_orders (scope_type, scope_id, host_id, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);
`, scopeType, scopeID, sortOrder.HostID, sortOrder.SortOrder, sortOrder.CreatedAt, sortOrder.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func latestHostConfigUpdatedAt(hosts []domain.Host, directories []domain.Directory, sortOrders []domain.HostSortOrder) string {
	latest := ""
	for _, host := range hosts {
		if host.UpdatedAt > latest {
			latest = host.UpdatedAt
		}
	}
	for _, directory := range directories {
		if directory.UpdatedAt > latest {
			latest = directory.UpdatedAt
		}
	}
	for _, sortOrder := range sortOrders {
		if sortOrder.UpdatedAt > latest {
			latest = sortOrder.UpdatedAt
		}
	}
	return latest
}
