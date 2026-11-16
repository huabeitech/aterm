package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	desktopkeys "ssh-connect/internal/desktop/keys"
	"ssh-connect/internal/fsutil"
)

var ErrCloudVaultConflict = errors.New("the iCloud backup belongs to different or unreadable protected data")

const (
	VaultUnlockAccount = "termssh-vault-key"

	AutoUnlockSkippedNoVault      = "skipped_no_vault"
	AutoUnlockSkippedNoKey        = "skipped_no_key"
	AutoUnlockUnlocked            = "unlocked"
	AutoUnlockKeychainDenied      = "keychain_denied"
	AutoUnlockKeychainUnavailable = "keychain_unavailable"
	AutoUnlockInvalidKey          = "invalid_key"
	AutoUnlockVaultUnavailable    = "vault_unavailable"
	AutoUnlockVaultCorrupted      = "vault_corrupted"
)

type vaultSession struct {
	managedKeys    []desktopkeys.BackupKey
	key            []byte
	encoded        []byte
	vaultID        string
	revision       string
	passwords      *vaultPasswordStore
	lastUpload     string
	lastDownload   string
	hostCount      int
	directoryCount int
}

type vaultSessionSnapshot struct {
	key            []byte
	passwords      *vaultPasswordStore
	lastUpload     string
	lastDownload   string
	hostCount      int
	directoryCount int
}

func (s *SyncService) GetEncryptedSyncStatus(ctx context.Context) (EncryptedSyncStatus, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncStatus{}, err
	}
	status := s.baseEncryptedSyncStatus()
	var localData []byte
	if data, err := os.ReadFile(s.localVaultPath); err == nil {
		status.LocalVaultAvailable = true
		localData = data
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		status.Message = "Unable to inspect protected data on this Mac."
	}
	session, hasSession := s.currentVaultSessionSnapshot()
	if hasSession {
		status.Unlocked = true
		status.LastDownload = session.lastDownload
	}
	s.applyAutoUnlockStatus(&status)

	paths, done, cloudStatus, err := s.encryptedSyncPaths(ctx)
	if err != nil {
		status.SyncDirectoryPath = cloudStatus.SyncDirectoryPath
		status.SyncDirectoryAuthorizationRequired = cloudStatus.SyncDirectoryAuthorizationRequired
		status.SyncDirectoryAuthorizationErrorText = cloudStatus.SyncDirectoryAuthorizationErrorText
	} else {
		defer done()
		status.CloudAvailable = true
		status.VaultPath = paths.syncVaultPath
		status.SyncDirectoryPath = cloudStatus.SyncDirectoryPath
		status.SyncDirectoryAuthorized = cloudStatus.SyncDirectoryAuthorized
		if _, statErr := os.Stat(filepath.Dir(paths.syncVaultPath)); statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			status.CloudAvailable = false
			status.Message = "iCloud backup storage is unavailable."
		}
		cloudData, statErr := readEncryptedBackup(paths.syncVaultPath)
		if statErr == nil {
			status.VaultAvailable = true
			if len(localData) > 0 {
				localVaultID, localIDErr := VaultID(localData)
				cloudVaultID, cloudIDErr := VaultID(cloudData)
				status.BackupUpdateRequired = localIDErr == nil && cloudIDErr == nil && localVaultID == cloudVaultID && !bytes.Equal(localData, cloudData)
			}
			if hasSession {
				if opened, openErr := s.vaultStore.Open(cloudData, session.key); openErr == nil {
					metadata := syncMetadataFromVaultPayload(opened.Payload)
					status.Metadata = &metadata
					status.LastUpload = metadata.LastUpload
				}
			}
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			status.Message = "Unable to inspect the iCloud backup."
		}
	}
	if status.Message == "" {
		switch {
		case !status.LocalVaultAvailable:
			status.Message = "Create an encryption password to protect saved passwords on this Mac."
		case status.AutoUnlockMessage != "":
			status.Message = status.AutoUnlockMessage
		case !status.Unlocked:
			status.Message = "Enter your encryption password to unlock protected data."
		case status.SyncDirectoryAuthorizationRequired:
			status.Message = "Protected data is ready. Authorize iCloud Drive to use encrypted backup."
		case status.BackupUpdateRequired:
			status.Message = "Protected data changed on this Mac. Update the iCloud backup."
		case status.VaultAvailable:
			status.Message = "Protected data and an iCloud backup are available."
		default:
			status.Message = "Protected data is ready. Back up to iCloud when needed."
		}
	}
	return status, nil
}

