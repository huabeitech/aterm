package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ssh-connect/internal/db"
)

func TestAppServiceUploadSyncSnapshotWritesMetadata(t *testing.T) {
	tmp := t.TempDir()
	store := newMemoryHostStore()
	store.hosts = []Host{{ID: "host-1", Alias: "prod-api", HostName: "10.0.0.12", Port: 22}}
	store.directories = []Directory{{ID: "dir-1", Name: "Production"}}
	app := NewAppService(AppServiceOptions{
		Store:            store,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncMetadataPath: filepath.Join(tmp, "iCloud", "sync", "metadata.json"),
		BackupDir:        filepath.Join(tmp, "backups"),
		DeviceName:       "Test Mac",
	})

	result, err := app.UploadSyncSnapshot(context.Background())
	if err != nil {
		t.Fatalf("UploadSyncSnapshot() error = %v", err)
	}
	if result.HostCount != 1 || result.DirectoryCount != 1 {
		t.Fatalf("UploadSyncSnapshot() result = %+v, want 1 host and 1 directory", result)
	}
	if result.LastUpload == "" {
		t.Fatal("UploadSyncSnapshot() LastUpload is empty")
	}
	if _, err := os.Stat(filepath.Join(tmp, "iCloud", "sync", "app.db")); err != nil {
		t.Fatalf("snapshot stat error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "iCloud", "sync", "metadata.json"))
	if err != nil {
		t.Fatalf("ReadFile(metadata) error = %v", err)
	}
	var metadata SyncMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("metadata json error = %v", err)
	}
	if metadata.SchemaVersion != 1 || metadata.SourceDeviceName != "Test Mac" || metadata.HostCount != 1 || metadata.DirectoryCount != 1 {
		t.Fatalf("metadata = %+v, want schema 1, Test Mac, counts 1/1", metadata)
	}
}

func TestAppServiceEncryptedSyncWritesOpaqueVault(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwords := newMemoryPasswordStore()
	app := NewAppService(AppServiceOptions{
		Store:            store,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncMetadataPath: filepath.Join(tmp, "iCloud", "sync", "metadata.json"),
		SyncVaultPath:    filepath.Join(tmp, "iCloud", "sync", "vault.enc"),
		LocalVaultPath:   filepath.Join(tmp, "vault.enc"),
		BackupDir:        filepath.Join(tmp, "backups"),
		DeviceName:       "Test Mac",
		PasswordStore:    passwords,
	})
	result, err := app.SetUpSecureData(context.Background(), "vault-password")
	if err != nil {
		t.Fatalf("SetUpSecureData() error = %v", err)
	}
	if result.HostCount != 0 {
		t.Fatalf("HostCount = %d, want 0 before hosts are added", result.HostCount)
	}
	host, err := app.CreateHost(context.Background(), HostInput{
		Alias:        "prod-api",
		HostName:     "10.0.0.12",
		Port:         22,
		PasswordAuth: true,
		Password:     "super-secret-password",
		IdentityFile: "~/.ssh/prod.pem",
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "iCloud", "sync", "vault.enc")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cloud vault stat after setup = %v, want not exist", err)
	}
	if _, err := passwords.Get(host.ID); !errors.Is(err, ErrPasswordNotFound) {
		t.Fatalf("host password was written to system store: %v", err)
	}
	if password, err := app.hostPasswordStore.Get(host.ID); err != nil || password != "super-secret-password" {
		t.Fatalf("vault password = %q, %v", password, err)
	}
	if password, err := app.GetHostPassword(context.Background(), host.ID); err != nil || password != "super-secret-password" {
		t.Fatalf("GetHostPassword() = %q, %v", password, err)
	}
	if _, err := app.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatalf("UploadEncryptedSync() error = %v", err)
	}
	status, err := app.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Metadata == nil || status.Metadata.SourceDeviceName != "Test Mac" || status.Metadata.HostCount != 1 || status.Metadata.LastUpload == "" {
		t.Fatalf("encrypted backup metadata = %+v", status.Metadata)
	}
	if status.BackupUpdateRequired {
		t.Fatalf("BackupUpdateRequired = true immediately after upload: %+v", status)
	}
	if _, err := app.DownloadEncryptedSync(context.Background(), "wrong-password"); !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatalf("DownloadEncryptedSync(wrong password) error = %v, want ErrInvalidVaultPassword", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "iCloud", "sync", "vault.enc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, plain := range [][]byte{[]byte("prod-api"), []byte("10.0.0.12"), []byte("super-secret-password"), []byte("prod.pem")} {
		if bytes.Contains(data, plain) {
			t.Fatalf("encrypted vault contains plaintext %q", plain)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "iCloud", "sync", "metadata.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("metadata.json stat error = %v, want not exist", err)
	}
	if err := app.hostPasswordStore.Set(host.ID, "changed-password"); err != nil {
		t.Fatal(err)
	}
	status, err = app.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.BackupUpdateRequired {
		t.Fatalf("BackupUpdateRequired = false after local protected data changed: %+v", status)
	}
}

func TestAppServicePreviewICloudBackupDoesNotReplaceLocalData(t *testing.T) {
	tmp := t.TempDir()
	source, err := db.Open(filepath.Join(tmp, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	vaultPath := filepath.Join(tmp, "iCloud", "sync", "vault.enc")
	sourceApp := NewAppService(AppServiceOptions{
		Store:            source,
		DatabasePath:     filepath.Join(tmp, "source.db"),
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncVaultPath:    vaultPath,
		LocalVaultPath:   filepath.Join(tmp, "source-vault.enc"),
		DeviceName:       "Source Mac",
		PasswordStore:    newMemoryPasswordStore(),
	})
	if _, err := sourceApp.CreateHost(context.Background(), HostInput{Alias: "cloud", HostName: "10.0.0.10", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceApp.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceApp.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatal(err)
	}

	target, err := db.Open(filepath.Join(tmp, "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetApp := NewAppService(AppServiceOptions{
		Store:            target,
		DatabasePath:     filepath.Join(tmp, "target.db"),
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncVaultPath:    vaultPath,
		LocalVaultPath:   filepath.Join(tmp, "target-vault.enc"),
		PasswordStore:    newMemoryPasswordStore(),
	})
	if _, err := targetApp.CreateHost(context.Background(), HostInput{Alias: "local", HostName: "127.0.0.1", Port: 22}); err != nil {
		t.Fatal(err)
	}
	metadata, err := targetApp.PreviewICloudBackup(context.Background(), "vault-password")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.SourceDeviceName != "Source Mac" || metadata.HostCount != 1 || metadata.LastUpload == "" {
		t.Fatalf("preview metadata = %+v", metadata)
	}
	hosts, err := target.ListHosts()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "local" {
		t.Fatalf("hosts after preview = %+v, want unchanged local host", hosts)
	}
}

func TestAppServiceEncryptedSyncUsesLocalVaultDirectoryForTemporaryFiles(t *testing.T) {
	tmp := t.TempDir()
	app := NewAppService(AppServiceOptions{
		Store:          newMemoryHostStore(),
		LocalVaultPath: filepath.Join(tmp, "app-support", "vault.enc"),
		BackupDir:      filepath.Join(tmp, "backups"),
		PasswordStore:  newMemoryPasswordStore(),
	})

	file, err := app.createVaultTempFile("ssh-connect-vault-import-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	expectedDir := filepath.Join(tmp, "app-support", "tmp")
	if filepath.Dir(path) != expectedDir {
		t.Fatalf("vault temp dir = %q, want %q", filepath.Dir(path), expectedDir)
	}
}

func TestAppServiceDownloadEncryptedSyncReplacesLocalData(t *testing.T) {
	tmp := t.TempDir()
	vaultPath := filepath.Join(tmp, "iCloud", "sync", "vault.enc")
	sourceDBPath := filepath.Join(tmp, "source.db")
	source, err := db.Open(sourceDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	sourcePasswords := newMemoryPasswordStore()
	sourceApp := NewAppService(AppServiceOptions{
		Store:            source,
		DatabasePath:     sourceDBPath,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncVaultPath:    vaultPath,
		LocalVaultPath:   filepath.Join(tmp, "source-vault.enc"),
		BackupDir:        filepath.Join(tmp, "source-backups"),
		DeviceName:       "Source Mac",
		PasswordStore:    sourcePasswords,
	})
	if _, err := sourceApp.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	cloudHost, err := sourceApp.CreateHost(context.Background(), HostInput{
		Alias:        "cloud",
		HostName:     "10.0.0.10",
		Port:         22,
		PasswordAuth: true,
		Password:     "cloud-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceApp.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatal(err)
	}

	targetDBPath := filepath.Join(tmp, "target.db")
	target, err := db.Open(targetDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetApp := NewAppService(AppServiceOptions{
		Store:            target,
		DatabasePath:     targetDBPath,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncVaultPath:    vaultPath,
		LocalVaultPath:   filepath.Join(tmp, "target-vault.enc"),
		BackupDir:        filepath.Join(tmp, "target-backups"),
		PasswordStore:    newMemoryPasswordStore(),
	})
	if _, err := targetApp.CreateHost(context.Background(), HostInput{Alias: "local", HostName: "127.0.0.1", Port: 22}); err != nil {
		t.Fatal(err)
	}

	result, err := targetApp.DownloadEncryptedSync(context.Background(), "vault-password")
	if err != nil {
		t.Fatal(err)
	}
	if result.HostCount != 1 || result.DirectoryCount != 0 || result.LastDownload == "" {
		t.Fatalf("DownloadEncryptedSync() result = %+v, want restored counts and download time", result)
	}
	hosts, err := target.ListHosts()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "cloud" || hosts[0].HostName != "10.0.0.10" {
		t.Fatalf("hosts after restore = %+v, want cloud backup host only", hosts)
	}
	password, err := targetApp.GetHostPassword(context.Background(), cloudHost.ID)
	if err != nil || password != "cloud-secret" {
		t.Fatalf("restored host password = %q, %v", password, err)
	}
}

func TestAppServiceEncryptedSyncRejectsTamperedDownload(t *testing.T) {
	tmp := t.TempDir()
	source, err := db.Open(filepath.Join(tmp, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	passwords := newMemoryPasswordStore()
	sourceApp := NewAppService(AppServiceOptions{
		Store:            source,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncMetadataPath: filepath.Join(tmp, "iCloud", "sync", "metadata.json"),
		SyncVaultPath:    filepath.Join(tmp, "iCloud", "sync", "vault.enc"),
		LocalVaultPath:   filepath.Join(tmp, "source-vault.enc"),
		BackupDir:        filepath.Join(tmp, "source-backups"),
		PasswordStore:    passwords,
	})
	if _, err := sourceApp.CreateHost(context.Background(), HostInput{Alias: "source", HostName: "10.0.0.10", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceApp.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceApp.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(tmp, "iCloud", "sync", "vault.enc")
	data, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 0x01
	if err := os.WriteFile(vaultPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := db.Open(filepath.Join(tmp, "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetApp := NewAppService(AppServiceOptions{
		Store:            target,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncMetadataPath: filepath.Join(tmp, "iCloud", "sync", "metadata.json"),
		SyncVaultPath:    vaultPath,
		LocalVaultPath:   filepath.Join(tmp, "target-vault.enc"),
		BackupDir:        filepath.Join(tmp, "target-backups"),
		PasswordStore:    newMemoryPasswordStore(),
	})
	if _, err := targetApp.CreateHost(context.Background(), HostInput{Alias: "local", HostName: "127.0.0.1", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if _, err := targetApp.DownloadEncryptedSync(context.Background(), "vault-password"); !errors.Is(err, ErrVaultCorrupted) && !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatalf("DownloadEncryptedSync() error = %v, want vault auth error", err)
	}
	hosts, err := target.ListHosts()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "local" {
		t.Fatalf("hosts after tampered download = %+v, want unchanged local host", hosts)
	}
}

func TestAppServiceAutoUnlocksSecureDataOnNextLaunch(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwords := newMemoryPasswordStore()
	localVaultPath := filepath.Join(tmp, "vault.enc")
	first := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	if _, err := first.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	host, err := first.CreateHost(context.Background(), HostInput{
		Alias:        "production",
		HostName:     "10.0.0.20",
		Port:         22,
		PasswordAuth: true,
		Password:     "host-password",
	})
	if err != nil {
		t.Fatal(err)
	}

	relaunched := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	relaunched.TryAutoUnlockSecureData(context.Background())
	status, err := relaunched.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Unlocked {
		t.Fatalf("secure data status after relaunch = %+v", status)
	}
	if password, err := relaunched.hostPasswordStore.Get(host.ID); err != nil || password != "host-password" {
		t.Fatalf("vault password after relaunch = %q, %v", password, err)
	}
}

func TestAppServiceAutoUnlockReportsMissingVaultAndKeychainFailures(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwords := newMemoryPasswordStore()
	localVaultPath := filepath.Join(tmp, "vault.enc")
	app := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	if result := app.TryAutoUnlockSecureData(context.Background()); result.Status != AutoUnlockSkippedNoVault {
		t.Fatalf("auto unlock without vault = %+v, want %s", result, AutoUnlockSkippedNoVault)
	}
	if _, err := app.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}

	noKey := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	if err := passwords.Delete(VaultUnlockAccount); err != nil {
		t.Fatal(err)
	}
	if result := noKey.TryAutoUnlockSecureData(context.Background()); result.Status != AutoUnlockSkippedNoKey {
		t.Fatalf("auto unlock without key = %+v, want %s", result, AutoUnlockSkippedNoKey)
	}

	denied := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  &failingPasswordStore{PasswordStore: passwords, getErrors: map[string]error{VaultUnlockAccount: ErrKeychainAccessDenied}},
	})
	if result := denied.TryAutoUnlockSecureData(context.Background()); result.Status != AutoUnlockKeychainDenied {
		t.Fatalf("auto unlock with denied keychain = %+v, want %s", result, AutoUnlockKeychainDenied)
	}
}

func TestAppServiceAutoUnlockReportsInvalidKey(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwords := newMemoryPasswordStore()
	localVaultPath := filepath.Join(tmp, "vault.enc")
	app := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	if _, err := app.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	if err := passwords.Set(VaultUnlockAccount, "not-base64"); err != nil {
		t.Fatal(err)
	}
	relaunched := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	if result := relaunched.TryAutoUnlockSecureData(context.Background()); result.Status != AutoUnlockInvalidKey {
		t.Fatalf("auto unlock with invalid key = %+v, want %s", result, AutoUnlockInvalidKey)
	}
	status, err := relaunched.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.AutoUnlockRepairRequired || status.AutoUnlockStatus != AutoUnlockInvalidKey {
		t.Fatalf("status after invalid auto unlock = %+v", status)
	}
}

func TestAppServiceRejectsPasswordHostWhenVaultIsLocked(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwords := newMemoryPasswordStore()
	app := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: filepath.Join(tmp, "vault.enc"),
		PasswordStore:  passwords,
	})
	if _, err := app.CreateHost(context.Background(), HostInput{
		Alias:        "production",
		HostName:     "10.0.0.20",
		Port:         22,
		PasswordAuth: true,
		Password:     "host-password",
	}); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("CreateHost() error = %v, want ErrVaultLocked", err)
	}
	if _, err := app.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	host, err := app.CreateHost(context.Background(), HostInput{Alias: "production", HostName: "10.0.0.20", Port: 22, PasswordAuth: true, Password: "host-password"})
	if err != nil {
		t.Fatal(err)
	}
	if password, err := app.hostPasswordStore.Get(host.ID); err != nil || password != "host-password" {
		t.Fatalf("vault password = %q, %v", password, err)
	}
	if _, err := passwords.Get(host.ID); !errors.Is(err, ErrPasswordNotFound) {
		t.Fatalf("host password was written to system store: %v", err)
	}
}

func TestAppServiceUnlockRepairDoesNotReadKeychainAfterWritingAutoUnlockKey(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwords := newMemoryPasswordStore()
	localVaultPath := filepath.Join(tmp, "vault.enc")
	first := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  passwords,
	})
	if _, err := first.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}

	relaunched := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore: &failingPasswordStore{
			PasswordStore: passwords,
			getErrors:     map[string]error{VaultUnlockAccount: ErrKeychainAccessDenied},
		},
	})
	if _, err := relaunched.UnlockSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatalf("UnlockSecureData() error = %v", err)
	}
	status, err := relaunched.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Unlocked || status.AutoUnlockStatus != AutoUnlockUnlocked {
		t.Fatalf("status after unlock repair = %+v, want unlocked auto unlock ready", status)
	}
}

func TestAppServiceDeleteEncryptedSyncKeepsLocalVault(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	localVaultPath := filepath.Join(tmp, "vault.enc")
	cloudVaultPath := filepath.Join(tmp, "iCloud", "sync", "vault.enc")
	app := NewAppService(AppServiceOptions{
		Store:            store,
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncVaultPath:    cloudVaultPath,
		LocalVaultPath:   localVaultPath,
		BackupDir:        filepath.Join(tmp, "backups"),
		PasswordStore:    newMemoryPasswordStore(),
	})
	if _, err := app.SetUpSecureData(context.Background(), "vault-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteICloudBackup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cloudVaultPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cloud vault stat = %v, want not exist", err)
	}
	if _, err := os.Stat(localVaultPath); err != nil {
		t.Fatalf("local vault was removed: %v", err)
	}
	status, err := app.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.LocalVaultAvailable || !status.Unlocked {
		t.Fatalf("secure data status after deleting cloud backup = %+v", status)
	}
}

func TestAppServiceUploadEncryptedSyncDoesNotOverwriteDifferentCloudVault(t *testing.T) {
	tmp := t.TempDir()
	cloudVaultPath := filepath.Join(tmp, "iCloud", "sync", "vault.enc")
	newApp := func(name string) (*AppService, *db.Store) {
		appDir := filepath.Join(tmp, name)
		if err := os.MkdirAll(appDir, 0o700); err != nil {
			t.Fatal(err)
		}
		store, err := db.Open(filepath.Join(appDir, "app.db"))
		if err != nil {
			t.Fatal(err)
		}
		app := NewAppService(AppServiceOptions{
			Store:            store,
			DatabasePath:     filepath.Join(appDir, "app.db"),
			SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
			SyncVaultPath:    cloudVaultPath,
			LocalVaultPath:   filepath.Join(appDir, "vault.enc"),
			PasswordStore:    newMemoryPasswordStore(),
		})
		return app, store
	}

	first, firstStore := newApp("first")
	defer firstStore.Close()
	if _, err := first.SetUpSecureData(context.Background(), "first-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	originalCloudData, err := os.ReadFile(cloudVaultPath)
	if err != nil {
		t.Fatal(err)
	}

	second, secondStore := newApp("second")
	defer secondStore.Close()
	if _, err := second.SetUpSecureData(context.Background(), "second-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.UploadEncryptedSync(context.Background()); !errors.Is(err, ErrCloudVaultConflict) {
		t.Fatalf("UploadEncryptedSync() error = %v, want ErrCloudVaultConflict", err)
	}
	cloudData, err := os.ReadFile(cloudVaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cloudData, originalCloudData) {
		t.Fatal("different cloud vault was overwritten")
	}
}

func TestAppServiceChangeMasterPasswordDoesNotRequireCloudStorage(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	localVaultPath := filepath.Join(tmp, "vault.enc")
	app := NewAppService(AppServiceOptions{
		Store:          store,
		DatabasePath:   filepath.Join(tmp, "app.db"),
		LocalVaultPath: localVaultPath,
		PasswordStore:  newMemoryPasswordStore(),
	})
	if _, err := app.SetUpSecureData(context.Background(), "old-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ChangeEncryptionPassword(context.Background(), "old-password", "new-password"); err != nil {
		t.Fatalf("ChangeEncryptionPassword() error = %v", err)
	}
	data, err := os.ReadFile(localVaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.vaultStore.Unlock(data, "new-password"); err != nil {
		t.Fatalf("Unlock(new password) error = %v", err)
	}
	if _, _, err := app.vaultStore.Unlock(data, "old-password"); !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatalf("Unlock(old password) error = %v, want ErrInvalidVaultPassword", err)
	}
}

func TestAppServiceChangeMasterPasswordUpdatesExistingICloudBackup(t *testing.T) {
	tmp := t.TempDir()
	store, err := db.Open(filepath.Join(tmp, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	vaultPath := filepath.Join(tmp, "iCloud", "sync", "vault.enc")
	app := NewAppService(AppServiceOptions{
		Store:            store,
		DatabasePath:     filepath.Join(tmp, "app.db"),
		SyncSnapshotPath: filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncVaultPath:    vaultPath,
		LocalVaultPath:   filepath.Join(tmp, "vault.enc"),
		PasswordStore:    newMemoryPasswordStore(),
	})
	if _, err := app.SetUpSecureData(context.Background(), "old-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UploadEncryptedSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := app.ChangeEncryptionPassword(context.Background(), "old-password", "new-password")
	if err != nil {
		t.Fatal(err)
	}
	if !result.BackupUpdated || result.BackupUpdateRequired {
		t.Fatalf("change password result = %+v", result)
	}
	cloudData, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.vaultStore.Unlock(cloudData, "new-password"); err != nil {
		t.Fatalf("Unlock cloud backup with new password error = %v", err)
	}
	if _, _, err := app.vaultStore.Unlock(cloudData, "old-password"); !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatalf("Unlock cloud backup with old password error = %v, want ErrInvalidVaultPassword", err)
	}
	status, err := app.GetEncryptedSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.BackupUpdateRequired {
		t.Fatalf("BackupUpdateRequired = true after automatic update: %+v", status)
	}
}

type memoryPasswordStore struct {
	values map[string]string
}

func newMemoryPasswordStore() *memoryPasswordStore {
	return &memoryPasswordStore{values: map[string]string{}}
}

func (s *memoryPasswordStore) Get(hostID string) (string, error) {
	value, ok := s.values[hostID]
	if !ok {
		return "", ErrPasswordNotFound
	}
	return value, nil
}

func (s *memoryPasswordStore) Set(hostID, password string) error {
	s.values[hostID] = password
	return nil
}

func (s *memoryPasswordStore) Delete(hostID string) error {
	delete(s.values, hostID)
	return nil
}

type failingPasswordStore struct {
	PasswordStore
	getErrors map[string]error
	setErrors map[string]error
	delErrors map[string]error
}

func (s *failingPasswordStore) Get(hostID string) (string, error) {
	if err := s.getErrors[hostID]; err != nil {
		return "", err
	}
	return s.PasswordStore.Get(hostID)
}

func (s *failingPasswordStore) Set(hostID, password string) error {
	if err := s.setErrors[hostID]; err != nil {
		return err
	}
	return s.PasswordStore.Set(hostID, password)
}

func (s *failingPasswordStore) Delete(hostID string) error {
	if err := s.delErrors[hostID]; err != nil {
		return err
	}
	return s.PasswordStore.Delete(hostID)
}

func TestAppServiceGetSyncStatusUsesDefaultSyncDirectoryWithoutAuthorization(t *testing.T) {
	tmp := t.TempDir()
	syncRoot := filepath.Join(tmp, "iCloud")
	if err := os.MkdirAll(filepath.Join(syncRoot, "aTerm", "sync"), 0o700); err != nil {
		t.Fatalf("MkdirAll() sync dir error = %v", err)
	}
	app := NewAppService(AppServiceOptions{
		Store:            newMemoryHostStore(),
		SyncRootPath:     syncRoot,
		SyncSnapshotPath: filepath.Join(syncRoot, "aTerm", "sync", "app.db"),
		SyncMetadataPath: filepath.Join(syncRoot, "aTerm", "sync", "metadata.json"),
	})

	status, err := app.GetSyncStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSyncStatus() error = %v", err)
	}

	if macAppStoreBuild {
		if !status.SyncDirectoryAuthorizationRequired {
			t.Fatalf("SyncDirectoryAuthorizationRequired = false, want true in MAS build: %+v", status)
		}
		if status.SyncDirectoryAuthorized {
			t.Fatalf("SyncDirectoryAuthorized = true, want false before MAS authorization: %+v", status)
		}
		return
	}

	if status.SyncDirectoryAuthorizationRequired {
		t.Fatalf("SyncDirectoryAuthorizationRequired = true, want false: %+v", status)
	}
	if !status.SyncDirectoryAuthorized {
		t.Fatalf("SyncDirectoryAuthorized = false, want true: %+v", status)
	}
	if !status.CloudAvailable {
		t.Fatalf("CloudAvailable = false, want true: %+v", status)
	}
	if status.SyncDirectoryPath != syncRoot {
		t.Fatalf("SyncDirectoryPath = %q, want %q", status.SyncDirectoryPath, syncRoot)
	}
}

func TestAppServiceAuthorizeSyncDirectoryAllowsStatusRead(t *testing.T) {
	tmp := t.TempDir()
	syncRoot := filepath.Join(tmp, "iCloud")
	syncDir := filepath.Join(syncRoot, "aTerm", "sync")
	if err := os.MkdirAll(syncDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() sync dir error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "app.db"), []byte("snapshot"), 0o600); err != nil {
		t.Fatalf("WriteFile(snapshot) error = %v", err)
	}
	if err := writeSyncMetadata(filepath.Join(syncDir, "metadata.json"), SyncMetadata{
		SchemaVersion:     syncSchemaVersion,
		AppName:           "aTerm",
		SnapshotCreatedAt: "2026-07-08T00:00:00Z",
		SourceDeviceName:  "Test Mac",
		HostCount:         2,
		DirectoryCount:    1,
		LastUpload:        "2026-07-08T00:00:00Z",
	}); err != nil {
		t.Fatalf("writeSyncMetadata() error = %v", err)
	}
	app := NewAppService(AppServiceOptions{
		Store:            newMemoryHostStore(),
		SyncRootPath:     syncRoot,
		SyncSnapshotPath: filepath.Join(syncDir, "app.db"),
		SyncMetadataPath: filepath.Join(syncDir, "metadata.json"),
	})

	if _, err := app.AuthorizeSyncDirectory(context.Background(), syncRoot); err != nil {
		t.Fatalf("AuthorizeSyncDirectory() error = %v", err)
	}
	status, err := app.GetSyncStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSyncStatus() error = %v", err)
	}

	if status.SyncDirectoryAuthorizationRequired || !status.SyncDirectoryAuthorized || !status.CloudAvailable || !status.SnapshotAvailable {
		t.Fatalf("status = %+v, want authorized cloud snapshot", status)
	}
	if status.Metadata == nil || status.Metadata.HostCount != 2 {
		t.Fatalf("metadata = %+v, want host count 2", status.Metadata)
	}
}

func TestAppServiceAuthorizeSyncDirectoryRejectsUnexpectedDirectory(t *testing.T) {
	tmp := t.TempDir()
	syncRoot := filepath.Join(tmp, "iCloud")
	otherDir := filepath.Join(tmp, "Documents")
	if err := os.MkdirAll(otherDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() other dir error = %v", err)
	}
	app := NewAppService(AppServiceOptions{
		Store:        newMemoryHostStore(),
		SyncRootPath: syncRoot,
	})

	_, err := app.AuthorizeSyncDirectory(context.Background(), otherDir)

	if !errors.Is(err, ErrUnexpectedSyncDirectory) {
		t.Fatalf("AuthorizeSyncDirectory() error = %v, want ErrUnexpectedSyncDirectory", err)
	}
}

func TestAppServiceDownloadSyncSnapshotCreatesBackupAndUpdatesStatus(t *testing.T) {
	tmp := t.TempDir()
	store := newMemoryHostStore()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               store,
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		SyncSnapshotPath:    filepath.Join(tmp, "iCloud", "sync", "app.db"),
		SyncMetadataPath:    filepath.Join(tmp, "iCloud", "sync", "metadata.json"),
		BackupDir:           filepath.Join(tmp, "backups"),
		DeviceName:          "Test Mac",
	})
	authorizeTestSSHDirectory(t, app, sshDir)
	store.hosts = []Host{{
		ID:         "cloud",
		Alias:      "cloud-api",
		HostName:   "10.0.0.20",
		Port:       22,
		Favorite:   true,
		LastUsedAt: "2026-07-11T08:00:00Z",
	}}
	if _, err := app.UploadSyncSnapshot(context.Background()); err != nil {
		t.Fatalf("UploadSyncSnapshot() seed error = %v", err)
	}
	store.hosts = []Host{{ID: "local", Alias: "local-only", HostName: "127.0.0.1", Port: 22}}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() seed error = %v", err)
	}

	result, err := app.DownloadSyncSnapshot(context.Background())
	if err != nil {
		t.Fatalf("DownloadSyncSnapshot() error = %v", err)
	}
	if result.LastDownload == "" {
		t.Fatal("DownloadSyncSnapshot() LastDownload is empty")
	}
	if result.BackupPath == "" {
		t.Fatal("DownloadSyncSnapshot() BackupPath is empty")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup stat error = %v", err)
	}
	if len(store.importedSnapshots) != 1 {
		t.Fatalf("importedSnapshots len = %d, want 1", len(store.importedSnapshots))
	}
	if len(store.hosts) != 1 || !store.hosts[0].Favorite || store.hosts[0].LastUsedAt != "2026-07-11T08:00:00Z" {
		t.Fatalf("downloaded host state = %+v, want synced favorite and recent-use state", store.hosts)
	}

	status, err := app.GetSyncStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSyncStatus() error = %v", err)
	}
	if !status.CloudAvailable || !status.SnapshotAvailable {
		t.Fatalf("status = %+v, want available cloud snapshot", status)
	}
	if status.LastDownload == "" || status.LastUpload == "" {
		t.Fatalf("status sync times = %+v, want upload and download", status)
	}
	config := readGeneratedConfig(t, sshDir)
	if !strings.Contains(config, "Host local-only\n") || strings.Contains(config, "Host cloud-api\n") {
		t.Fatalf("generated config after download = %q, want prior manual snapshot", config)
	}
}
