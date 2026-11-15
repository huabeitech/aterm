package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	desktopkeys "ssh-connect/internal/desktop/keys"
	"testing"
)

func makeFileBackup(t *testing.T) (*AppService, string) {
	t.Helper()
	source, keys, store := managedBackupService(t)
	if _, err := source.SetUpSecureData(context.Background(), "backup-fixture-password"); err != nil {
		t.Fatal(err)
	}
	key, err := keys.Create(desktopkeys.CreateInput{Name: "File backup key", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateHost(HostInput{Alias: "file-backup", HostName: "fixture.test", Port: 22, IdentityFile: desktopkeys.Prefix + key.ID}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vault.enc")
	if _, err = source.ExportEncryptedBackup(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	return source, path
}
func TestEncryptedFileBackupRestoreAndCancel(t *testing.T) {
	_, path := makeFileBackup(t)
	destination, keys, store := managedBackupService(t)
	os.WriteFile(destination.localVaultPath, []byte("previous encrypted file"), 0600)
	confirmCount := 0
	confirm := func(meta SyncMetadata) bool {
		confirmCount++
		if meta.HostCount != 1 {
			t.Fatal(meta)
		}
		return false
	}
	if _, err := destination.RestoreEncryptedBackup(context.Background(), path, "wrong", confirm); !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatal(err)
	}
	if confirmCount != 0 {
		t.Fatal("confirmation shown before authentication")
	}
	if _, err := destination.RestoreEncryptedBackup(context.Background(), path, "backup-fixture-password", confirm); err != nil {
		t.Fatal(err)
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 0 || len(keys.List()) != 0 {
		t.Fatal("cancel changed data")
	}
	data, _ := os.ReadFile(destination.localVaultPath)
	if string(data) != "previous encrypted file" {
		t.Fatal("cancel changed encrypted file")
	}
	result, err := destination.RestoreEncryptedBackup(context.Background(), path, "backup-fixture-password", func(SyncMetadata) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if result.HostCount != 1 || result.BackupPath == "" || result.LastDownload == "" {
		t.Fatal(result)
	}
	old, _ := os.ReadFile(result.BackupPath)
	if string(old) != "previous encrypted file" {
		t.Fatal("previous vault not retained")
	}
	hosts, _ = store.ListHosts()
	if len(hosts) != 1 || len(keys.List()) != 1 {
		t.Fatal("restore omitted workspace")
	}
	if _, err = keys.ReadIdentity(hosts[0].IdentityFile); err != nil {
		t.Fatal(err)
	}
}
func TestFileBackupOverwriteAndSymlink(t *testing.T) {
	source, path := makeFileBackup(t)
	before, _ := os.ReadFile(path)
	if _, err := source.ExportEncryptedBackup(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	previous, _ := filepath.Glob(path + ".previous-*")
	if len(previous) != 1 {
		t.Fatal(previous)
	}
	data, _ := os.ReadFile(previous[0])
	if !bytes.Equal(before, data) {
		t.Fatal("previous export damaged")
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("keep"), 0600)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(target, link)
	if _, err := source.ExportEncryptedBackup(context.Background(), link); err == nil {
		t.Fatal("export followed symlink")
	}
	if _, err := source.RestoreEncryptedBackup(context.Background(), link, "password", nil); err == nil {
		t.Fatal("restore followed symlink")
	}
}
func TestRestoreRejectsMetadataMismatchBeforeConfirmation(t *testing.T) {
	source, path := makeFileBackup(t)
	original, _ := os.ReadFile(path)
	session := source.vaultSession
	opened, err := source.vaultStore.Open(original, session.key)
	if err != nil {
		t.Fatal(err)
	}
	opened.Payload.HostCount++
	invalid, err := source.vaultStore.Seal(original, session.key, opened.Payload)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, invalid, 0600)
	destination, _, store := managedBackupService(t)
	called := false
	_, err = destination.RestoreEncryptedBackup(context.Background(), path, "backup-fixture-password", func(SyncMetadata) bool { called = true; return true })
	if err == nil || called {
		t.Fatal("mismatched metadata reached confirmation", err)
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 0 {
		t.Fatal("changed live workspace")
	}
}
func TestRestoreDoesNotChangeTablesWhenVaultWriteFails(t *testing.T) {
	_, path := makeFileBackup(t)
	destination, keys, store := managedBackupService(t)
	// A parent regular file prevents staging the vault before live import.
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("keep"), 0600)
	destination.localVaultPath = filepath.Join(blocker, "vault.enc")
	if _, err := destination.RestoreEncryptedBackup(context.Background(), path, "backup-fixture-password", func(SyncMetadata) bool { return true }); err == nil {
		t.Fatal("unwritable vault accepted")
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 0 || len(keys.List()) != 0 || destination.vaultSession != nil {
		t.Fatal("failed persistence changed live state")
	}
}
func TestRestoreDoesNotDependOnPreviousKeyStore(t *testing.T) {
	source, path := makeFileBackup(t)
	destination, keys, store := managedBackupService(t)
	old := []byte("preserve old file")
	os.WriteFile(destination.localVaultPath, old, 0600)
	keys.Close()
	if _, err := destination.RestoreEncryptedBackup(context.Background(), path, "backup-fixture-password", func(SyncMetadata) bool { return true }); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(destination.localVaultPath)
	if bytes.Equal(old, data) {
		t.Fatal("restore did not install incoming vault")
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 1 {
		t.Fatal("restore omitted hosts")
	}
	// Keep proof that this fixture is the public v1 envelope, not plaintext.
	data, _ = os.ReadFile(path)
	var envelope map[string]any
	json.Unmarshal(data, &envelope)
	if envelope["formatVersion"] != float64(1) || bytes.Contains(data, []byte("PRIVATE KEY")) {
		t.Fatal("unexpected file format")
	}
	_ = source
}
