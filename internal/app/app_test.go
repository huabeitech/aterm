package app

import (
	"os"
	"path/filepath"
	"ssh-connect/internal/service"
	"strings"
	"testing"
)

func TestDefaultAppPathsUseRealHomeForSSHDirectoryWhenSandboxed(t *testing.T) {
	containerHome := filepath.Join(string(filepath.Separator), "Users", "alice", "Library", "Containers", "pro.huabei.sshconfig", "Data")
	t.Setenv("HOME", containerHome)

	paths, err := defaultAppPaths()
	if err != nil {
		t.Fatalf("defaultAppPaths() error = %v", err)
	}

	expectedAppSupport := filepath.Join(containerHome, "Library", "Application Support", service.DataDirectoryName())
	if paths.appSupportDir != expectedAppSupport {
		t.Fatalf("appSupportDir = %q, want %q", paths.appSupportDir, expectedAppSupport)
	}

	expectedSSHDir := filepath.Join(string(filepath.Separator), "Users", "alice", ".ssh")
	if paths.sshDir != expectedSSHDir {
		t.Fatalf("sshDir = %q, want %q", paths.sshDir, expectedSSHDir)
	}
	if paths.sshConfigPath != filepath.Join(expectedSSHDir, "config") {
		t.Fatalf("sshConfigPath = %q, want config under %q", paths.sshConfigPath, expectedSSHDir)
	}

	expectedSyncRoot := filepath.Join(string(filepath.Separator), "Users", "alice", "Library", "Mobile Documents", "com~apple~CloudDocs")
	if paths.syncRootPath != expectedSyncRoot {
		t.Fatalf("syncRootPath = %q, want %q", paths.syncRootPath, expectedSyncRoot)
	}
	expectedSyncSnapshot := filepath.Join(expectedSyncRoot, "aTerm", "sync", "app.db")
	if paths.syncSnapshotPath != expectedSyncSnapshot {
		t.Fatalf("syncSnapshotPath = %q, want %q", paths.syncSnapshotPath, expectedSyncSnapshot)
	}
}

func TestSSHAuthorizationDialogDoesNotCreateMissingDirectory(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")

	if got := existingSSHDirectoryOrHome(home, sshDir); got != home {
		t.Fatalf("default directory = %q, want home %q", got, home)
	}
	if _, err := os.Stat(sshDir); !os.IsNotExist(err) {
		t.Fatalf("SSH directory stat after lookup = %v, want directory to remain absent", err)
	}

	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatalf("create SSH directory: %v", err)
	}
	if got := existingSSHDirectoryOrHome(home, sshDir); got != sshDir {
		t.Fatalf("default directory = %q, want existing SSH directory %q", got, sshDir)
	}
}

func TestMASEntitlementsAvoidAppleEvents(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "build", "darwin", "entitlements.mas.plist"))
	if err != nil {
		t.Fatalf("ReadFile() entitlements error = %v", err)
	}
	text := string(data)
	for _, rejected := range []string{
		"<key>com.apple.security.automation.apple-events</key>",
		"<key>com.apple.security.temporary-exception.apple-events</key>",
		"<string>com.apple.terminal</string>",
		"<string>com.googlecode.iterm2</string>",
	} {
		if strings.Contains(text, rejected) {
			t.Fatalf("entitlements.mas.plist should not include %q", rejected)
		}
	}
}

func TestAppStoreEntitlementsAvoidAppleEvents(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "build", "darwin", "entitlements.appstore.plist"))
	if err != nil {
		t.Fatalf("ReadFile() entitlements error = %v", err)
	}
	text := string(data)
	for _, rejected := range []string{
		"<key>com.apple.security.automation.apple-events</key>",
		"<key>com.apple.security.temporary-exception.apple-events</key>",
		"<string>com.apple.terminal</string>",
		"<string>com.googlecode.iterm2</string>",
		"<string>com.apple.systemevents</string>",
	} {
		if strings.Contains(text, rejected) {
			t.Fatalf("entitlements.appstore.plist should not include %q", rejected)
		}
	}
}
