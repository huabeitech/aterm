package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ssh-connect/internal/db"
	desktopkeys "ssh-connect/internal/desktop/keys"
)

func managedBackupService(t *testing.T) (*AppService, *desktopkeys.Store, *db.Store) {
	t.Helper()
	directory := t.TempDir()
	store, err := db.Open(filepath.Join(directory, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	var keys *desktopkeys.Store
	service := NewAppService(AppServiceOptions{Store: store, PasswordStore: newMemoryPasswordStore(), IdentityReader: func(identity string) ([]byte, error) { return keys.ReadIdentity(identity) }, DatabasePath: filepath.Join(directory, "app.db"), LocalVaultPath: filepath.Join(directory, "vault.enc"), BackupDir: filepath.Join(directory, "backups")})
	keys = desktopkeys.NewStore(service.SyncService)
	t.Cleanup(keys.Close)
	return service, keys, store
}
func TestVaultPayloadIncludesManagedKeyAndPreservesPortableIdentity(t *testing.T) {
	service, keys, store := managedBackupService(t)
	if _, err := service.SetUpSecureData(context.Background(), "fixture-password"); err != nil {
		t.Fatal(err)
	}
	key, err := keys.Create(desktopkeys.CreateInput{Name: "Portable", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateHost(HostInput{Name: "portable", Alias: "portable", HostName: "server.test", Port: 22, IdentityFile: desktopkeys.Prefix + key.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateHost(HostInput{Name: "device", Alias: "device", HostName: "server.test", Port: 22, IdentityFile: "/device/id_rsa"}); err != nil {
		t.Fatal(err)
	}
	payload, _, err := service.buildVaultPayload()
	if err != nil {
		t.Fatal(err)
	}
	if payload.ManagedKeys == nil || len(*payload.ManagedKeys) != 1 || !strings.Contains((*payload.ManagedKeys)[0].PrivateKey, "PRIVATE KEY") {
		t.Fatal("encrypted payload omitted managed private material")
	}
	incoming, err := db.Open(filepath.Join(t.TempDir(), "incoming.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer incoming.Close()
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(snapshot, payload.Snapshot, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := incoming.ImportHostConfigSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	hosts, err := incoming.ListHosts()
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if host.Alias == "portable" && host.IdentityFile != desktopkeys.Prefix+key.ID {
			t.Fatal("portable managed identity was stripped")
		}
		if host.Alias == "device" && host.IdentityFile != "" {
			t.Fatal("device-only identity leaked into snapshot")
		}
	}
	destination, destinationKeys, _ := managedBackupService(t)
	if err := destination.importVaultPayload(nil, payload, make([]byte, 32), "fixture", "revision"); err != nil {
		t.Fatal(err)
	}
	if len(destinationKeys.List()) != 1 {
		t.Fatal("restore omitted managed keys")
	}
	if _, err := destinationKeys.ReadIdentity(desktopkeys.Prefix + key.ID); err != nil {
		t.Fatal(err)
	}
}
func TestVaultPayloadDistinguishesAbsentAndEmptyManagedKeyCatalog(t *testing.T) {
	service, _, _ := managedBackupService(t)
	payload, _, err := service.buildVaultPayload()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"managedKeys":[]`) {
		t.Fatal("an explicitly empty key backup must not become an absent legacy field")
	}
	legacy, err := json.Marshal(VaultPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), `managedKeys`) {
		t.Fatal("legacy payload unexpectedly includes a managed key catalog")
	}
}

func TestFailedWorkspaceImportRestoresPreviousManagedKeyLibrary(t *testing.T) {
	source, sourceKeys, _ := managedBackupService(t)
	if _, err := source.SetUpSecureData(context.Background(), "fixture-password"); err != nil {
		t.Fatal(err)
	}
	_, err := sourceKeys.Create(desktopkeys.CreateInput{Name: "Incoming", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _, err := source.buildVaultPayload()
	if err != nil {
		t.Fatal(err)
	}
	payload.Snapshot = []byte("not a SQLite database")
	destination, existingKeys, _ := managedBackupService(t)
	if _, err := destination.SetUpSecureData(context.Background(), "fixture-password"); err != nil {
		t.Fatal(err)
	}
	original, err := existingKeys.Create(desktopkeys.CreateInput{Name: "Keep", Algorithm: "ecdsa"})
	if err != nil {
		t.Fatal(err)
	}
	if err := destination.importVaultPayload(nil, payload, make([]byte, 32), "fixture", "revision"); err == nil {
		t.Fatal("invalid workspace snapshot was imported")
	}
	remaining := existingKeys.List()
	if len(remaining) != 1 || remaining[0].ID != original.ID {
		t.Fatal("failed workspace restore changed the managed key catalog")
	}
	if _, err := existingKeys.ReadIdentity(desktopkeys.Prefix + original.ID); err != nil {
		t.Fatal("failed workspace restore lost protected private material")
	}
}
