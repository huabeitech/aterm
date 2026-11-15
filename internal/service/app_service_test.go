package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-connect/internal/fsutil"
)

func authorizeTestSSHDirectory(t *testing.T, app *AppService, sshDir string) {
	t.Helper()
	if _, err := app.UpdateSettings(context.Background(), AppSettingsInput{
		SSHDirectoryPath:              sshDir,
		SSHDirectoryBookmark:          sshDir,
		SSHDirectoryBookmarkCreatedAt: "2026-07-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateSettings() authorization error = %v", err)
	}
}

func readGeneratedConfig(t *testing.T, sshDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"))
	if err != nil {
		t.Fatalf("ReadFile() generated config error = %v", err)
	}
	return string(data)
}

func TestAppServiceHostMutationsDoNotExportAutomatically(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{Store: newMemoryHostStore(), SSHDir: sshDir, ConfigPath: filepath.Join(sshDir, "config"), GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")})
	if _, err := app.UpdateSettings(context.Background(), AppSettingsInput{SSHDirectoryPath: sshDir, SSHDirectoryBookmark: sshDir}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if _, err := app.CreateHost(context.Background(), HostInput{Alias: "private-api", HostName: "10.0.0.8", Port: 22}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated config stat error = %v, want no exported config", err)
	}
}

func TestAppServiceStoresSelectedJumpHost(t *testing.T) {
	app := NewAppService(AppServiceOptions{Store: newMemoryHostStore()})
	jump, err := app.CreateHost(context.Background(), HostInput{Alias: "bastion", HostName: "10.0.0.2", Port: 22})
	if err != nil {
		t.Fatalf("CreateHost() jump error = %v", err)
	}
	target, err := app.CreateHost(context.Background(), HostInput{Alias: "private-api", HostName: "10.0.0.8", Port: 22, JumpHostID: jump.ID})
	if err != nil {
		t.Fatalf("CreateHost() target error = %v", err)
	}
	if target.JumpHostID != jump.ID || target.ProxyJump != jump.Alias {
		t.Fatalf("target jump = %+v, want %q", target, jump.ID)
	}
	if _, err := app.UpdateHost(context.Background(), jump.ID, HostInput{Alias: jump.Alias, HostName: jump.HostName, Port: jump.Port, JumpHostID: target.ID}); err == nil {
		t.Fatal("UpdateHost() cycle error = nil, want rejection")
	}
}

func skipIfConnectionFeaturesUnavailable(t *testing.T) {
	t.Helper()
	if macAppStoreBuild {
		t.Skip("Terminal connection features are unavailable in MAS builds")
	}
}

func TestAppServiceManualExportSetsUpIncludeAndWritesSnapshot(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{
		Store:               store,
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)

	status, err := app.OpenSSHExportStatus(context.Background())
	if err != nil {
		t.Fatalf("OpenSSHExportStatus() error = %v", err)
	}
	if status.IncludeConfigured {
		t.Fatal("IncludeConfigured = true, want false before export")
	}

	if _, err := app.CreateHost(context.Background(), HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		User:     "ubuntu",
		Port:     22,
	}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated config stat before export = %v, want no file", err)
	}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}
	status, err = app.OpenSSHExportStatus(context.Background())
	if err != nil {
		t.Fatalf("OpenSSHExportStatus() after export error = %v", err)
	}
	if !status.IncludeConfigured {
		t.Fatal("IncludeConfigured = false, want true after export")
	}
	if !status.SnapshotExists {
		t.Fatal("SnapshotExists = false, want true after export")
	}
	if status.LastExportedAt == "" {
		t.Fatal("LastExportedAt is empty after export")
	}

	data, err := os.ReadFile(filepath.Join(tmp, ".ssh", "config.d", "ssh-connect.generated.conf"))
	if err != nil {
		t.Fatalf("ReadFile() generated config error = %v", err)
	}
	if string(data) == "" {
		t.Fatal("generated config is empty")
	}
}

