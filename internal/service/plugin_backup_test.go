package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"ssh-connect/internal/domain"
	"testing"
)

func TestEncryptedBackupIncludesOpaquePluginDataAndResetsWorkers(t *testing.T) {
	source, _, store := managedBackupService(t)
	ctx := context.Background()
	if _, err := source.SetUpSecureData(ctx, "fixture-password"); err != nil {
		t.Fatal(err)
	}
	record := domain.PluginData{PluginID: "example.tool", Key: "configs", Value: json.RawMessage(`{"version":1}`)}
	if err := store.SetPluginData(record); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vault.enc")
	if _, err := source.ExportEncryptedBackup(ctx, path); err != nil {
		t.Fatal(err)
	}
	dest, _, out := managedBackupService(t)
	reset := 0
	dest.workspaceReset = func() func() { reset++; return func() {} }
	if _, err := dest.RestoreEncryptedBackup(ctx, path, "fixture-password", func(SyncMetadata) bool { return true }); err != nil {
		t.Fatal(err)
	}
	value, err := out.GetPluginData(record.PluginID, record.Key)
	if err != nil || string(value) != string(record.Value) || reset != 1 {
		t.Fatalf("restore = %s, resets %d: %v", value, reset, err)
	}
}

func TestPluginSecretsAreNamespacedAndIncludedInBackup(t *testing.T) {
	source, _, _ := managedBackupService(t)
	ctx := context.Background()
	if _, err := source.SetUpSecureData(ctx, "fixture-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := source.PluginSecret(ctx, "example.one", "api-key", "set", "fixture-secret"); err != nil {
		t.Fatal(err)
	}
	other, err := source.PluginSecret(ctx, "example.two", "api-key", "get", "")
	if err != nil || other != "" {
		t.Fatal("secret crossed plugin namespaces", err)
	}
	path := filepath.Join(t.TempDir(), "vault.enc")
	if _, err = source.ExportEncryptedBackup(ctx, path); err != nil {
		t.Fatal(err)
	}
	dest, _, _ := managedBackupService(t)
	if _, err = dest.RestoreEncryptedBackup(ctx, path, "fixture-password", func(SyncMetadata) bool { return true }); err != nil {
		t.Fatal(err)
	}
	value, err := dest.PluginSecret(ctx, "example.one", "api-key", "get", "")
	if err != nil || value != "fixture-secret" {
		t.Fatal("plugin secret missing from encrypted backup", err)
	}
}
