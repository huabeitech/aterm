package service

import (
	"context"
	"errors"
	"testing"
)

func TestAutoUnlockDisableSurvivesManualUnlockAndRestart(t *testing.T) {
	app, _, _ := managedBackupService(t)
	ctx := context.Background()
	if err := app.SetAutoUnlock(ctx, true); !errors.Is(err, ErrVaultLocked) {
		t.Fatal(err)
	}
	if _, err := app.SetUpSecureData(ctx, "auto-fixture-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.passwordStore.Get(VaultUnlockAccount); err != nil {
		t.Fatal(err)
	}
	if err := app.SetAutoUnlock(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := app.passwordStore.Get(VaultUnlockAccount); !errors.Is(err, ErrPasswordNotFound) {
		t.Fatal("disabled key retained", err)
	}
	if _, err := app.UnlockSecureData(ctx, "auto-fixture-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.passwordStore.Get(VaultUnlockAccount); !errors.Is(err, ErrPasswordNotFound) {
		t.Fatal("manual unlock silently reenabled auto unlock", err)
	}
	status := EncryptedSyncStatus{Unlocked: true}
	app.applyAutoUnlockStatus(&status)
	if status.AutoUnlockStatus != AutoUnlockSkippedNoKey || status.AutoUnlockRepairRequired {
		t.Fatal(status)
	}
	// A stale system key must not bypass the persisted opt-out on startup.
	app.passwordStore.Set(VaultUnlockAccount, "stale")
	restarted := NewAppService(AppServiceOptions{Store: app.store, PasswordStore: app.passwordStore, LocalVaultPath: app.localVaultPath})
	if result := restarted.TryAutoUnlockSecureData(ctx); result.Status != AutoUnlockSkippedNoKey {
		t.Fatal(result)
	}
	if restarted.vaultSession != nil {
		t.Fatal("opted-out startup activated a vault")
	}
	if err := app.SetAutoUnlock(ctx, true); err != nil {
		t.Fatal(err)
	}
	if result := restarted.TryAutoUnlockSecureData(ctx); result.Status != AutoUnlockUnlocked {
		t.Fatal(result)
	}
}
func TestAutoUnlockRejectedKeychainWriteKeepsDisabledSetting(t *testing.T) {
	app, _, _ := managedBackupService(t)
	ctx := context.Background()
	if _, err := app.SetUpSecureData(ctx, "auto-fixture-password"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetAutoUnlock(ctx, false); err != nil {
		t.Fatal(err)
	}
	app.passwordStore = &failingPasswordStore{PasswordStore: app.passwordStore, setErrors: map[string]error{VaultUnlockAccount: ErrKeychainAccessDenied}}
	if err := app.SetAutoUnlock(ctx, true); !errors.Is(err, ErrSecureStorageUnavailable) {
		t.Fatal(err)
	}
	disabled, err := app.autoUnlockDisabled()
	if err != nil || !disabled {
		t.Fatal("failed enable changed persisted opt-out", err)
	}
}