func TestAppServiceManualExportSnapshotFailureDoesNotModifyInclude(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	configPath := filepath.Join(sshDir, "config")
	generatedPath := filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("Host existing\n  HostName example.com\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          configPath,
		GeneratedConfigPath: generatedPath,
	})
	authorizeTestSSHDirectory(t, app, sshDir)
	app.SSHConfigService.atomicWriteFile = func(path string, data []byte, mode os.FileMode) error {
		if path == generatedPath {
			return errors.New("disk full")
		}
		return fsutil.AtomicWriteFile(path, data, mode)
	}

	err := app.ExportOpenSSHConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), "write OpenSSH snapshot") {
		t.Fatalf("ExportOpenSSHConfig() error = %v, want snapshot write error", err)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("SSH config changed after snapshot failure\nbefore: %q\nafter: %q", original, after)
	}
}

func TestAppServiceManualExportReportsSnapshotWhenIncludeUpdateFails(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	configPath := filepath.Join(sshDir, "config")
	generatedPath := filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          configPath,
		GeneratedConfigPath: generatedPath,
	})
	authorizeTestSSHDirectory(t, app, sshDir)
	app.SSHConfigService.atomicWriteFile = func(path string, data []byte, mode os.FileMode) error {
		if path == configPath {
			return errors.New("config is read-only")
		}
		return fsutil.AtomicWriteFile(path, data, mode)
	}

	err := app.ExportOpenSSHConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), "snapshot was created") || !strings.Contains(err.Error(), "Include line could not be updated") {
		t.Fatalf("ExportOpenSSHConfig() error = %v, want partial-success details", err)
	}
	if _, statErr := os.Stat(generatedPath); statErr != nil {
		t.Fatalf("generated snapshot missing after partial success: %v", statErr)
	}
}

func TestAppServiceStatusReportsDistributionFeatures(t *testing.T) {
	app := NewAppService(AppServiceOptions{Store: newMemoryHostStore()})

	status, err := app.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.MacAppStoreBuild != macAppStoreBuild {
		t.Fatalf("MacAppStoreBuild = %v, want %v", status.MacAppStoreBuild, macAppStoreBuild)
	}
	if !status.ConnectionFeaturesAvailable {
		t.Fatal("ConnectionFeaturesAvailable = false, want true")
	}
	if status.LocalTerminalAvailable != LocalTerminalAvailable() {
		t.Fatalf("LocalTerminalAvailable = %v, want %v", status.LocalTerminalAvailable, LocalTerminalAvailable())
	}
}

func TestAppServiceHostMutationsLeaveManualExportUnchanged(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)

	created, err := app.CreateHost(context.Background(), HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		User:     "ubuntu",
		Port:     22,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}
	config := readGeneratedConfig(t, sshDir)
	if !strings.Contains(config, "Host prod-api\n") || !strings.Contains(config, "HostName 10.0.0.12\n") {
		t.Fatalf("generated config after create = %q, want prod-api host", config)
	}

	if _, err := app.UpdateHost(context.Background(), created.ID, HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.13",
		User:     "root",
		Port:     2200,
	}); err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}
	config = readGeneratedConfig(t, sshDir)
	if !strings.Contains(config, "HostName 10.0.0.12\n") || strings.Contains(config, "HostName 10.0.0.13\n") {
		t.Fatalf("generated config after update = %q, want prior manual snapshot", config)
	}

	if err := app.DeleteHost(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	config = readGeneratedConfig(t, sshDir)
	if !strings.Contains(config, "Host prod-api\n") {
		t.Fatalf("generated config after delete = %q, want prior manual snapshot", config)
	}
}

func TestAppServiceBatchDeleteLeavesManualExportUnchanged(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)
	first, err := app.CreateHost(context.Background(), HostInput{Alias: "batch-one", HostName: "10.0.0.21", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateHost(context.Background(), HostInput{Alias: "batch-two", HostName: "10.0.0.22", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}
	before := readGeneratedConfig(t, sshDir)
	if err := app.DeleteHosts(context.Background(), []string{first.ID, second.ID}); err != nil {
		t.Fatalf("DeleteHosts() error = %v", err)
	}
	if after := readGeneratedConfig(t, sshDir); after != before {
		t.Fatalf("generated config changed after batch delete\nbefore: %q\nafter: %q", before, after)
	}
}

func TestAppServiceImportHostSnippetDoesNotExportAutomatically(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)
	if _, err := app.CreateHost(context.Background(), HostInput{Alias: "snapshot-api", HostName: "10.0.0.19", Port: 22}); err != nil {
		t.Fatalf("CreateHost() seed error = %v", err)
	}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() seed error = %v", err)
	}
	before := readGeneratedConfig(t, sshDir)

	_, err := app.ImportHostSnippet(context.Background(), HostSnippetImportInput{
		Content: "Host batch-api\n  HostName 10.0.0.20\n  User deploy\n",
		RowIDs:  []string{"host-1"},
	})
	if err != nil {
		t.Fatalf("ImportHostSnippet() error = %v", err)
	}

	if after := readGeneratedConfig(t, sshDir); after != before {
		t.Fatalf("generated config changed after batch import\nbefore: %q\nafter: %q", before, after)
	}
}

