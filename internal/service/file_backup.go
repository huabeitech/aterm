package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	desktopkeys "ssh-connect/internal/desktop/keys"
	"ssh-connect/internal/fsutil"
	"time"
)

const MaxEncryptedBackupBytes = 64 * 1024 * 1024

// ExportEncryptedBackup uses the same authenticated v1 format as current
// Electron and cloud backups. Only encrypted bytes reach the selected file.
func (s *SyncService) ExportEncryptedBackup(ctx context.Context, path string) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	if s.vaultSession == nil {
		return EncryptedSyncResult{}, ErrVaultLocked
	}
	encoded, info, err := s.sealCurrentVaultLocked(s.vaultSession)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	if len(encoded) > MaxEncryptedBackupBytes {
		return EncryptedSyncResult{}, errors.New("backup exceeds the supported size")
	}
	if err = preserveBackupDestination(path); err != nil {
		return EncryptedSyncResult{}, err
	}
	if err = fsutil.AtomicWriteFile(path, encoded, 0600); err != nil {
		return EncryptedSyncResult{}, err
	}
	if err = fsutil.AtomicWriteFile(s.localVaultPath, encoded, 0600); err != nil {
		return EncryptedSyncResult{}, err
	}
	return encryptedSyncResultFromInfo(info), nil
}

func preserveBackupDestination(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("backup destination must be a regular file")
	}
	if info.Size() > MaxEncryptedBackupBytes {
		return errors.New("existing backup exceeds the supported size")
	}
	data, err := readEncryptedBackup(path)
	if err != nil {
		return err
	}
	previous := fmt.Sprintf("%s.previous-%d", path, time.Now().UnixNano())
	file, err := os.OpenFile(previous, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}
func readEncryptedBackup(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxEncryptedBackupBytes {
		return nil, errors.New("backup must be a regular file no larger than 64 MB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("backup changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxEncryptedBackupBytes+1))
	if err == nil && len(data) > MaxEncryptedBackupBytes {
		err = errors.New("backup exceeds the supported size")
	}
	return data, err
}

// RestoreEncryptedBackup validates all snapshot metadata before asking the
// caller to confirm replacement. Cancellation never changes live state.
func (s *SyncService) RestoreEncryptedBackup(ctx context.Context, path, password string, confirm func(SyncMetadata) bool) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	data, err := readEncryptedBackup(path)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	return s.restoreEncodedBackup(ctx, data, password, confirm)
}

// RestoreCloudEncryptedBackup shares the file restore confirmation and rollback
// path, retaining the exact bytes inspected before approval.
func (s *SyncService) RestoreCloudEncryptedBackup(ctx context.Context, password string, confirm func(SyncMetadata) bool) (EncryptedSyncResult, error) {
	data, done, status, err := s.readCloudVault(ctx)
	if err != nil {
		return EncryptedSyncResult{}, encryptedSyncStatusError(status, err)
	}
	defer done()
	return s.restoreEncodedBackup(ctx, data, password, confirm)
}
func (s *SyncService) restoreEncodedBackup(ctx context.Context, data []byte, password string, confirm func(SyncMetadata) bool) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	if len(data) > MaxEncryptedBackupBytes {
		return EncryptedSyncResult{}, errors.New("backup exceeds the supported size")
	}
	prepared, err := s.prepareIncomingBackup(data, password)
	opened, key := prepared.Opened, prepared.Key
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	defer clear(key)
	if err = s.validateIncomingWorkspace(opened.Payload); err != nil {
		return EncryptedSyncResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	if confirm == nil || !confirm(syncMetadataFromVaultPayload(opened.Payload)) {
		return EncryptedSyncResult{}, nil
	}
	if err = ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	data = prepared.Encoded
	installed, err := s.installIncomingWorkspace(ctx, data, opened, key, prepared.Metadata)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	result := encryptedSyncResultFromPayload(opened.Payload)
	result.BackupPath = installed.BackupPath
	result.LastDownload = installed.LastDownload
	return result, nil
}

func (s *SyncService) validateIncomingWorkspace(payload VaultPayload) error {
	if payload.ManagedKeys != nil {
		if err := desktopkeys.ValidateBackup(*payload.ManagedKeys); err != nil {
			return err
		}
	}
	// Read the isolated snapshot with the exact live schema validators and
	// verifies counts before any private keys or live tables can change.
	dir, err := os.MkdirTemp(s.vaultTempDir(), "validate-backup-*")
	if os.IsNotExist(err) {
		if err = os.MkdirAll(s.vaultTempDir(), 0700); err == nil {
			dir, err = os.MkdirTemp(s.vaultTempDir(), "validate-backup-*")
		}
	}
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "snapshot.db")
	if err = os.WriteFile(snapshot, payload.Snapshot, 0600); err != nil {
		return err
	}
	validator, ok := s.store.(interface {
		ValidateHostConfigSnapshot(string) (SyncSnapshotInfo, error)
	})
	if !ok {
		return errors.New("backup snapshot validation is unavailable")
	}
	info, err := validator.ValidateHostConfigSnapshot(snapshot)
	if err != nil {
		return err
	}
	if info.HostCount != payload.HostCount || info.DirectoryCount != payload.DirectoryCount {
		return errors.New("backup metadata does not match its snapshot")
	}
	return nil
}

// Persist the incoming vault before replacing live tables. A failed import
// restores the old encrypted file; importVaultPayload compensates key changes.
// There is no fallible vault write after a successful database replacement.
type installedWorkspace struct{ BackupPath, LastDownload string }

func (s *SyncService) installIncomingWorkspace(ctx context.Context, data []byte, opened OpenedVault, key []byte, metadata map[string]string) (installedWorkspace, error) {
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return installedWorkspace{}, err
	}
	defer release()
	previous, readErr := os.ReadFile(s.localVaultPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return installedWorkspace{}, readErr
	}
	backup, err := s.backupLocalVault()
	if err != nil {
		return installedWorkspace{}, err
	}
	if err = ctx.Err(); err != nil {
		return installedWorkspace{}, err
	}
	if err = fsutil.AtomicWriteFile(s.localVaultPath, data, 0600); err != nil {
		return installedWorkspace{}, err
	}
	if s.workspaceReset != nil {
		resume := s.workspaceReset()
		if resume != nil {
			defer resume()
		}
	}
	err = ctx.Err()
	if err == nil {
		err = s.importVaultPayloadWithMetadata(data, opened.Payload, key, opened.VaultID, opened.Revision, metadata)
	}
	if err != nil {
		var rollback error
		if os.IsNotExist(readErr) {
			rollback = os.Remove(s.localVaultPath)
		} else {
			rollback = fsutil.AtomicWriteFile(s.localVaultPath, previous, 0600)
		}
		return installedWorkspace{}, errors.Join(err, rollback)
	}
	s.repairAfterUnlock(key, data)
	completed := time.Now().UTC().Format(time.RFC3339Nano)
	s.setVaultSessionLastDownload(completed)
	return installedWorkspace{BackupPath: backup, LastDownload: completed}, nil
}
