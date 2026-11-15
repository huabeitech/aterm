package service

import (
	"context"
	"errors"
)

const autoUnlockDisabledKey = "desktop-auto-unlock-disabled-v1"

type autoUnlockMetadata interface {
	GetAppMetadata(string) (string, error)
	SetAppMetadata(string, string) error
}

func (s *SyncService) autoUnlockDisabled() (bool, error) {
	store, ok := s.store.(autoUnlockMetadata)
	if !ok {
		return false, nil
	}
	value, err := store.GetAppMetadata(autoUnlockDisabledKey)
	return value == "true", err
}

// SetAutoUnlock saves the currently unlocked vault key. It does not attempt to
// unlock a locked vault with a key that may not exist yet.
func (s *SyncService) SetAutoUnlock(ctx context.Context, enable bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store, ok := s.store.(autoUnlockMetadata)
	if !ok {
		return errors.New("auto unlock settings storage is unavailable")
	}
	previous, err := store.GetAppMetadata(autoUnlockDisabledKey)
	if err != nil {
		return err
	}
	if !enable {
		if err = store.SetAppMetadata(autoUnlockDisabledKey, "true"); err != nil {
			return err
		}
		if err = s.passwordStore.Delete(VaultUnlockAccount); err != nil && !isPasswordMissing(err) {
			return errors.Join(err, store.SetAppMetadata(autoUnlockDisabledKey, previous))
		}
		s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockSkippedNoKey})
		return nil
	}
	s.encryptedSyncMu.Lock()
	var key []byte
	if s.vaultSession != nil {
		key = append([]byte(nil), s.vaultSession.key...)
	}
	s.encryptedSyncMu.Unlock()
	defer clear(key)
	if len(key) == 0 {
		return ErrVaultLocked
	}
	if err = store.SetAppMetadata(autoUnlockDisabledKey, "false"); err != nil {
		return err
	}
	if err = s.rememberVaultKey(key); err != nil {
		return errors.Join(err, store.SetAppMetadata(autoUnlockDisabledKey, previous))
	}
	s.setAutoUnlockResult(AutoUnlockResult{Status: AutoUnlockUnlocked})
	return nil
}