func TestAppServiceAuthorizeSSHDirectoryDoesNotExportAutomatically(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	if _, err := app.CreateHost(context.Background(), HostInput{
		Alias:    "auth-api",
		HostName: "10.0.0.30",
		User:     "deploy",
		Port:     22,
	}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if _, err := app.AuthorizeSSHDirectory(context.Background(), sshDir); err != nil {
		t.Fatalf("AuthorizeSSHDirectory() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated config stat after first authorization = %v, want no file", err)
	}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() seed error = %v", err)
	}
	before := readGeneratedConfig(t, sshDir)
	if _, err := app.AuthorizeSSHDirectory(context.Background(), sshDir); err != nil {
		t.Fatalf("AuthorizeSSHDirectory() reauthorization error = %v", err)
	}
	if after := readGeneratedConfig(t, sshDir); after != before {
		t.Fatalf("generated config changed after reauthorization\nbefore: %q\nafter: %q", before, after)
	}
}

func TestAppServiceExportOpenSSHConfigSerializesConcurrentWrites(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	store := &renderConcurrencyStore{memoryHostStore: newMemoryHostStore()}
	store.hosts = []Host{{
		ID:       "prod-api",
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		Port:     22,
	}}
	app := NewAppService(AppServiceOptions{
		Store:               store,
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- app.ExportOpenSSHConfig(context.Background())
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ExportOpenSSHConfig() concurrent error = %v", err)
		}
	}
	if store.maxActive > 1 {
		t.Fatalf("ExportOpenSSHConfig() ran %d concurrent ListHosts calls, want serialized writes", store.maxActive)
	}
}

func TestAppServiceStatusUsesDefaultSSHDirectoryWithoutAuthorization(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{
		Store:               store,
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})

	status, err := app.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config stat after Status() = %v, want no file", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated config stat after Status() = %v, want no file", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "known_hosts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("known_hosts stat after Status() = %v, want no file", err)
	}
	if macAppStoreBuild {
		if status.SSHDirectoryAuthorized {
			t.Fatal("SSHDirectoryAuthorized = true, want false before MAS authorization")
		}
		if !status.SSHDirectoryAuthorizationRequired {
			t.Fatal("SSHDirectoryAuthorizationRequired = false, want true before MAS authorization")
		}
		return
	}
	if !status.SSHDirectoryAuthorized {
		t.Fatal("SSHDirectoryAuthorized = false, want true")
	}
	if status.SSHDirectoryAuthorizationRequired {
		t.Fatal("SSHDirectoryAuthorizationRequired = true, want false")
	}
	if status.SSHDirectoryPath != sshDir {
		t.Fatalf("SSHDirectoryPath = %q, want %q", status.SSHDirectoryPath, sshDir)
	}
}

func TestAppServiceManualExportUsesDefaultSSHDirectoryWithoutAuthorization(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})

	if macAppStoreBuild {
		err := app.ExportOpenSSHConfig(context.Background())
		if !errors.Is(err, ErrSSHDirectoryAuthorizationRequired) {
			t.Fatalf("ExportOpenSSHConfig() error = %v, want ErrSSHDirectoryAuthorizationRequired before MAS authorization", err)
		}
		return
	}

	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(sshDir, "config"))
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(string(data), "Include ~/.ssh/config.d/*.conf") {
		t.Fatalf("config = %q, want generated include", string(data))
	}
}

func TestAppServiceMASAllowsSSHConfigAfterAuthorization(t *testing.T) {
	if !macAppStoreBuild {
		t.Skip("MAS-only behavior")
	}
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)

	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")); err != nil {
		t.Fatalf("generated config stat error = %v", err)
	}
}