func (s *SyncService) SetUpSecureData(ctx context.Context, password string) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	if !validEncryptionPassword(password, 8) {
		return EncryptedSyncResult{}, ErrInvalidVaultPassword
	}
	if _, err := os.Stat(s.localVaultPath); err == nil {
		return EncryptedSyncResult{}, errors.New("protected data is already set up")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return EncryptedSyncResult{}, err
	}
	payload, info, err := s.buildVaultPayload()
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	encoded, key, err := s.vaultStore.Create(password, payload)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	if err := fsutil.AtomicWriteFile(s.localVaultPath, encoded, 0o600); err != nil {
		return EncryptedSyncResult{}, err
	}
	opened, err := s.vaultStore.Open(encoded, key)
	if err != nil {
		_ = os.Remove(s.localVaultPath)
		return EncryptedSyncResult{}, err
	}
	s.activateVaultSession(opened, key, encoded)
	s.repairAfterUnlock(key, encoded)
	return EncryptedSyncResult{HostCount: info.HostCount, DirectoryCount: info.DirectoryCount}, nil
}

func (s *SyncService) UnlockSecureData(ctx context.Context, password string) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	data, err := s.readBestVault(ctx)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	opened, key, err := s.vaultStore.Unlock(data, password)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	session := s.activateVaultSession(opened, key, data)
	if err := fsutil.AtomicWriteFile(s.localVaultPath, data, 0o600); err != nil {
		return EncryptedSyncResult{}, err
	}
	s.repairAfterUnlock(key, data)
	return encryptedSyncResultFromSession(session), nil
}

func (s *SyncService) UploadEncryptedSync(ctx context.Context) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	session := s.vaultSession
	if session == nil {
		return EncryptedSyncResult{}, ErrVaultLocked
	}
	paths, done, status, err := s.encryptedSyncPaths(ctx)
	if err != nil {
		return EncryptedSyncResult{}, errors.New(status.Message)
	}
	defer done()
	if err := s.ensureCloudVaultMatchesSession(paths.syncVaultPath, session); err != nil {
		return EncryptedSyncResult{}, err
	}
	encoded, info, err := s.sealCurrentVaultLocked(session)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	if err := fsutil.AtomicWriteFile(paths.syncVaultPath, encoded, 0o600); err != nil {
		return EncryptedSyncResult{}, err
	}
	if err := fsutil.AtomicWriteFile(s.localVaultPath, encoded, 0o600); err != nil {
		return EncryptedSyncResult{}, err
	}
	now := formatSyncTime(time.Now())
	session.lastUpload = now
	result := encryptedSyncResultFromInfo(info)
	result.LastUpload = now
	return result, nil
}

func (s *SyncService) ensureCloudVaultMatchesSession(path string, session *vaultSession) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cloudVaultID, err := VaultID(data)
	if err != nil || cloudVaultID != session.vaultID {
		return ErrCloudVaultConflict
	}
	if _, err := s.vaultStore.Open(data, session.key); err != nil {
		return ErrCloudVaultConflict
	}
	return nil
}

func (s *SyncService) DownloadEncryptedSync(ctx context.Context, password string) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	data, done, status, err := s.readCloudVault(ctx)
	if err != nil {
		return EncryptedSyncResult{}, encryptedSyncStatusError(status, err)
	}
	defer done()
	opened, key, err := s.openIncomingVault(data, password)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	defer clear(key)
	if err := s.validateIncomingWorkspace(opened.Payload); err != nil {
		return EncryptedSyncResult{}, err
	}
	installed, err := s.installIncomingWorkspace(ctx, data, opened, key, nil)
	if err != nil {
		return EncryptedSyncResult{}, err
	}

	result := encryptedSyncResultFromPayload(opened.Payload)
	result.LastDownload = installed.LastDownload
	result.BackupPath = installed.BackupPath
	return result, nil
}

