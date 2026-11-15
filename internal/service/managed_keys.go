package service

import (
	"context"
	"errors"

	desktopkeys "ssh-connect/internal/desktop/keys"
	"ssh-connect/internal/fsutil"
)

// LoadKeys returns a detached view of the unlocked vault. Keychain is never a
// source of private keys; a missing auto-unlock credential only requires a password.
func (s *SyncService) LoadKeys() ([]desktopkeys.BackupKey, string, error) {
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	if s.vaultSession == nil {
		return nil, "", ErrVaultLocked
	}
	session := s.vaultSession
	return append([]desktopkeys.BackupKey{}, session.managedKeys...), session.vaultID + ":" + session.revision, nil
}

// SaveKeys commits catalog and private material in one authenticated file write.
// The active session changes only after durable persistence succeeds.
func (s *SyncService) SaveKeys(revision string, values []desktopkeys.BackupKey) error {
	if err := desktopkeys.ValidateBackup(values); err != nil {
		return err
	}
	release, err := s.workspaceRestoreGate.acquire(context.Background())
	if err != nil {
		return err
	}
	defer release()
	s.encryptedSyncMu.Lock()
	defer s.encryptedSyncMu.Unlock()
	session := s.vaultSession
	if session == nil {
		return ErrVaultLocked
	}
	if revision != session.vaultID+":"+session.revision {
		return errors.New("protected data changed; try the SSH key operation again")
	}
	next := *session
	next.managedKeys = append([]desktopkeys.BackupKey{}, values...)
	encoded, _, err := s.sealCurrentVaultLocked(&next)
	if err != nil {
		return err
	}
	if len(encoded) > MaxEncryptedBackupBytes {
		return errors.New("protected data exceeds the supported size")
	}
	if err = fsutil.AtomicWriteFile(s.localVaultPath, encoded, 0600); err != nil {
		return err
	}
	*session = next
	return nil
}