func TestAppServiceUsesAuthorizedSSHDirectory(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{
		Store:               store,
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	if _, err := app.UpdateSettings(context.Background(), AppSettingsInput{
		SSHDirectoryPath:              sshDir,
		SSHDirectoryBookmark:          sshDir,
		SSHDirectoryBookmarkCreatedAt: "2026-07-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	status, err := app.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.SSHDirectoryAuthorized {
		t.Fatal("SSHDirectoryAuthorized = false, want true")
	}
	if status.SSHDirectoryPath != sshDir {
		t.Fatalf("SSHDirectoryPath = %q, want authorized directory", status.SSHDirectoryPath)
	}
	if _, err := app.CreateHost(context.Background(), HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		Port:     22,
	}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")); err != nil {
		t.Fatalf("generated config stat error = %v", err)
	}
}

func TestAppServiceAuthorizeSSHDirectoryRejectsUnexpectedDirectory(t *testing.T) {
	tmp := t.TempDir()
	expectedSSHDir := filepath.Join(tmp, ".ssh")
	otherDir := filepath.Join(tmp, "Documents")
	if err := os.MkdirAll(otherDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() otherDir error = %v", err)
	}
	app := NewAppService(AppServiceOptions{
		Store:                newMemoryHostStore(),
		ExpectedSSHDirectory: expectedSSHDir,
	})

	_, err := app.AuthorizeSSHDirectory(context.Background(), otherDir)

	if !errors.Is(err, ErrUnexpectedSSHDirectory) {
		t.Fatalf("AuthorizeSSHDirectory() error = %v, want ErrUnexpectedSSHDirectory", err)
	}
}

func TestAppServiceAuthorizeSSHDirectoryCreatesExpectedDirectoryWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	expectedSSHDir := filepath.Join(tmp, ".ssh")
	app := NewAppService(AppServiceOptions{
		Store:                newMemoryHostStore(),
		ExpectedSSHDirectory: expectedSSHDir,
	})

	settings, err := app.AuthorizeSSHDirectory(context.Background(), expectedSSHDir)

	if err != nil {
		t.Fatalf("AuthorizeSSHDirectory() error = %v", err)
	}
	if settings.SSHDirectoryPath != expectedSSHDir {
		t.Fatalf("SSHDirectoryPath = %q, want %q", settings.SSHDirectoryPath, expectedSSHDir)
	}
	info, err := os.Stat(expectedSSHDir)
	if err != nil {
		t.Fatalf("Stat() expected SSH directory error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", expectedSSHDir)
	}
}

