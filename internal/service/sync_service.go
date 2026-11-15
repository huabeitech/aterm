package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ssh-connect/internal/fsutil"
)

const syncSchemaVersion = 1

var ErrSyncStoreUnsupported = errors.New("sync store is not available")

type SyncStore interface {
	ExportHostConfigSnapshot(path string) (SyncSnapshotInfo, error)
	ImportHostConfigSnapshot(path string) (SyncSnapshotInfo, error)
}

func (s *SyncService) GetSyncStatus(ctx context.Context) (SyncStatus, error) {
	if err := ctx.Err(); err != nil {
		return SyncStatus{}, err
	}
	status := SyncStatus{
		SnapshotPath: s.syncSnapshotPath,
		MetadataPath: s.syncMetadataPath,
	}
	if s.syncSnapshotPath == "" || s.syncMetadataPath == "" {
		status.Message = "iCloud sync is not configured."
		return status, nil
	}
	paths, done, err := s.authorizedSyncPaths(ctx)
	if err != nil {
		status.SyncDirectoryPath = s.settingsSyncDirectoryPath()
		status.SyncDirectoryAuthorizationRequired = true
		status.SyncDirectoryAuthorizationErrorText = err.Error()
		status.Message = "Authorize iCloud Drive access to use iCloud Sync."
		return status, nil
	}
	defer done()
	status.SyncDirectoryPath = paths.syncRootPath
	status.SyncDirectoryAuthorized = true
	status.SnapshotPath = paths.syncSnapshotPath
	status.MetadataPath = paths.syncMetadataPath
	status.CloudAvailable = true

	syncDir := filepath.Dir(paths.syncSnapshotPath)
	if info, err := os.Stat(syncDir); err == nil && info.IsDir() {
		status.CloudAvailable = true
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		status.Message = "iCloud Drive is not available on this Mac."
		return status, nil
	}
	if _, err := os.Stat(paths.syncSnapshotPath); err == nil {
		status.SnapshotAvailable = true
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		status.Message = "Unable to inspect the iCloud snapshot."
		return status, nil
	}
	metadata, err := readSyncMetadata(paths.syncMetadataPath)
	if err == nil {
		status.Metadata = &metadata
		status.LastUpload = metadata.LastUpload
		status.LastDownload = metadata.LastDownload
		if metadata.SchemaVersion != syncSchemaVersion {
			status.Message = "The iCloud snapshot was created by an incompatible version of aTerm."
		}
		return status, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		status.Message = "Unable to read iCloud sync metadata."
		return status, nil
	}
	if status.SnapshotAvailable {
		status.Message = "iCloud sync metadata is missing."
	} else {
		status.Message = "No cloud snapshot found."
	}
	return status, nil
}

func (s *SyncService) UploadSyncSnapshot(ctx context.Context) (SyncResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncResult{}, err
	}
	store, err := s.syncStore()
	if err != nil {
		return SyncResult{}, err
	}
	paths, done, err := s.authorizedSyncPaths(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	defer done()
	if err := os.MkdirAll(filepath.Dir(paths.syncSnapshotPath), 0o700); err != nil {
		return SyncResult{}, err
	}
	info, err := store.ExportHostConfigSnapshot(paths.syncSnapshotPath)
	if err != nil {
		return SyncResult{}, err
	}
	now := formatSyncTime(time.Now())
	metadata := SyncMetadata{
		SchemaVersion:     syncSchemaVersion,
		AppName:           "TermSSH",
		SnapshotCreatedAt: now,
		SourceDeviceName:  s.resolvedDeviceName(),
		HostCount:         info.HostCount,
		DirectoryCount:    info.DirectoryCount,
		LastUpload:        now,
	}
	if existing, err := readSyncMetadata(paths.syncMetadataPath); err == nil {
		metadata.LastDownload = existing.LastDownload
	}
	if err := writeSyncMetadata(paths.syncMetadataPath, metadata); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		HostCount:      info.HostCount,
		DirectoryCount: info.DirectoryCount,
		LastUpload:     now,
	}, nil
}

func (s *SyncService) DownloadSyncSnapshot(ctx context.Context) (SyncResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncResult{}, err
	}
	store, err := s.syncStore()
	if err != nil {
		return SyncResult{}, err
	}
	paths, done, err := s.authorizedSyncPaths(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	defer done()
	if _, err := os.Stat(paths.syncSnapshotPath); err != nil {
		return SyncResult{}, err
	}
	metadata, err := readSyncMetadata(paths.syncMetadataPath)
	if err != nil {
		return SyncResult{}, err
	}
	if metadata.SchemaVersion != syncSchemaVersion {
		return SyncResult{}, errors.New("incompatible sync snapshot schema")
	}
	backupPath := filepath.Join(s.backupDir, "sync-"+time.Now().UTC().Format("20060102-150405")+".db")
	if _, err := store.ExportHostConfigSnapshot(backupPath); err != nil {
		return SyncResult{}, err
	}
	info, err := store.ImportHostConfigSnapshot(paths.syncSnapshotPath)
	if err != nil {
		return SyncResult{}, err
	}
	now := formatSyncTime(time.Now())
	metadata.LastDownload = now
	if err := writeSyncMetadata(paths.syncMetadataPath, metadata); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		HostCount:      info.HostCount,
		DirectoryCount: info.DirectoryCount,
		LastUpload:     metadata.LastUpload,
		LastDownload:   now,
		BackupPath:     backupPath,
	}, nil
}

