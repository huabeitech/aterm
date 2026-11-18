package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixturePackage(t *testing.T) Package {
	t.Helper()
	m, err := ParseManifest([]byte(`{"schemaVersion":1,"id":"example.monitor","name":"Example","version":"1.0.0","runtime":"sandbox","entry":"plugin.js","permissions":[],"contributes":{"terminalStatusBar":{"id":"example.monitor.status"},"views":[{"id":"details","title":"Details","entry":"ui/details.html","placement":"terminal.sidePanel"}],"commands":[{"id":"show","title":"Show","action":{"type":"view.open","view":"details","context":{"page":"one"}}}]},"i18n":{"defaultLocale":"en","locales":{"en":"locales/en.json"}},"engines":{"hostApi":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	return Package{Manifest: m, Source: "void 0", Views: map[string]string{"ui/details.html": "<div>Hello</div>"}, Localizations: &Localizations{DefaultLocale: "en", Messages: map[string]map[string]string{"en": {"name": "Example"}}}}
}
func writePackage(t *testing.T, root string, plugin Package) {
	t.Helper()
	files, err := packageFiles(plugin)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func zipFiles(t *testing.T, files map[string][]byte, extras ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(data)
	}
	for _, name := range extras {
		f, _ := writer.Create(name)
		f.Write([]byte("duplicate"))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func TestImmutablePackageRoundtripAndIntegrity(t *testing.T) {
	incoming := t.TempDir()
	plugin := fixturePackage(t)
	writePackage(t, incoming, plugin)
	inspected, err := InspectDirectory(incoming)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPackageStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.Install(inspected)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(incoming, "plugin.js"), []byte("changed source"), 0600)
	loaded, err := store.Load(key)
	if err != nil || loaded.Source != "void 0" {
		t.Fatal(loaded, err)
	}
	if _, err = store.Load("../escape"); err == nil {
		t.Fatal("accepted traversal")
	}
	os.WriteFile(filepath.Join(store.root, key, "plugin.js"), []byte("tampered"), 0600)
	if _, err = store.Load(key); err == nil {
		t.Fatal("accepted changed package")
	}
	if _, err = store.Install(inspected); err == nil {
		t.Fatal("ignored corrupted existing package")
	}
}
func TestDirectoryRejectsLinksAndBounds(t *testing.T) {
	root := t.TempDir()
	plugin := fixturePackage(t)
	writePackage(t, root, plugin)
	original := filepath.Join(t.TempDir(), "source")
	os.WriteFile(original, []byte("void 0"), 0600)
	os.Remove(filepath.Join(root, "plugin.js"))
	os.Symlink(original, filepath.Join(root, "plugin.js"))
	if _, err := InspectDirectory(root); err == nil {
		t.Fatal("followed source symlink")
	}
	os.Remove(filepath.Join(root, "plugin.js"))
	os.WriteFile(filepath.Join(root, "plugin.js"), []byte(strings.Repeat("a", 256*1024+1)), 0600)
	if _, err := InspectDirectory(root); err == nil {
		t.Fatal("accepted oversized source")
	}
	os.WriteFile(filepath.Join(root, "plugin.js"), []byte("void 0"), 0600)
	os.Rename(filepath.Join(root, "ui"), filepath.Join(root, "saved-ui"))
	os.Symlink(filepath.Join(root, "saved-ui"), filepath.Join(root, "ui"))
	if _, err := InspectDirectory(root); err == nil {
		t.Fatal("followed UI directory symlink")
	}
}
func TestArchiveValidation(t *testing.T) {
	plugin := fixturePackage(t)
	files, _ := packageFiles(plugin)
	archive := zipFiles(t, files)
	inspected, err := InspectArchiveBytes(archive)
	if err != nil || inspected.Manifest.ID != plugin.Manifest.ID {
		t.Fatal(inspected, err)
	}
	for _, name := range []string{"../escape", "src/source.ts", "ui/undeclared.html", "locales/xx.json"} {
		invalid := map[string][]byte{}
		for k, v := range files {
			invalid[k] = v
		}
		invalid[name] = []byte("bad")
		if _, err = InspectArchiveBytes(zipFiles(t, invalid)); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	if _, err = InspectArchiveBytes(zipFiles(t, files, "plugin.js")); err == nil {
		t.Fatal("accepted duplicate")
	}
	if _, err = InspectArchiveBytes(zipFiles(t, map[string][]byte{"manifest.json": files["manifest.json"]})); err == nil {
		t.Fatal("accepted missing files")
	}
	if _, err = InspectArchiveBytes(make([]byte, ArchiveLimit+1)); err == nil {
		t.Fatal("accepted oversized archive")
	}
	for _, mutate := range []func([]byte, int){
		func(b []byte, at int) { b[at+8] |= 1 },
		func(b []byte, at int) { b[at+41] = 0xa0 },
		func(b []byte, at int) {
			for i := 0; i < 4; i++ {
				b[at+16+i] = 0
			}
		},
	} {
		data := append([]byte(nil), archive...)
		at := bytes.Index(data, []byte{0x50, 0x4b, 0x01, 0x02})
		mutate(data, at)
		if _, err = InspectArchiveBytes(data); err == nil {
			t.Fatal("accepted unsafe ZIP metadata or checksum")
		}
	}
}
func TestManifestRejectsUndeclaredCapabilitiesAndMalformedContributions(t *testing.T) {
	original, _ := marshalJSON(fixturePackage(t).Manifest)
	cases := []func(map[string]any){
		func(m map[string]any) { m["entry"] = "../plugin.js" }, func(m map[string]any) { m["permissions"] = []string{"exec"} }, func(m map[string]any) { m["runtime"] = "node" }, func(m map[string]any) { m["schemaVersion"] = 2 },
		func(m map[string]any) { m["arbitrary"] = true }, func(m map[string]any) { m["engines"] = nil }, func(m map[string]any) { m["permissions"] = nil }, func(m map[string]any) {
			m["execution"] = map[string]any{"programs": []string{"sh"}, "targets": []string{"ssh"}}
		},
		func(m map[string]any) {
			m["contributes"].(map[string]any)["terminalStatusBar"] = map[string]string{"id": "aterm.status"}
		},
		func(m map[string]any) {
			m["contributes"].(map[string]any)["views"].([]any)[0].(map[string]any)["entry"] = "../details.html"
		},
		func(m map[string]any) {
			m["contributes"].(map[string]any)["commands"].([]any)[0].(map[string]any)["action"].(map[string]any)["view"] = "unknown"
		},
		func(m map[string]any) {
			m["i18n"] = map[string]any{"defaultLocale": "en-us", "locales": map[string]string{"en-us": "locales/en-us.json"}}
		},
	}
	for i, modify := range cases {
		var m map[string]any
		json.Unmarshal(original, &m)
		modify(m)
		data, _ := json.Marshal(m)
		if _, err := ParseManifest(data); err == nil {
			t.Fatalf("accepted case %d", i)
		}
	}
	if _, err := JSONSnapshot([]byte(`{"constructor":"bad"}`), 4096); err == nil {
		t.Fatal("accepted prototype data key")
	}
	deep := strings.Repeat("[", 18) + "null" + strings.Repeat("]", 18)
	if _, err := JSONSnapshot([]byte(deep), 4096); err == nil {
		t.Fatal("accepted excessive depth")
	}
}

func TestViewIconAcceptsGeometryOnly(t *testing.T) {
	p := fixturePackage(t)
	for _, icon := range []string{"M8,8H248V248H8Z", "<svg onload=\"alert(1)\"/>", "javascript:alert(1)"} {
		p.Manifest.Contributes.Views[0].Icon = icon
		raw, _ := json.Marshal(p.Manifest)
		m, err := ParseManifest(raw)
		if strings.HasPrefix(icon, "M") {
			if err != nil || m.Contributes.Views[0].Icon != icon {
				t.Fatalf("valid geometry rejected: %v", err)
			}
		} else if err == nil {
			t.Fatalf("accepted non-geometry icon %q", icon)
		}
	}
}