func TestAppServiceManualExportMovesIncludeBeforeFirstHost(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	configPath := filepath.Join(sshDir, "config")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	initialConfig := "Host github2.com\n\tHostName github.com\n\n# Added by TermSSH\n" + IncludeLine + "\n"
	if err := os.WriteFile(configPath, []byte(initialConfig), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app := NewAppService(AppServiceOptions{
		Store:               newMemoryHostStore(),
		SSHDir:              sshDir,
		ConfigPath:          configPath,
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)

	if err := app.ExportOpenSSHConfig(context.Background()); err != nil {
		t.Fatalf("ExportOpenSSHConfig() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if strings.Count(got, IncludeLine) != 1 {
		t.Fatalf("Include count = %d, config = %q", strings.Count(got, IncludeLine), got)
	}
	if !strings.Contains(got, "# Added by aTerm") {
		t.Fatalf("updated config should use the current app name: %q", got)
	}
	if strings.Index(got, IncludeLine) > strings.Index(got, "Host github2.com") {
		t.Fatalf("Include line appears after first Host block: %q", got)
	}
}

func TestAppServiceMASConnectionFeaturesAvailable(t *testing.T) {
	if !macAppStoreBuild {
		t.Skip("MAS-only behavior")
	}
	app := NewAppService(AppServiceOptions{Store: newMemoryHostStore()})

	_, err := app.PrepareTerminalConnection(context.Background(), "prod-api")

	if !errors.Is(err, ErrHostNotFound) {
		t.Fatalf("PrepareTerminalConnection() error = %v, want ErrHostNotFound", err)
	}
}

func TestAppServicePrepareTerminalConnectionMarksHostUsed(t *testing.T) {
	skipIfConnectionFeaturesUnavailable(t)
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{
		Store:               store,
		SSHDir:              sshDir,
		ConfigPath:          filepath.Join(sshDir, "config"),
		GeneratedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
		DatabasePath:        filepath.Join(tmp, "app.db"),
	})
	authorizeTestSSHDirectory(t, app, sshDir)

	created, err := app.CreateHost(context.Background(), HostInput{
		Alias:    "prod-api",
		HostName: "10.0.0.12",
		Port:     22,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if _, err := app.PrepareTerminalConnection(context.Background(), "prod-api"); err != nil {
		t.Fatalf("PrepareTerminalConnection() error = %v", err)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("ListHosts() len = %d, want 1", len(hosts))
	}
	if hosts[0].ID != created.ID {
		t.Fatalf("used host ID = %q, want %q", hosts[0].ID, created.ID)
	}
	if hosts[0].LastUsedAt == "" {
		t.Fatal("LastUsedAt is empty after preparing the built-in terminal")
	}
}

func TestAppServiceSettingsDefaultsAndUpdates(t *testing.T) {
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{Store: store})

	settings, err := app.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if settings.CommandPaletteShortcut != DefaultCommandPaletteShortcut {
		t.Fatalf("default CommandPaletteShortcut = %q, want %q", settings.CommandPaletteShortcut, DefaultCommandPaletteShortcut)
	}
	if settings.DefaultHome != "hosts" {
		t.Fatalf("default DefaultHome = %q, want hosts", settings.DefaultHome)
	}
	updated, err := app.UpdateSettings(context.Background(), AppSettingsInput{
		CommandPaletteShortcut: "meta+shift+p",
		DefaultHome:            "local-terminal",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if updated.CommandPaletteShortcut != "meta+shift+p" || updated.DefaultHome != "local-terminal" {
		t.Fatalf("UpdateSettings() = %+v, want updated command palette shortcut and default home", updated)
	}

	again, err := app.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() after update error = %v", err)
	}
	if again != updated {
		t.Fatalf("GetSettings() after update = %+v, want %+v", again, updated)
	}
}

func TestDefaultAppSettingsUsesShortcutDefaults(t *testing.T) {
	settings := DefaultAppSettings()

	if settings.CommandPaletteShortcut != DefaultCommandPaletteShortcut {
		t.Fatalf("default CommandPaletteShortcut = %q, want %q", settings.CommandPaletteShortcut, DefaultCommandPaletteShortcut)
	}
	if settings.NewTerminalShortcut != DefaultNewTerminalShortcut ||
		settings.CloseTerminalShortcut != DefaultCloseTerminalShortcut ||
		settings.PreviousTerminalShortcut != DefaultPreviousTerminalShortcut ||
		settings.NextTerminalShortcut != DefaultNextTerminalShortcut {
		t.Fatalf("default terminal shortcuts = %+v, want built-in tab shortcut defaults", settings)
	}
}

func TestNormalizeAppSettingsInputNormalizesTerminalTabShortcuts(t *testing.T) {
	settings, err := NormalizeAppSettingsInput(AppSettingsInput{
		NewTerminalShortcut:      "meta + shift + t",
		CloseTerminalShortcut:    "ctrl+w",
		PreviousTerminalShortcut: "meta + arrowleft",
		NextTerminalShortcut:     "meta + arrowright",
	})
	if err != nil {
		t.Fatalf("NormalizeAppSettingsInput() error = %v", err)
	}
	if settings.NewTerminalShortcut != "meta+shift+t" ||
		settings.CloseTerminalShortcut != "ctrl+w" ||
		settings.PreviousTerminalShortcut != "meta+arrowleft" ||
		settings.NextTerminalShortcut != "meta+arrowright" {
		t.Fatalf("terminal shortcuts = %+v, want normalized values", settings)
	}
}

func TestNormalizeAppSettingsInputRejectsDuplicateShortcuts(t *testing.T) {
	_, err := NormalizeAppSettingsInput(AppSettingsInput{
		CommandPaletteShortcut: "meta+t",
		NewTerminalShortcut:    "meta+t",
	})
	if err == nil {
		t.Fatal("NormalizeAppSettingsInput() error = nil, want duplicate shortcut validation error")
	}
}

func TestNormalizeAppSettingsInputRejectsInvalidDefaultHome(t *testing.T) {
	_, err := NormalizeAppSettingsInput(AppSettingsInput{DefaultHome: "remote-terminal"})
	if err == nil {
		t.Fatal("NormalizeAppSettingsInput() error = nil, want invalid default home error")
	}
}

func TestNormalizeAppSettingsInputNormalizesCommandPaletteShortcut(t *testing.T) {
	settings, err := NormalizeAppSettingsInput(AppSettingsInput{
		CommandPaletteShortcut: "shift + meta + p",
	})
	if err != nil {
		t.Fatalf("NormalizeAppSettingsInput() error = %v", err)
	}
	if settings.CommandPaletteShortcut != "meta+shift+p" {
		t.Fatalf("CommandPaletteShortcut = %q, want meta+shift+p", settings.CommandPaletteShortcut)
	}
}

func TestNormalizeAppSettingsInputRejectsShortcutWithoutCommandOrControl(t *testing.T) {
	_, err := NormalizeAppSettingsInput(AppSettingsInput{
		CommandPaletteShortcut: "shift+p",
	})
	if err == nil {
		t.Fatal("NormalizeAppSettingsInput() error = nil, want shortcut validation error")
	}
}

func TestAppServiceDirectoryCRUD(t *testing.T) {
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{Store: store})

	created, err := app.CreateDirectory(context.Background(), DirectoryInput{Name: "Production"})
	if err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateDirectory() ID is empty")
	}

	updated, err := app.UpdateDirectory(context.Background(), created.ID, DirectoryInput{Name: "Production API"})
	if err != nil {
		t.Fatalf("UpdateDirectory() error = %v", err)
	}
	if updated.Name != "Production API" {
		t.Fatalf("Updated directory Name = %q, want Production API", updated.Name)
	}

	directories, err := app.ListDirectories(context.Background())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if len(directories) != 1 {
		t.Fatalf("ListDirectories() len = %d, want 1", len(directories))
	}
	if directories[0].Name != "Production API" {
		t.Fatalf("ListDirectories()[0].Name = %q, want Production API", directories[0].Name)
	}

	if err := app.DeleteDirectory(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteDirectory() error = %v", err)
	}
	directories, err = app.ListDirectories(context.Background())
	if err != nil {
		t.Fatalf("ListDirectories() after delete error = %v", err)
	}
	if len(directories) != 0 {
		t.Fatalf("ListDirectories() len after delete = %d, want 0", len(directories))
	}
}

func TestAppServiceDeleteDirectoryRejectsDirectoryWithHosts(t *testing.T) {
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{Store: store})

	directory, err := app.CreateDirectory(context.Background(), DirectoryInput{Name: "Production"})
	if err != nil {
		t.Fatalf("CreateDirectory() error = %v", err)
	}
	if _, err = app.CreateHost(context.Background(), HostInput{
		DirectoryID: directory.ID,
		Alias:       "prod-api",
		HostName:    "10.0.0.1",
		User:        "deploy",
		Port:        22,
	}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if err = app.DeleteDirectory(context.Background(), directory.ID); !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("DeleteDirectory() error = %v, want ErrDirectoryNotEmpty", err)
	}

	directories, err := app.ListDirectories(context.Background())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if len(directories) != 1 {
		t.Fatalf("ListDirectories() len = %d, want 1", len(directories))
	}
	hosts, err := app.ListHosts(context.Background())
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 || hosts[0].DirectoryID != directory.ID {
		t.Fatalf("host directory after rejected delete = %+v, want directory %q", hosts, directory.ID)
	}
}

func TestAppServiceDeleteDirectoryRejectsDirectoryWithDescendantHosts(t *testing.T) {
	store := newMemoryHostStore()
	app := NewAppService(AppServiceOptions{Store: store})

	parent, err := app.CreateDirectory(context.Background(), DirectoryInput{Name: "Production"})
	if err != nil {
		t.Fatalf("CreateDirectory() parent error = %v", err)
	}
	child, err := app.CreateDirectory(context.Background(), DirectoryInput{Name: "API", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("CreateDirectory() child error = %v", err)
	}
	if _, err = app.CreateHost(context.Background(), HostInput{
		DirectoryID: child.ID,
		Alias:       "prod-api",
		HostName:    "10.0.0.1",
		User:        "deploy",
		Port:        22,
	}); err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}

	if err = app.DeleteDirectory(context.Background(), parent.ID); !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("DeleteDirectory() error = %v, want ErrDirectoryNotEmpty", err)
	}
}