func (s *SyncService) PreviewICloudBackup(ctx context.Context, password string) (SyncMetadata, error) {
	if err := ctx.Err(); err != nil {
		return SyncMetadata{}, err
	}
	data, done, status, err := s.readCloudVault(ctx)
	if err != nil {
		return SyncMetadata{}, encryptedSyncStatusError(status, err)
	}
	defer done()
	opened, _, err := s.openIncomingVault(data, password)
	if err != nil {
		return SyncMetadata{}, err
	}
	return syncMetadataFromVaultPayload(opened.Payload), nil
}

func (s *SyncService) ChangeEncryptionPassword(ctx context.Context, currentPassword, newPassword string) (EncryptedSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptedSyncResult{}, err
	}
	if !validEncryptionPassword(currentPassword, 1) || !validEncryptionPassword(newPassword, 8) {
		return EncryptedSyncResult{}, ErrInvalidVaultPassword
	}
	data, err := os.ReadFile(s.localVaultPath)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	localVaultID, err := VaultID(data)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	matchingCloudBackupExists := s.matchingCloudBackupExists(ctx, localVaultID)
	opened, key, err := s.vaultStore.Unlock(data, currentPassword)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	changed, err := s.vaultStore.ChangePassword(data, key, newPassword)
	if err != nil {
		return EncryptedSyncResult{}, err
	}
	if err := fsutil.AtomicWriteFile(s.localVaultPath, changed, 0o600); err != nil {
		return EncryptedSyncResult{}, err
	}
	s.activateVaultSession(opened, key, changed)
	s.repairAfterUnlock(key, changed)
	result := encryptedSyncResultFromPayload(opened.Payload)
	if !matchingCloudBackupExists {
		return result, nil
	}
	uploaded, uploadErr := s.UploadEncryptedSync(ctx)
	if uploadErr != nil {
		result.BackupUpdateRequired = true
		return result, nil
	}
	result.LastUpload = uploaded.LastUpload
	result.BackupUpdated = true
	return result, nil
}

func (s *SyncService) DeleteICloudBackup(ctx context.Context) error {
	return s.DeleteICloudBackupWithConfirmation(ctx, func(string) bool { return true })
}

// Confirm the cloud backup path before removing it.
func (s *SyncService) DeleteICloudBackupWithConfirmation(ctx context.Context, confirm func(string) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	paths, done, _, err := s.encryptedSyncPaths(ctx)
	if err != nil {
		return err
	}
	defer done()
	path := paths.syncVaultPath
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("cloud backup must be a regular file")
	}
	if confirm == nil || !confirm(path) {
		return nil
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *SyncService) TryAutoUnlockSecureData(ctx context.Context) AutoUnlockResult {
	if disabled, err := s.autoUnlockDisabled(); err != nil {
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockVaultUnavailable, Message: "Unable to read Auto Unlock settings."})
	} else if disabled {
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockSkippedNoKey})
	}

	if err := ctx.Err(); err != nil {
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockVaultUnavailable, Message: "Protected data is unavailable on this Mac."})
	}
	if _, err := os.Stat(s.localVaultPath); errors.Is(err, os.ErrNotExist) {
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockSkippedNoVault})
	} else if err != nil {
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockVaultUnavailable, Message: "Protected data is unavailable on this Mac."})
	}
	value, err := s.passwordStore.Get(VaultUnlockAccount)
	if err != nil || strings.TrimSpace(value) == "" {
		return s.setAutoUnlockResult(s.autoUnlockResultForPasswordStoreError(err))
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		_ = s.passwordStore.Delete(VaultUnlockAccount)
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockInvalidKey, Message: "The saved auto-unlock key no longer matches the protected data on this Mac. Enter your encryption password to re-enable Auto Unlock."})
	}
	data, err := os.ReadFile(s.localVaultPath)
	if err != nil {
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockVaultUnavailable, Message: "Protected data is unavailable on this Mac."})
	}
	opened, err := s.vaultStore.Open(data, key)
	if err != nil {
		if errors.Is(err, ErrVaultCorrupted) {
			return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockVaultCorrupted, Message: "The protected data on this Mac failed integrity validation."})
		}
		_ = s.passwordStore.Delete(VaultUnlockAccount)
		return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockInvalidKey, Message: "The saved auto-unlock key no longer matches the protected data on this Mac. Enter your encryption password to re-enable Auto Unlock."})
	}
	s.activateVaultSession(opened, key, data)
	return s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockUnlocked})
}

