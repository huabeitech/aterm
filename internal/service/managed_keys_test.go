package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	desktopkeys "ssh-connect/internal/desktop/keys"
)

func TestManagedKeysPersistWithoutSystemCredentialsAndUnlockAfterRestart(t *testing.T) {
	app, keys, store := managedBackupService(t)
	ctx := context.Background()
	if _, err := keys.Create(desktopkeys.CreateInput{Name: "Locked", Algorithm: "ed25519"}); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("locked create: %v", err)
	}
	if _, err := app.SetUpSecureData(ctx, "vault-fixture-password"); err != nil {
		t.Fatal(err)
	}
	// Keychain failure must not prevent private key creation or file backup.
	app.passwordStore = unavailableKeychain{}
	key, err := keys.Create(desktopkeys.CreateInput{Name: "Persisted", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := keys.ReadIdentity(desktopkeys.Prefix + key.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(original)
	data, err := os.ReadFile(app.localVaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("PRIVATE KEY")) || bytes.Contains(data, original) {
		t.Fatal("private material persisted in plaintext")
	}
	info, _ := os.Stat(app.localVaultPath)
	if info.Mode().Perm() != 0600 {
		t.Fatal("vault is not owner-only")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(app.localVaultPath), "managed-keys.json")); !os.IsNotExist(err) {
		t.Fatal("separate catalog was written")
	}
	restarted := NewAppService(AppServiceOptions{Store: store, PasswordStore: unavailableKeychain{}, LocalVaultPath: app.localVaultPath, BackupDir: app.backupDir})
	reopened := desktopkeys.NewStore(restarted.SyncService)
	defer reopened.Close()
	if _, err = reopened.ReadIdentity(desktopkeys.Prefix + key.ID); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("locked read: %v", err)
	}
	if _, err = restarted.UnlockSecureData(ctx, "vault-fixture-password"); err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.ReadIdentity(desktopkeys.Prefix + key.ID)
	if err != nil || !bytes.Equal(original, restored) {
		t.Fatal("password unlock did not recover private key", err)
	}
	clear(restored)
	if _, err = restarted.ExportEncryptedBackup(ctx, filepath.Join(t.TempDir(), "backup.enc")); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Delete(key.ID); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(app.localVaultPath)
	opened, rawKey, err := restarted.vaultStore.Unlock(data, "vault-fixture-password")
	defer clear(rawKey)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Payload.ManagedKeys == nil || len(*opened.Payload.ManagedKeys) != 0 {
		t.Fatal("deletion was not persisted")
	}
}

type unavailableKeychain struct{}

func (unavailableKeychain) Get(string) (string, error) { return "", ErrPasswordStoreUnavailable }
func (unavailableKeychain) Set(string, string) error   { return ErrPasswordStoreUnavailable }
func (unavailableKeychain) Delete(string) error        { return ErrPasswordStoreUnavailable }

func TestManagedKeyWriteFailurePreservesSessionAndFile(t *testing.T) {
	app, keys, _ := managedBackupService(t)
	if _, err := app.SetUpSecureData(context.Background(), "fixture-password"); err != nil {
		t.Fatal(err)
	}
	key, err := keys.Create(desktopkeys.CreateInput{Name: "Keep", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	originalPath := app.localVaultPath
	before, _ := os.ReadFile(originalPath)
	_, revision, _ := app.LoadKeys()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	app.localVaultPath = filepath.Join(blocker, "vault.enc")
	if err := keys.Delete(key.ID); err == nil {
		t.Fatal("failed write reported success")
	}
	if _, err := keys.Create(desktopkeys.CreateInput{Name: "Fail", Algorithm: "ed25519"}); err == nil {
		t.Fatal("failed create reported success")
	}
	values, afterRevision, err := app.LoadKeys()
	if err != nil || len(values) != 1 || values[0].ID != key.ID || revision != afterRevision {
		t.Fatal("failed write mutated session", err)
	}
	after, _ := os.ReadFile(originalPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed write changed original vault")
	}
}

func TestManagedKeyStaleMutationCannotOverwriteRestoredWorkspace(t *testing.T) {
	app, _, _ := managedBackupService(t)
	if _, err := app.SetUpSecureData(context.Background(), "fixture-password"); err != nil {
		t.Fatal(err)
	}
	_, revision, _ := app.LoadKeys()
	_, path := makeFileBackup(t)
	if _, err := app.RestoreEncryptedBackup(context.Background(), path, "backup-fixture-password", func(SyncMetadata) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := app.SaveKeys(revision, nil); err == nil {
		t.Fatal("stale mutation replaced restored keys")
	}
	values, _, err := app.LoadKeys()
	if err != nil || len(values) != 1 {
		t.Fatal("restored keys were lost", err)
	}
}