type memoryHostStore struct {
	hosts             []Host
	directories       []Directory
	settings          AppSettings
	hasSettings       bool
	metadata          map[string]string
	exportedSnapshots map[string]memorySnapshot
	importedSnapshots []string
}

type memorySnapshot struct {
	hosts       []Host
	directories []Directory
}

type renderConcurrencyStore struct {
	*memoryHostStore
	mu        sync.Mutex
	active    int
	maxActive int
}

func (s *renderConcurrencyStore) ListHosts() ([]Host, error) {
	s.mu.Lock()
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.mu.Unlock()

	time.Sleep(20 * time.Millisecond)
	hosts, err := s.memoryHostStore.ListHosts()

	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return hosts, err
}

func newMemoryHostStore() *memoryHostStore {
	return &memoryHostStore{
		metadata:          make(map[string]string),
		exportedSnapshots: make(map[string]memorySnapshot),
	}
}

func (s *memoryHostStore) ListHosts() ([]Host, error) {
	return append([]Host(nil), s.hosts...), nil
}

func (s *memoryHostStore) CountHosts() (int, error) {
	return len(s.hosts), nil
}

func (s *memoryHostStore) CreateHost(input HostInput) (Host, error) {
	host := Host{
		ID:           input.Alias,
		DirectoryID:  input.DirectoryID,
		Alias:        input.Alias,
		HostName:     input.HostName,
		User:         input.User,
		Port:         input.Port,
		IdentityFile: input.IdentityFile,
		PasswordAuth: input.PasswordAuth,
		ProxyJump:    input.ProxyJump,
		JumpHostID:   input.JumpHostID,
	}
	s.hosts = append(s.hosts, host)
	return host, nil
}