func (s *SyncService) rememberVaultKey(key []byte) error {
	if err := s.passwordStore.Set(VaultUnlockAccount, base64.StdEncoding.EncodeToString(key)); err != nil {
		return fmt.Errorf("%w: %v", ErrSecureStorageUnavailable, err)
	}
	return nil
}

func (s *SyncService) repairAfterUnlock(key, encoded []byte) {
	if disabled, err := s.autoUnlockDisabled(); err != nil || disabled {
		s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockSkippedNoKey})
		return
	}

	result := AutoUnlockResult{Status: AutoUnlockUnlocked}
	if err := s.rememberVaultKey(key); err != nil {
		result = AutoUnlockResult{
			Status:  AutoUnlockKeychainUnavailable,
			Message: "aTerm could not save the auto-unlock key in Keychain. You can continue, but you will need your encryption password next time.",
		}
	} else if _, err := s.vaultStore.Open(encoded, key); err != nil {
		result = AutoUnlockResult{Status: AutoUnlockInvalidKey, Message: "The saved auto-unlock key no longer matches the protected data on this Mac. Enter your encryption password to re-enable Auto Unlock."}
	}
	s.setAutoUnlockResult(result)
}

func (s *SyncService) setAutoUnlockResult(result AutoUnlockResult) AutoUnlockResult {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	s.autoUnlockResult = result
	slog.InfoContext(s.wailsContext, "termssh-auto-unlock", "status", result.Status)
	return result
}

func (s *SyncService) currentAutoUnlockResult() AutoUnlockResult {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	return s.autoUnlockResult
}

func (s *SyncService) autoUnlockResultForPasswordStoreError(err error) AutoUnlockResult {
	switch {
	case err == nil, errors.Is(err, ErrPasswordNotFound):
		return AutoUnlockResult{Status: AutoUnlockSkippedNoKey, Message: "Enter your encryption password once to enable auto unlock on this Mac."}
	case errors.Is(err, ErrKeychainUserCanceled), errors.Is(err, ErrKeychainInteractionNotAllowed), errors.Is(err, ErrKeychainAccessDenied):
		return AutoUnlockResult{Status: AutoUnlockKeychainDenied, Message: "aTerm could not access its auto-unlock key in Keychain. Enter your encryption password to try again."}
	default:
		return AutoUnlockResult{Status: AutoUnlockKeychainUnavailable, Message: "Unlock your Mac and try again, or enter your encryption password to continue."}
	}
}

func (s *SyncService) applyAutoUnlockStatus(status *EncryptedSyncStatus) {
	if disabled, err := s.autoUnlockDisabled(); err == nil && disabled {
		status.AutoUnlockStatus = AutoUnlockSkippedNoKey
		status.AutoUnlockMessage = ""
		status.AutoUnlockRepairRequired = false
		return
	}

	result := s.currentAutoUnlockResult()
	if result.Status == "" {
		if status.LocalVaultAvailable {
			result = AutoUnlockResult{Status: AutoUnlockSkippedNoKey, Message: "Enter your encryption password once to enable auto unlock on this Mac."}
		} else {
			result = AutoUnlockResult{Status: AutoUnlockSkippedNoVault}
		}
	}
	if status.Unlocked && !autoUnlockRepairRequired(result.Status) && result.Status != AutoUnlockKeychainUnavailable {
		result = AutoUnlockResult{Status: AutoUnlockUnlocked}
	}
	status.AutoUnlockStatus = result.Status
	status.AutoUnlockMessage = result.Message
	status.AutoUnlockRepairRequired = autoUnlockRepairRequired(result.Status)
}

func autoUnlockRepairRequired(status string) bool {
	switch status {
	case AutoUnlockSkippedNoKey, AutoUnlockKeychainDenied, AutoUnlockInvalidKey:
		return true
	default:
		return false
	}
}

