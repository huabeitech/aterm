package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryUpdatesRollbackPersistenceAndGrants(t *testing.T) {
	root := t.TempDir()
	r, err := NewRegistry(RegistryOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	plugin := fixturePackage(t)
	if err = r.Install(plugin, r.Revision()); err != nil {
		t.Fatal(err)
	}
	manifest, config, generation, err := r.Package(plugin.Manifest.ID)
	if err != nil || !config.Enabled {
		t.Fatal(manifest, config, err)
	}
	grant := Grant{ID: strings.Repeat("a", 64), Label: "Fixture host"}
	if err = r.Grant(plugin.Manifest.ID, grant, generation); err != nil {
		t.Fatal(err)
	}
	stale := r.Revision()
	if err = r.Configure(plugin.Manifest.ID, true, 10); err != nil {
		t.Fatal(err)
	}
	plugin.Manifest.Version = "1.1.0"
	if err = r.Install(plugin, stale); err == nil {
		t.Fatal("accepted stale installation approval")
	}
	if err = r.Install(plugin, r.Revision()); err != nil {
		t.Fatal(err)
	}
	list, _ := r.List()
	if len(list) != 1 || !list[0].CanRollback || len(list[0].Grants) != 1 || list[0].Interval != 10 {
		t.Fatal(list)
	}
	if err = r.Rollback(plugin.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = r.List()
	if list[0].Manifest.Version != "1.0.0" || !list[0].CanRollback {
		t.Fatal(list)
	}
	// A changed policy invalidates the old host permission even at the same target.
	plugin.Manifest.Version = "2.0.0"
	plugin.Manifest.Permissions = []string{"process.execute"}
	plugin.Manifest.Execution = &ExecutionPolicy{Programs: []string{"sh"}, Targets: []string{"ssh"}}
	if err = r.Install(plugin, r.Revision()); err != nil {
		t.Fatal(err)
	}
	list, _ = r.List()
	if len(list[0].Grants) != 0 {
		t.Fatal("grant survived changed capabilities")
	}
	reopened, err := NewRegistry(RegistryOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	list, _ = reopened.List()
	if len(list) != 1 || list[0].Manifest.Version != "2.0.0" {
		t.Fatal(list)
	}
	list[0].Manifest.Name = "external mutation"
	again, _ := reopened.List()
	if again[0].Manifest.Name == "external mutation" {
		t.Fatal("catalog exposes mutable internal state")
	}
	if err = reopened.Uninstall(plugin.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = reopened.List()
	if len(list) != 0 {
		t.Fatal(list)
	}
}
func TestRegistrySafeModeCorruptionAndFailedWrite(t *testing.T) {
	root := t.TempDir()
	r, _ := NewRegistry(RegistryOptions{Root: root})
	plugin := fixturePackage(t)
	if err := r.Install(plugin, r.Revision()); err != nil {
		t.Fatal(err)
	}
	safe, _ := NewRegistry(RegistryOptions{Root: root, SafeMode: true})
	list, _ := safe.List()
	if len(list) != 1 || list[0].Enabled {
		t.Fatal(list)
	}
	if err := safe.Install(plugin, safe.Revision()); err == nil {
		t.Fatal("safe mode accepted installation")
	}
	// Registry persistence errors must not alter the active in-memory config.
	originalPath := r.path
	r.path = filepath.Join(t.TempDir(), "directory")
	os.Mkdir(r.path, 0700)
	if err := r.Configure(plugin.Manifest.ID, false, 30); err == nil {
		t.Fatal("accepted failed write")
	}
	list, _ = r.List()
	if !list[0].Enabled || list[0].Interval != 5 {
		t.Fatal("failed write changed config")
	}
	r.path = originalPath
	os.WriteFile(originalPath, []byte("{"), 0600)
	broken, _ := NewRegistry(RegistryOptions{Root: root})
	if _, err := broken.List(); err == nil {
		t.Fatal("silently replaced unreadable registry")
	}
	if err := broken.Configure(plugin.Manifest.ID, false, 5); err == nil {
		t.Fatal("mutated corrupted registry")
	}
}
func TestDamagedInstalledPackageRemainsDiagnosable(t *testing.T) {
	root := t.TempDir()
	r, _ := NewRegistry(RegistryOptions{Root: root})
	plugin := fixturePackage(t)
	if err := r.Install(plugin, r.Revision()); err != nil {
		t.Fatal(err)
	}
	_, config, _, _ := r.Package(plugin.Manifest.ID)
	os.WriteFile(filepath.Join(root, "packages", config.Key, "plugin.js"), []byte("tampered"), 0600)
	reopened, _ := NewRegistry(RegistryOptions{Root: root})
	list, err := reopened.List()
	if err != nil || len(list) != 1 || list[0].State != "failed" || list[0].Error == "" {
		t.Fatal(list, err)
	}
	if err = reopened.Uninstall(plugin.Manifest.ID); err != nil {
		t.Fatal(err)
	}
}
func TestStorageScopeQuotaAndSymlinkRejection(t *testing.T) {
	root := t.TempDir()
	storage := NewStorage(root)
	if err := storage.Set("example.one", "key", []byte(`{"value":"one"}`)); err != nil {
		t.Fatal(err)
	}
	data, err := storage.Get("example.two", "key")
	if err != nil || string(data) != "null" {
		t.Fatal(string(data), err)
	}
	if err = storage.Set("example.one", "../escape", []byte("null")); err == nil {
		t.Fatal("accepted invalid storage key")
	}
	if err = storage.Set("example.one", "bad", []byte(`{"__proto__":1}`)); err == nil {
		t.Fatal("accepted unsafe data")
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("1"), 0600)
	os.Symlink(target, filepath.Join(root, "example.one", "link.json"))
	if _, err = storage.Get("example.one", "link"); err == nil {
		t.Fatal("followed storage symlink")
	}
	os.Remove(filepath.Join(root, "example.one", "link.json"))
	value := []byte(`"` + strings.Repeat("a", 29000) + `"`)
	for i := 0; i < 10; i++ {
		err = storage.Set("example.one", string(rune('a'+i)), value)
	}
	if err == nil {
		t.Fatal("exceeded storage quota")
	}
}