func (s *memoryHostStore) UpdateHost(id string, input HostInput) (Host, error) {
	for i := range s.hosts {
		if s.hosts[i].ID == id {
			s.hosts[i].Alias = input.Alias
			s.hosts[i].DirectoryID = input.DirectoryID
			s.hosts[i].HostName = input.HostName
			s.hosts[i].User = input.User
			s.hosts[i].Port = input.Port
			s.hosts[i].IdentityFile = input.IdentityFile
			s.hosts[i].PasswordAuth = input.PasswordAuth
			s.hosts[i].ProxyJump = input.ProxyJump
			s.hosts[i].JumpHostID = input.JumpHostID
			return s.hosts[i], nil
		}
	}
	return Host{}, os.ErrNotExist
}

func (s *memoryHostStore) MoveHosts(ids []string, directoryID string) ([]Host, error) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	moved := make([]Host, 0, len(ids))
	for i := range s.hosts {
		if idSet[s.hosts[i].ID] {
			s.hosts[i].DirectoryID = directoryID
			moved = append(moved, s.hosts[i])
		}
	}
	return moved, nil
}

func (s *memoryHostStore) ReorderHosts(scopeType string, scopeID string, ids []string) ([]Host, error) {
	idSet := make(map[string]bool, len(ids))
	scopeKey := scopeType + ":" + scopeID
	for order, id := range ids {
		idSet[id] = true
		found := false
		for i := range s.hosts {
			if s.hosts[i].ID == id {
				if s.hosts[i].SortOrders == nil {
					s.hosts[i].SortOrders = make(map[string]int)
				}
				s.hosts[i].SortOrders[scopeKey] = order
				found = true
			}
		}
		if !found {
			return nil, ErrHostNotFound
		}
	}
	hosts := make([]Host, 0, len(ids))
	for _, host := range s.hosts {
		if idSet[host.ID] {
			hosts = append(hosts, host)
		}
	}
	return hosts, nil
}

func (s *memoryHostStore) MarkHostUsed(id string, usedAt string) (Host, error) {
	for i := range s.hosts {
		if s.hosts[i].ID == id {
			s.hosts[i].LastUsedAt = usedAt
			return s.hosts[i], nil
		}
	}
	return Host{}, os.ErrNotExist
}

