package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ssh-connect/internal/desktop"
)

const legacyFixturePassword = "legacy-fixture-password"

func legacyFixturePath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../desktop/legacybackup/testdata/electron-aes-v1.enc")
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func checkLegacyRestore(t *testing.T, path string) {
	t.Helper()
	destination, _, store := managedBackupService(t)
	ctx := context.Background()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = destination.SetUpSecureData(ctx, "previous-fixture-password"); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(destination.localVaultPath)
	if err != nil {
		t.Fatal(err)
	}
	confirmations := 0
	confirm := func(meta SyncMetadata) bool {
		confirmations++
		if meta.HostCount != 2 || meta.DirectoryCount != 2 || meta.SnapshotCreatedAt != "" {
			t.Fatal(meta)
		}
		return false
	}
	if _, err = destination.RestoreEncryptedBackup(ctx, path, "wrong", confirm); !errors.Is(err, ErrInvalidVaultPassword) || confirmations != 0 {
		t.Fatal("unauthenticated backup reached confirmation", err)
	}
	if _, err = destination.RestoreEncryptedBackup(ctx, path, legacyFixturePassword, confirm); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(destination.localVaultPath)
	if !bytes.Equal(after, previous) {
		t.Fatal("cancel modified local vault")
	}
	prefs, _ := store.GetAppMetadata(desktop.PreferencesMetadataKey)
	if prefs != "" {
		t.Fatal("cancel modified preferences")
	}
	result, err := destination.RestoreEncryptedBackup(ctx, path, legacyFixturePassword, func(SyncMetadata) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := store.ListHosts()
	if err != nil || len(hosts) != 2 {
		t.Fatal("host restore failed", err, len(hosts))
	}
	directories, _ := store.ListDirectories()
	if len(directories) != 2 {
		t.Fatal("folder restore failed")
	}
	targetID := ""
	jumpID := ""
	for _, host := range hosts {
		if host.IdentityFile != "/fixture/id_ed25519" || host.Port != 2222 || !host.Compression || !host.ForwardAgent || !host.Favorite {
			t.Fatal("legacy host options changed", host)
		}
		if host.Alias == "fixture-target" {
			targetID = host.ID
			if host.ProxyJump != "fixture-jump" || host.SortOrders["all:"] != 0 {
				t.Fatal("jump/sort relationship lost", host)
			}
		} else if host.Alias == "fixture-jump" {
			jumpID = host.ID
		}
	}
	if targetID == "" || jumpID == "" {
		t.Fatal("expected aliases missing")
	}
	if password, err := destination.hostPasswordStore.Get(targetID); err != nil || password != "fixture-secret-中文" {
		t.Fatal("legacy password not activated", err)
	}
	prefs, _ = store.GetAppMetadata(desktop.PreferencesMetadataKey)
	var settings desktop.Preferences
	if json.Unmarshal([]byte(prefs), &settings) != nil || settings["appFontScale"] != float64(110) || settings["terminalFontSize"] != float64(18) {
		t.Fatal("legacy preferences not restored")
	}
	if result.LastDownload == "" {
		t.Fatal("file restore did not signal renderer refresh")
	}
	status, err := destination.GetEncryptedSyncStatus(ctx)
	if err != nil || status.LastDownload != result.LastDownload {
		t.Fatal("renderer cannot observe completed restore", err, status.LastDownload)
	}
	converted, err := os.ReadFile(destination.localVaultPath)
	if err != nil || !bytes.Contains(converted, []byte(`"formatVersion":1`)) {
		t.Fatal("legacy backup was not migrated to current vault format", err)
	}
	if _, _, err = destination.vaultStore.Unlock(converted, legacyFixturePassword); err != nil {
		t.Fatal("converted vault cannot unlock", err)
	}
	sourceAfter, _ := os.ReadFile(path)
	if !bytes.Equal(sourceAfter, original) {
		t.Fatal("legacy source backup was modified")
	}
	if len(result.BackupPath) == 0 {
		t.Fatal("previous local vault was not preserved")
	}
	saved, _ := os.ReadFile(result.BackupPath)
	if !bytes.Equal(saved, previous) {
		t.Fatal("previous vault backup differs")
	}
	// A fresh service unlocks the converted bytes with memory-only credentials.
	restarted := NewAppService(AppServiceOptions{Store: store, PasswordStore: newMemoryPasswordStore(), DatabasePath: destination.databasePath, LocalVaultPath: destination.localVaultPath, BackupDir: destination.backupDir})
	if _, err = restarted.UnlockSecureData(ctx, legacyFixturePassword); err != nil {
		t.Fatal("restart unlock failed", err)
	}
	if password, err := restarted.hostPasswordStore.Get(targetID); err != nil || password != "fixture-secret-中文" {
		t.Fatal("password did not survive restart", err)
	}
}
func TestLegacyElectronFileBackupRestore(t *testing.T) { checkLegacyRestore(t, legacyFixturePath(t)) }
func TestLegacyBackupGeneratedByCurrentElectron(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR to execute actual Electron legacy library/backup code")
	}
	script, err := filepath.Abs("../desktop/legacybackup/testdata/generate.mts")
	if err != nil {
		t.Fatal(err)
	}
	for _, embedded := range []bool{false, true} {
		t.Run(map[bool]string{false: "AES-library", true: "embedded-compatible-vault"}[embedded], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.enc")
			args := []string{script, reference, path}
			if embedded {
				args = append(args, "--embedded")
			}
			command := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), args...)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("Electron legacy fixture: %v: %s", err, output)
			}
			checkLegacyRestore(t, path)
		})
	}

}
func TestLegacyRestorePreferencesFailureRollsBackLibraryAndVault(t *testing.T) {
	destination, _, store := managedBackupService(t)
	ctx := context.Background()
	if _, err := store.CreateHost(HostInput{Name: "preserved-host", Alias: "preserved-host", HostName: "before.test", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if _, err := destination.SetUpSecureData(ctx, "previous-fixture-password"); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.ReadFile(destination.localVaultPath)
	beforePrefs := `{"terminalFontSize":14}`
	if err := store.SetAppMetadata(desktop.PreferencesMetadataKey, beforePrefs); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", destination.databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_, err = raw.Exec(`CREATE TRIGGER reject_restored_preferences BEFORE INSERT ON app_metadata WHEN NEW.key='desktop-preferences-v1' BEGIN SELECT RAISE(ABORT,'fixture preferences failure'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = destination.RestoreEncryptedBackup(ctx, legacyFixturePath(t), legacyFixturePassword, func(SyncMetadata) bool { return true }); err == nil {
		t.Fatal("metadata failure was hidden")
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 1 || hosts[0].Alias != "preserved-host" {
		t.Fatal("failed metadata restore replaced live hosts", hosts)
	}
	prefs, _ := store.GetAppMetadata(desktop.PreferencesMetadataKey)
	if prefs != beforePrefs {
		t.Fatal("failed restore replaced preferences")
	}
	after, _ := os.ReadFile(destination.localVaultPath)
	if !bytes.Equal(after, previous) {
		t.Fatal("failed metadata restore replaced local vault")
	}
	if _, _, err = destination.vaultStore.Unlock(after, "previous-fixture-password"); err != nil {
		t.Fatal("previous vault no longer unlocks", err)
	}
}

func TestCloudRestoreUsesOnlyConfiguredPath(t *testing.T) {
	destination, _, store := managedBackupService(t)
	ctx := context.Background()
	root := t.TempDir()
	current := filepath.Join(root, "aTerm", "sync", "vault.enc")
	destination.syncRootPath = root
	destination.syncSnapshotPath = filepath.Join(filepath.Dir(current), "app.db")
	destination.syncVaultPath = current
	backup, err := os.ReadFile(legacyFixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "other-app", "sync", "vault.enc"),
		filepath.Join(root, "aTerm-dev", "sync", "vault.enc"),
		filepath.Join(root, "aterm-backup.enc"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, backup, 0600); err != nil {
			t.Fatal(err)
		}
	}
	status, err := destination.GetEncryptedSyncStatus(ctx)
	if err != nil || status.VaultAvailable || status.VaultPath != current {
		t.Fatal("backup outside the configured path was discovered", err, status)
	}
	if _, err := destination.RestoreCloudEncryptedBackup(ctx, legacyFixturePassword, func(SyncMetadata) bool {
		t.Fatal("backup outside the configured path reached confirmation")
		return false
	}); err == nil {
		t.Fatal("restore succeeded without a backup at the configured path")
	}
	if err := os.MkdirAll(filepath.Dir(current), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, backup, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := destination.RestoreCloudEncryptedBackup(ctx, legacyFixturePassword, func(meta SyncMetadata) bool { return meta.HostCount == 2 })
	if err != nil || result.LastDownload == "" {
		t.Fatal("cloud restore failed", err, result)
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 2 {
		t.Fatal("cloud restore missing hosts")
	}
}

func TestCloudDeletionConfirmsSelectedPathAndKeepsLocalData(t *testing.T) {
	app, _, _ := managedBackupService(t)
	ctx := context.Background()
	root := t.TempDir()
	app.syncRootPath = root
	app.syncSnapshotPath = filepath.Join(root, "aTerm", "sync", "app.db")
	path := filepath.Join(root, "aTerm", "sync", "vault.enc")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(legacyFixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = app.SetUpSecureData(ctx, "local-fixture-password"); err != nil {
		t.Fatal(err)
	}
	local, _ := os.ReadFile(app.localVaultPath)
	if err = app.DeleteICloudBackupWithConfirmation(ctx, func(selected string) bool {
		if selected != path {
			t.Fatal("confirmed wrong path", selected)
		}
		return false
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("cancel deleted backup", err)
	}
	if err = app.DeleteICloudBackupWithConfirmation(ctx, func(selected string) bool { return selected == path }); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cloud backup remains", err)
	}
	after, _ := os.ReadFile(app.localVaultPath)
	if !bytes.Equal(local, after) {
		t.Fatal("cloud deletion touched local vault")
	}
}