func (s *SyncService) encryptedSyncPaths(ctx context.Context) (syncPaths, func(), EncryptedSyncStatus, error) {
	status := s.baseEncryptedSyncStatus()
	paths, done, err := s.authorizedSyncPaths(ctx)
	if err != nil {
		status.SyncDirectoryPath = s.settingsSyncDirectoryPath()
		status.SyncDirectoryAuthorizationRequired = true
		status.SyncDirectoryAuthorizationErrorText = err.Error()
		status.Message = "Authorize iCloud Drive access to use encrypted backup and restore."
		return syncPaths{}, nil, status, err
	}
	if paths.syncRootPath != "" {
		status.SyncDirectoryPath = paths.syncRootPath
		status.SyncDirectoryAuthorized = true
	}
	paths.syncVaultPath = filepath.Join(filepath.Dir(paths.syncSnapshotPath), "vault.enc")
	status.VaultPath = paths.syncVaultPath
	status.LocalVaultPath = s.localVaultPath
	return paths, done, status, nil
}

func (s *SyncService) baseEncryptedSyncStatus() EncryptedSyncStatus {
	s.ensureVaultPaths()
	return EncryptedSyncStatus{
		VaultPath:      s.syncVaultPath,
		LocalVaultPath: s.localVaultPath,
	}
}

func (s *SyncService) ensureVaultPaths() {
	if s.localVaultPath == "" {
		s.localVaultPath = filepath.Join(filepath.Dir(s.databasePath), "vault.enc")
	}
	if s.syncVaultPath == "" {
		s.syncVaultPath = filepath.Join(filepath.Dir(s.syncSnapshotPath), "vault.enc")
	}
}

func (s *SyncService) currentVaultSessionSnapshot() (vaultSessionSnapshot, bool) {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	if s.vaultSession == nil {
		return vaultSessionSnapshot{}, false
	}
	return snapshotVaultSession(s.vaultSession), true
}

func snapshotVaultSession(session *vaultSession) vaultSessionSnapshot {
	return vaultSessionSnapshot{
		key:            append([]byte(nil), session.key...),
		passwords:      session.passwords,
		lastUpload:     session.lastUpload,
		lastDownload:   session.lastDownload,
		hostCount:      session.hostCount,
		directoryCount: session.directoryCount,
	}
}

func (s *SyncService) setVaultSessionLastDownload(lastDownload string) {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	if s.vaultSession != nil {
		s.vaultSession.lastDownload = lastDownload
	}
}

func encryptedSyncResultFromInfo(info SyncSnapshotInfo) EncryptedSyncResult {
	return EncryptedSyncResult{
		HostCount:      info.HostCount,
		DirectoryCount: info.DirectoryCount,
	}
}

func encryptedSyncResultFromPayload(payload VaultPayload) EncryptedSyncResult {
	return EncryptedSyncResult{
		HostCount:      payload.HostCount,
		DirectoryCount: payload.DirectoryCount,
	}
}

func encryptedSyncResultFromSession(session vaultSessionSnapshot) EncryptedSyncResult {
	return EncryptedSyncResult{
		HostCount:      session.hostCount,
		DirectoryCount: session.directoryCount,
		LastUpload:     session.lastUpload,
		LastDownload:   session.lastDownload,
	}
}

func encryptedSyncStatusError(status EncryptedSyncStatus, err error) error {
	if status.Message != "" {
		return errors.New(status.Message)
	}
	return err
}

func (s *SyncService) readCloudVault(ctx context.Context) ([]byte, func(), EncryptedSyncStatus, error) {
	paths, done, status, err := s.encryptedSyncPaths(ctx)
	if err != nil {
		return nil, nil, status, err
	}
	data, err := readEncryptedBackup(paths.syncVaultPath)
	if err != nil {
		done()
		return nil, nil, status, err
	}
	return data, done, status, nil
}

func (s *SyncService) matchingCloudBackupExists(ctx context.Context, localVaultID string) bool {
	data, done, _, err := s.readCloudVault(ctx)
	if err != nil {
		return false
	}
	defer done()
	cloudVaultID, err := VaultID(data)
	return err == nil && cloudVaultID == localVaultID
}

func (s *SyncService) buildVaultPayload() (VaultPayload, SyncSnapshotInfo, error) {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	return s.buildVaultPayloadLocked()
}

