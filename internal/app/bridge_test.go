package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"ssh-connect/internal/db"
	"ssh-connect/internal/desktop"
	desktopkeys "ssh-connect/internal/desktop/keys"
	desktopplugins "ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/domain"
	"ssh-connect/internal/service"
	"ssh-connect/internal/terminal"
	"ssh-connect/internal/tunnel"
	"testing"
	"time"
)

// Bridge tests must never read or modify the user's real Keychain.
type bridgePasswords map[string]string

func (s bridgePasswords) Get(id string) (string, error) {
	value, ok := s[id]
	if !ok {
		return "", service.ErrPasswordNotFound
	}
	return value, nil
}
func (s bridgePasswords) Set(id, value string) error { s[id] = value; return nil }
func (s bridgePasswords) Delete(id string) error     { delete(s, id); return nil }

func bridgeFixture(t *testing.T) *App {
	t.Helper()
	directory := t.TempDir()
	store, err := db.Open(filepath.Join(directory, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	app := &App{ctx: context.Background(), store: store}
	app.svc = service.NewAppService(service.AppServiceOptions{Store: store, PasswordStore: bridgePasswords{}, DatabasePath: filepath.Join(directory, "app.db"), SSHDir: filepath.Join(directory, "ssh"), LocalVaultPath: filepath.Join(directory, "vault.enc")})
	app.terminal = terminal.NewManager(app.ctx, nil)
	return app
}
func bridgeArguments(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	result := make([]json.RawMessage, len(values))
	for i, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		result[i] = data
	}
	return result
}
func TestDesktopBootstrapAndPreferencesPersistWithoutChangingLibrary(t *testing.T) {
	app := bridgeFixture(t)
	raw, err := app.DesktopRequest(context.Background(), "app:bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	boot := raw.(map[string]any)
	preferences := boot["preferences"].(desktop.Preferences)
	if preferences["commandPaletteShortcut"] != desktop.DefaultPreferences()["commandPaletteShortcut"] || preferences["language"] != "en" {
		t.Fatalf("unexpected defaults: %#v", preferences)
	}
	preferences["terminalFontSize"] = float64(18)
	preferences["appFontScale"] = float64(120)
	preferences["toggleSidebarShortcut"] = "meta+alt+b"
	if _, err := app.DesktopRequest(context.Background(), "preferences:save", bridgeArguments(t, preferences)); err != nil {
		t.Fatal(err)
	}
	saved, err := app.desktopPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if saved["terminalFontSize"] != float64(18) || saved["appFontScale"] != float64(120) {
		t.Fatal("preferences were not persisted")
	}
	preferences["newTerminalShortcut"] = preferences["closeTerminalShortcut"]
	if _, err := app.DesktopRequest(context.Background(), "preferences:save", bridgeArguments(t, preferences)); err == nil {
		t.Fatal("duplicate shortcuts accepted")
	}
	saved, _ = app.desktopPreferences()
	if saved["newTerminalShortcut"] != desktop.DefaultPreferences()["newTerminalShortcut"] {
		t.Fatal("invalid save changed persisted preferences")
	}
}
func TestDesktopLibraryRoutesTypedPayloadAndReportsInvalidArguments(t *testing.T) {
	app := bridgeFixture(t)
	raw, err := app.DesktopRequest(context.Background(), "library:request", bridgeArguments(t, "createDirectory", service.DirectoryInput{Name: "Servers"}))
	if err != nil {
		t.Fatal(err)
	}
	directory := raw.(service.Directory)
	raw, err = app.DesktopRequest(context.Background(), "library:request", bridgeArguments(t, "createHost", service.HostInput{DirectoryID: directory.ID, Alias: "test-host", HostName: "127.0.0.1", User: "test", Port: 22}))
	if err != nil {
		t.Fatal(err)
	}
	host := raw.(service.Host)
	raw, err = app.DesktopRequest(context.Background(), "library:request", bridgeArguments(t, "listHosts"))
	if err != nil {
		t.Fatal(err)
	}
	hosts := raw.([]service.Host)
	if len(hosts) != 1 || hosts[0].ID != host.ID {
		t.Fatal("host library contract mismatch")
	}
	for _, args := range [][]json.RawMessage{nil, bridgeArguments(t, "updateHost", 12), bridgeArguments(t, "deleteHosts", "wrong type")} {
		if _, err := app.DesktopRequest(context.Background(), "library:request", args); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
	if _, err := app.DesktopRequest(context.Background(), "arbitrary:method", nil); err == nil {
		t.Fatal("unknown channel accepted")
	}
}

func TestDesktopProfileIsolatesEveryUserDataPath(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("ATERM_DATA_DIR", profile)
	paths, err := defaultAppPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{paths.databasePath, paths.localVaultPath, paths.backupDir, paths.syncRootPath, paths.syncVaultPath, paths.sshDir, paths.sshConfigPath} {
		relative, err := filepath.Rel(profile, name)
		if err != nil || relative == ".." || len(relative) > 3 && relative[:3] == "../" {
			t.Fatalf("profile leaked into external path %q", name)
		}
	}
	t.Setenv("ATERM_DATA_DIR", "relative")
	if _, err := defaultAppPaths(); err == nil {
		t.Fatal("relative profile accepted")
	}
}

func TestFileEditorRequiresItsNativeOwningWindow(t *testing.T) {
	app := bridgeFixture(t)
	for _, operation := range []string{"state", "change", "save", "close", "quit"} {
		if _, err := app.DesktopRequest(context.Background(), "file-editor:request", bridgeArguments(t, operation, "text")); err == nil {
			t.Fatalf("%s accepted a request without an owning editor window", operation)
		}
	}
}

func TestDesktopSSHFailureIsRetainedAsAnErrorPane(t *testing.T) {
	app := bridgeFixture(t)
	exited := make(chan struct{}, 1)
	app.terminal = terminal.NewManager(context.Background(), func(_ string, payload any) {
		if event, ok := payload.(terminal.OutputEvent); ok && event.State == "exited" {
			exited <- struct{}{}
		}
	})
	opened, err := app.terminal.OpenSSHAsync("fixture", func(context.Context, service.HostKeyConfirmer) (*service.NativeSSHClient, string, error) {
		return nil, "", errors.New("fixture authentication failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.terminal.CloseAll()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("SSH failure was not published")
	}
	session, err := app.desktopTerminalSession(opened.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session["state"] != "error" || session["error"] == "" {
		t.Fatal("failed SSH authentication would disappear as a normal exited tab")
	}
	if _, ok := session["localNumber"]; ok {
		t.Fatal("SSH session contains a local terminal number")
	}
}

func TestManagedKeyCannotBeDeletedWhileReferencedByAHost(t *testing.T) {
	app := bridgeFixture(t)
	if _, err := app.svc.SetUpSecureData(context.Background(), "fixture-password"); err != nil {
		t.Fatal(err)
	}
	keys := desktopkeys.NewStore(app.svc.SyncService)
	defer keys.Close()
	app.desktopKeys = keys
	key, err := keys.Create(desktopkeys.CreateInput{Name: "Referenced", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := app.CreateHost(service.HostInput{Alias: "referenced", HostName: "server.test", Port: 22, IdentityFile: desktopkeys.Prefix + key.ID})
	if err != nil {
		t.Fatal(err)
	}
	args := bridgeArguments(t, "deleteManagedKey", key.ID)
	if _, err := app.DesktopRequest(context.Background(), "library:request", args); err == nil {
		t.Fatal("referenced managed key was deleted")
	}
	if len(keys.List()) != 1 {
		t.Fatal("rejected deletion changed the key library")
	}
	if err := app.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DesktopRequest(context.Background(), "library:request", args); err != nil {
		t.Fatal(err)
	}
	if len(keys.List()) != 0 {
		t.Fatal("unreferenced key was not deleted")
	}
}

func TestPluginRuntimeRequiresOwningWindow(t *testing.T) {
	app := bridgeFixture(t)
	registry, err := desktopplugins.NewRegistry(desktopplugins.RegistryOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	app.desktopPluginRegistry = registry
	if _, err := app.DesktopRequest(context.Background(), "plugins:manage", bridgeArguments(t, "list")); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"plugins:runtime:rpc", "plugins:runtime:reset", "plugins:manage"} {
		if _, err := app.DesktopRequest(context.Background(), channel, bridgeArguments(t, "configure", "example.monitor", true, 5)); err == nil {
			t.Fatalf("%s accepted an ownerless request", channel)
		}
	}
}

func TestDesktopBootstrapMigratesMissingBindingsFromStoredPreferences(t *testing.T) {
	app := bridgeFixture(t)
	preferences := desktop.DefaultPreferencesForPlatform("win32")
	preferences["newTerminalShortcut"] = "ctrl+shift+b"
	preferences["closeTerminalShortcut"] = "ctrl+shift+e"
	delete(preferences, "toggleSidebarShortcut")
	delete(preferences, "toggleTerminalFilesShortcut")
	delete(preferences, "commandSearchShortcut")
	encoded, err := json.Marshal(preferences)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.SetAppMetadata(desktopPreferencesKey, string(encoded)); err != nil {
		t.Fatal(err)
	}
	raw, err := app.DesktopRequest(context.Background(), "app:bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(map[string]any)["preferences"].(desktop.Preferences)
	for field, want := range map[string]string{
		"newTerminalShortcut": "ctrl+shift+b", "closeTerminalShortcut": "ctrl+shift+e",
		"toggleSidebarShortcut": "ctrl+alt+b", "toggleTerminalFilesShortcut": "ctrl+alt+e",
		"commandSearchShortcut": "ctrl+shift+p",
	} {
		if got[field] != want {
			t.Errorf("%s: got %v, want %s", field, got[field], want)
		}
	}
	stored, err := app.store.GetAppMetadata(desktopPreferencesKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored != string(encoded) {
		t.Fatal("bootstrap rewrote the stored preferences")
	}
	if _, err := app.DesktopRequest(context.Background(), "preferences:save", bridgeArguments(t, got)); err != nil {
		t.Fatal(err)
	}
	reloaded, err := app.desktopPreferences()
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range got {
		if reloaded[field] != want {
			t.Errorf("%s changed after save/reload", field)
		}
	}
}

func TestDesktopPreferenceSaveRejectsRendererInvalidValuesWithoutPersisting(t *testing.T) {
	app := bridgeFixture(t)
	valid := desktop.DefaultPreferences()
	if _, err := app.DesktopRequest(context.Background(), "preferences:save", bridgeArguments(t, valid)); err != nil {
		t.Fatal(err)
	}
	before, err := app.store.GetAppMetadata(desktopPreferencesKey)
	if err != nil {
		t.Fatal(err)
	}
	for field, value := range map[string]string{"theme": "unknown-theme", "lightTheme": "system", "darkTheme": "example/../dark", "newTerminalShortcut": "meta+q"} {
		invalid := desktop.DefaultPreferences()
		invalid[field] = value
		if _, err := app.DesktopRequest(context.Background(), "preferences:save", bridgeArguments(t, invalid)); err == nil {
			t.Errorf("%s=%s accepted", field, value)
		}
		after, err := app.store.GetAppMetadata(desktopPreferencesKey)
		if err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Errorf("rejected %s changed persisted settings", field)
		}
	}
}

func TestDesktopLanguagePersistsAndBootstraps(t *testing.T) {
	app := bridgeFixture(t)
	for index, language := range []string{"zh-CN", "ja", "system", "en"} {
		preferences := desktop.DefaultPreferences()
		preferences["language"] = language
		raw, err := app.desktopSavePreferences(bridgeArguments(t, preferences))
		if err != nil {
			t.Fatal(err)
		}
		if raw.(desktop.Preferences)["language"] != language {
			t.Fatalf("save did not retain language %s", language)
		}
		raw, err = app.desktopBootstrap()
		if err != nil {
			t.Fatal(err)
		}
		boot := raw.(map[string]any)
		locale := boot["locale"].(map[string]any)
		if boot["preferences"].(desktop.Preferences)["language"] != language || locale["preference"] != language || locale["revision"] != uint64(index+1) {
			t.Fatalf("unexpected locale snapshot: %#v", boot)
		}
	}
	preferences := desktop.DefaultPreferences()
	preferences["language"] = "invalid"
	if _, err := app.desktopSavePreferences(bridgeArguments(t, preferences)); err == nil {
		t.Fatal("unsupported language was accepted")
	}
	saved, err := app.desktopPreferences()
	if err != nil || saved["language"] != "en" {
		t.Fatalf("invalid save changed preferences: %v %v", saved, err)
	}
}

func TestDesktopTunnelRoutesValidatePersistAndProtectHosts(t *testing.T) {
	app := bridgeFixture(t)
	defer app.svc.CloseTunnels()
	host, err := app.store.CreateHost(service.HostInput{Alias: "tunnel-host", HostName: "127.0.0.1", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	input := domain.TunnelInput{Name: "Database", HostID: host.ID, LocalAddress: "127.0.0.1", LocalPort: 13306, TargetAddress: "db.internal", TargetPort: 3306}
	raw, err := app.DesktopRequest(app.ctx, "tunnels:request", bridgeArguments(t, "save", "", input))
	if err != nil {
		t.Fatal(err)
	}
	config := raw.(domain.Tunnel)
	raw, err = app.DesktopRequest(app.ctx, "tunnels:request", bridgeArguments(t, "list"))
	if err != nil {
		t.Fatal(err)
	}
	statuses := raw.([]tunnel.Status)
	if len(statuses) != 1 || statuses[0].ID != config.ID || statuses[0].State != "stopped" {
		t.Fatalf("statuses = %+v", statuses)
	}
	if _, err := app.DesktopRequest(app.ctx, "library:request", bridgeArguments(t, "deleteHosts", []string{host.ID})); err == nil {
		t.Fatal("deleted host referenced by tunnel")
	}
	if _, err := app.DesktopRequest(app.ctx, "tunnels:request", bridgeArguments(t, "save", config.ID)); err == nil {
		t.Fatal("accepted missing input")
	}
	if _, err := app.DesktopRequest(app.ctx, "tunnels:request", bridgeArguments(t, "unexpected", config.ID)); err == nil {
		t.Fatal("accepted unknown operation")
	}
	if _, err := app.DesktopRequest(app.ctx, "tunnels:request", bridgeArguments(t, "delete", config.ID)); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteHosts([]string{host.ID}); err != nil {
		t.Fatal(err)
	}
}