func (s *memoryHostStore) DeleteHost(id string) error {
	for i := range s.hosts {
		if s.hosts[i].ID == id {
			s.hosts = append(s.hosts[:i], s.hosts[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *memoryHostStore) DeleteHosts(ids []string) error {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	next := s.hosts[:0]
	for _, host := range s.hosts {
		if !idSet[host.ID] {
			next = append(next, host)
		}
	}
	s.hosts = next
	return nil
}

func (s *memoryHostStore) ListDirectories() ([]Directory, error) {
	return append([]Directory(nil), s.directories...), nil
}

func (s *memoryHostStore) CreateDirectory(input DirectoryInput) (Directory, error) {
	input = NormalizeDirectoryInput(input)
	if err := ValidateDirectoryInput(input); err != nil {
		return Directory{}, err
	}
	directory := Directory{ID: input.Name, ParentID: input.ParentID, Name: input.Name}
	s.directories = append(s.directories, directory)
	return directory, nil
}

func (s *memoryHostStore) UpdateDirectory(id string, input DirectoryInput) (Directory, error) {
	input = NormalizeDirectoryInput(input)
	if err := ValidateDirectoryInput(input); err != nil {
		return Directory{}, err
	}
	for i := range s.directories {
		if s.directories[i].ID == id {
			s.directories[i].Name = input.Name
			return s.directories[i], nil
		}
	}
	return Directory{}, os.ErrNotExist
}

func (s *memoryHostStore) MoveDirectory(id string, input DirectoryMoveInput) (Directory, error) {
	for i := range s.directories {
		if s.directories[i].ID == id {
			s.directories[i].ParentID = input.ParentID
			s.directories[i].SortOrder = input.SortOrder
			return s.directories[i], nil
		}
	}
	return Directory{}, os.ErrNotExist
}

func (s *memoryHostStore) DeleteDirectory(id string) error {
	for i := range s.directories {
		if s.directories[i].ID == id {
			s.directories = append(s.directories[:i], s.directories[i+1:]...)
			break
		}
	}
	for i := range s.hosts {
		if s.hosts[i].DirectoryID == id {
			s.hosts[i].DirectoryID = ""
		}
	}
	return nil
}

func (s *memoryHostStore) GetSettings() (AppSettings, error) {
	if !s.hasSettings {
		return DefaultAppSettings(), nil
	}
	return s.settings, nil
}

func (s *memoryHostStore) UpdateSettings(input AppSettingsInput) (AppSettings, error) {
	settings, err := NormalizeAppSettingsInput(input)
	if err != nil {
		return AppSettings{}, err
	}
	s.settings = settings
	s.hasSettings = true
	return settings, nil
}

func (s *memoryHostStore) GetAppMetadata(key string) (string, error) {
	return s.metadata[key], nil
}

func (s *memoryHostStore) SetAppMetadata(key, value string) error {
	s.metadata[key] = value
	return nil
}

func (s *memoryHostStore) ExportHostConfigSnapshot(path string) (SyncSnapshotInfo, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return SyncSnapshotInfo{}, err
	}
	if err := os.WriteFile(path, []byte("snapshot"), 0o600); err != nil {
		return SyncSnapshotInfo{}, err
	}
	hosts := append([]Host(nil), s.hosts...)
	directories := append([]Directory(nil), s.directories...)
	s.exportedSnapshots[path] = memorySnapshot{hosts: hosts, directories: directories}
	return SyncSnapshotInfo{HostCount: len(hosts), DirectoryCount: len(directories)}, nil
}

func (s *memoryHostStore) ImportHostConfigSnapshot(path string) (SyncSnapshotInfo, error) {
	snapshot, ok := s.exportedSnapshots[path]
	if !ok {
		return SyncSnapshotInfo{}, os.ErrNotExist
	}
	s.hosts = append([]Host(nil), snapshot.hosts...)
	s.directories = append([]Directory(nil), snapshot.directories...)
	s.importedSnapshots = append(s.importedSnapshots, path)
	return SyncSnapshotInfo{HostCount: len(s.hosts), DirectoryCount: len(s.directories)}, nil
}