func (s *SyncService) buildVaultPayloadLocked() (VaultPayload, SyncSnapshotInfo, error) {
	store, err := s.syncStore()
	if err != nil {
		return VaultPayload{}, SyncSnapshotInfo{}, err
	}
	tmp, err := s.createVaultTempFile("ssh-connect-vault-snapshot-*.db")
	if err != nil {
		return VaultPayload{}, SyncSnapshotInfo{}, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	info, err := store.ExportHostConfigSnapshot(tmpPath)
	if err != nil {
		return VaultPayload{}, SyncSnapshotInfo{}, err
	}
	snapshot, err := os.ReadFile(tmpPath)
	if err != nil {
		return VaultPayload{}, SyncSnapshotInfo{}, err
	}
	values := []desktopkeys.BackupKey{}
	if s.vaultSession != nil {
		values = append(values, s.vaultSession.managedKeys...)
	}
	managedKeys := &values
	passwords := map[string]string{}
	activePasswordStore := PasswordStore(nil)
	if s.hostPasswordStore != nil {
		activePasswordStore = s.hostPasswordStore.Active()
	}
	if activePasswordStore == nil {
		return VaultPayload{
			Snapshot:       snapshot,
			Passwords:      passwords,
			ManagedKeys:    managedKeys,
			SourceDevice:   s.resolvedDeviceName(),
			HostCount:      info.HostCount,
			DirectoryCount: info.DirectoryCount,
			SyncedAt:       formatSyncTime(time.Now()),
		}, info, nil
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return VaultPayload{}, SyncSnapshotInfo{}, err
	}
	for _, host := range hosts {
		if !host.PasswordAuth {
			continue
		}
		password, err := activePasswordStore.Get(host.ID)
		if err != nil {
			if isPasswordMissing(err) {
				continue
			}
			return VaultPayload{}, SyncSnapshotInfo{}, err
		}
		passwords[host.ID] = password
	}
	return VaultPayload{
		Snapshot:       snapshot,
		Passwords:      passwords,
		ManagedKeys:    managedKeys,
		SourceDevice:   s.resolvedDeviceName(),
		HostCount:      info.HostCount,
		DirectoryCount: info.DirectoryCount,
		SyncedAt:       formatSyncTime(time.Now()),
	}, info, nil
}

func (s *SyncService) sealCurrentVaultLocked(session *vaultSession) ([]byte, SyncSnapshotInfo, error) {
	payload, info, err := s.buildVaultPayloadLocked()
	if err != nil {
		return nil, SyncSnapshotInfo{}, err
	}
	values := append([]desktopkeys.BackupKey{}, session.managedKeys...)
	payload.ManagedKeys = &values
	if session.passwords != nil {
		payload.Passwords = session.passwords.Snapshot()
	}
	encoded, err := s.vaultStore.Seal(session.encoded, session.key, payload)
	if err != nil {
		return nil, SyncSnapshotInfo{}, err
	}
	opened, err := s.vaultStore.Open(encoded, session.key)
	if err != nil {
		return nil, SyncSnapshotInfo{}, err
	}
	session.encoded = encoded
	session.revision = opened.Revision
	session.hostCount = info.HostCount
	session.directoryCount = info.DirectoryCount
	return encoded, info, nil
}

func (s *SyncService) activateVaultSession(opened OpenedVault, key, encoded []byte) vaultSessionSnapshot {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()

	passwordStore := newVaultPasswordStore(opened.Payload.Passwords, func() error {
		s.encryptedSyncMu.Lock()
		defer s.encryptedSyncMu.Unlock()
		if s.vaultSession == nil {
			return ErrVaultLocked
		}
		encoded, _, err := s.sealCurrentVaultLocked(s.vaultSession)
		if err != nil {
			return err
		}
		return fsutil.AtomicWriteFile(s.localVaultPath, encoded, 0o600)
	})
	session := &vaultSession{
		key:            append([]byte(nil), key...),
		encoded:        append([]byte(nil), encoded...),
		vaultID:        opened.VaultID,
		revision:       opened.Revision,
		passwords:      passwordStore,
		lastUpload:     opened.Payload.SyncedAt,
		hostCount:      opened.Payload.HostCount,
		directoryCount: opened.Payload.DirectoryCount,
	}
	if opened.Payload.ManagedKeys != nil {
		session.managedKeys = append([]desktopkeys.BackupKey{}, (*opened.Payload.ManagedKeys)...)
	}
	s.vaultSession = session
	if s.hostPasswordStore != nil {
		s.hostPasswordStore.Activate(passwordStore)
	}
	return snapshotVaultSession(session)
}

func syncMetadataFromVaultPayload(payload VaultPayload) SyncMetadata {
	return SyncMetadata{
		SchemaVersion:     syncSchemaVersion,
		AppName:           "TermSSH",
		SnapshotCreatedAt: payload.SyncedAt,
		SourceDeviceName:  payload.SourceDevice,
		HostCount:         payload.HostCount,
		DirectoryCount:    payload.DirectoryCount,
		LastUpload:        payload.SyncedAt,
	}
}

func (s *SyncService) readBestVault(ctx context.Context) ([]byte, error) {
	if data, err := os.ReadFile(s.localVaultPath); err == nil {
		return data, nil
	}
	paths, done, _, err := s.encryptedSyncPaths(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	return readEncryptedBackup(paths.syncVaultPath)
}

func (s *SyncService) openIncomingVault(data []byte, password string) (OpenedVault, []byte, error) {
	if !validEncryptionPassword(password, 1) {
		return OpenedVault{}, nil, ErrInvalidVaultPassword
	}
	return s.vaultStore.Unlock(data, password)
}

func (s *SyncService) importVaultPayload(encoded []byte, payload VaultPayload, key []byte, vaultID, revision string) error {
	return s.importVaultPayloadWithMetadata(encoded, payload, key, vaultID, revision, nil)
}

func (s *SyncService) importVaultPayloadWithMetadata(encoded []byte, payload VaultPayload, key []byte, vaultID, revision string, metadata map[string]string) error {
	store, err := s.syncStore()
	if err != nil {
		return err
	}
	importSnapshot := store.ImportHostConfigSnapshot
	if len(metadata) > 0 {
		atomic, ok := s.store.(interface {
			ImportHostConfigSnapshotWithMetadata(string, map[string]string) (SyncSnapshotInfo, error)
		})
		if !ok {
			return errors.New("atomic backup preferences restore is unavailable")
		}
		importSnapshot = func(path string) (SyncSnapshotInfo, error) {
			return atomic.ImportHostConfigSnapshotWithMetadata(path, metadata)
		}
	}

	tmp, err := s.createVaultTempFile("ssh-connect-vault-import-*.db")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := os.WriteFile(tmpPath, payload.Snapshot, 0o600); err != nil {
		return err
	}
	if payload.ManagedKeys != nil {
		if err := desktopkeys.ValidateBackup(*payload.ManagedKeys); err != nil {
			return err
		}
	}
	if _, err := importSnapshot(tmpPath); err != nil {
		return err
	}
	opened := OpenedVault{VaultID: vaultID, Revision: revision, Payload: payload}
	s.activateVaultSession(opened, key, encoded)
	return nil
}

func (s *SyncService) createVaultTempFile(pattern string) (*os.File, error) {
	dir := s.vaultTempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, pattern)
}

func (s *SyncService) vaultTempDir() string {
	if strings.TrimSpace(s.localVaultPath) != "" {
		return filepath.Join(filepath.Dir(s.localVaultPath), "tmp")
	}
	if strings.TrimSpace(s.backupDir) != "" {
		return filepath.Join(s.backupDir, "tmp")
	}
	return filepath.Join(os.TempDir(), "TermSSH")
}

func (s *SyncService) backupLocalVault() (string, error) {
	data, err := os.ReadFile(s.localVaultPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// Exclusive creation retains every restore point, including multiple
	// restores in one second and backups from concurrent callers.
	directory := s.backupDir
	if directory == "" {
		directory = filepath.Join(filepath.Dir(s.localVaultPath), "backups")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, "vault-"+time.Now().UTC().Format("20060102-150405")+"-*.enc")
	if err != nil {
		return "", err
	}
	path := file.Name()
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(path)
		}
	}()
	if count, writeErr := file.Write(data); writeErr != nil {
		return "", writeErr
	} else if count != len(data) {
		return "", io.ErrShortWrite
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	complete = true
	return path, nil
}