func (s *SyncService) syncStore() (SyncStore, error) {
	store, ok := s.store.(SyncStore)
	if !ok {
		return nil, ErrSyncStoreUnsupported
	}
	return store, nil
}

type syncPaths struct {
	syncRootPath     string
	syncSnapshotPath string
	syncMetadataPath string
	syncVaultPath    string
}

func (s *SyncService) AuthorizeSyncDirectory(ctx context.Context, path string) (AppSettings, error) {
	if err := ctx.Err(); err != nil {
		return AppSettings{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return AppSettings{}, ErrSyncDirectoryAuthorizationRequired
	}
	if err := s.ensureExpectedSyncDirectory(path); err != nil {
		return AppSettings{}, err
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		return AppSettings{}, err
	}
	bookmark, err := createSecurityScopedBookmark(ctx, path)
	if err != nil {
		return AppSettings{}, err
	}
	return s.store.UpdateSettings(AppSettingsInput{
		Theme:                          settings.Theme,
		TerminalTheme:                  settings.TerminalTheme,
		DefaultHome:                    settings.DefaultHome,
		CommandPaletteShortcut:         settings.CommandPaletteShortcut,
		NewTerminalShortcut:            settings.NewTerminalShortcut,
		CloseTerminalShortcut:          settings.CloseTerminalShortcut,
		PreviousTerminalShortcut:       settings.PreviousTerminalShortcut,
		NextTerminalShortcut:           settings.NextTerminalShortcut,
		MoveTabLeftShortcut:            settings.MoveTabLeftShortcut,
		MoveTabRightShortcut:           settings.MoveTabRightShortcut,
		TerminalFontSize:               settings.TerminalFontSize,
		TerminalWordNavigation:         settings.TerminalWordNavigation,
		SplitVerticalShortcut:          settings.SplitVerticalShortcut,
		SplitHorizontalShortcut:        settings.SplitHorizontalShortcut,
		SSHDirectoryPath:               settings.SSHDirectoryPath,
		SSHDirectoryBookmark:           settings.SSHDirectoryBookmark,
		SSHDirectoryBookmarkCreatedAt:  settings.SSHDirectoryBookmarkCreatedAt,
		SyncDirectoryPath:              path,
		SyncDirectoryBookmark:          bookmark,
		SyncDirectoryBookmarkCreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *SyncService) settingsSyncDirectoryPath() string {
	if s.store == nil {
		return ""
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		return ""
	}
	return settings.SyncDirectoryPath
}

func (s *SyncService) authorizedSyncPaths(ctx context.Context) (syncPaths, func(), error) {
	if strings.TrimSpace(s.syncRootPath) == "" {
		return syncPaths{
			syncRootPath:     s.syncRootPath,
			syncSnapshotPath: s.syncSnapshotPath,
			syncMetadataPath: s.syncMetadataPath,
		}, func() {}, nil
	}
	if !macAppStoreBuild {
		return syncPaths{
			syncRootPath:     s.syncRootPath,
			syncSnapshotPath: s.syncSnapshotPath,
			syncMetadataPath: s.syncMetadataPath,
		}, func() {}, nil
	}
	if s.store == nil {
		return syncPaths{}, nil, errors.New("host store is required")
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		return syncPaths{}, nil, err
	}
	syncRoot := strings.TrimSpace(settings.SyncDirectoryPath)
	bookmark := strings.TrimSpace(settings.SyncDirectoryBookmark)
	if syncRoot == "" || bookmark == "" {
		return syncPaths{}, nil, ErrSyncDirectoryAuthorizationRequired
	}
	if err := s.ensureExpectedSyncDirectory(syncRoot); err != nil {
		return syncPaths{}, nil, fmt.Errorf("%w: %v", ErrSyncDirectoryAuthorizationRequired, err)
	}
	resolvedPath, done, err := startSecurityScopedAccess(ctx, syncRoot, bookmark)
	if err != nil {
		return syncPaths{}, nil, fmt.Errorf("%w: %v", ErrSyncDirectoryAuthorizationRequired, err)
	}
	if resolvedPath == "" {
		done()
		return syncPaths{}, nil, ErrSyncDirectoryAuthorizationRequired
	}
	syncDir := filepath.Join(resolvedPath, "TermSSH", "sync")
	return syncPaths{
		syncRootPath:     resolvedPath,
		syncSnapshotPath: filepath.Join(syncDir, "app.db"),
		syncMetadataPath: filepath.Join(syncDir, "metadata.json"),
	}, done, nil
}

func (s *SyncService) ensureExpectedSyncDirectory(path string) error {
	cleanSelected, err := canonicalPath(path)
	if err != nil {
		return err
	}
	expected := strings.TrimSpace(s.syncRootPath)
	if expected == "" {
		return nil
	}
	cleanExpected, err := canonicalPath(expected)
	if err != nil {
		return err
	}
	if cleanSelected != cleanExpected {
		return fmt.Errorf("%w: select %s", ErrUnexpectedSyncDirectory, cleanExpected)
	}
	return nil
}

func (s *SyncService) resolvedDeviceName() string {
	if s.deviceName != "" {
		return s.deviceName
	}
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "This Mac"
	}
	return name
}

func readSyncMetadata(path string) (SyncMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SyncMetadata{}, err
	}
	var metadata SyncMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return SyncMetadata{}, err
	}
	return metadata, nil
}

func writeSyncMetadata(path string, metadata SyncMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(path, data, 0o600)
}

func formatSyncTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}
