package themes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPackage() Package { p := Builtin(); p.ID = "example.test"; return p }
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestBuiltinAndTokenMirror(t *testing.T) {
	// Changes in the copied Electron contract must explicitly update the backend.
	source, err := os.ReadFile("../../../frontend/src/shared/contracts/builtin-themes.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(source) != string(builtinData) {
		t.Fatal("built-in theme copy drifted from renderer")
	}
	source, err = os.ReadFile("../../../frontend/src/shared/contracts/themePackages.ts")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "export const colorTokens = {")
	end := strings.Index(string(source)[start:], "} as const") + start
	matches := regexpTokenNames(string(source)[start:end])
	if len(matches) != len(tokens) {
		t.Fatal("token count drift")
	}
	for _, name := range matches {
		if !tokens[name] {
			t.Fatalf("missing token %s", name)
		}
	}
	if len(Builtin().Themes) < 2 {
		t.Fatal("missing bundled themes")
	}
}
func regexpTokenNames(source string) []string {
	var result []string
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "'") {
			parts := strings.Split(line, "'")
			result = append(result, parts[1])
		}
	}
	return result
}
func TestCatalogRoundtripDuplicateAndRemoveIndividualTheme(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "packages"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := testPackage()
	catalog, err := s.InstallPackage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Packages) != 1 {
		t.Fatal(catalog)
	}
	export := filepath.Join(dir, "export.json")
	if err = s.Export(pkg.ID, export); err != nil {
		t.Fatal(err)
	}
	parsed, err := Inspect(export)
	if err != nil || parsed.ID != pkg.ID {
		t.Fatal(parsed, err)
	}
	catalog, err = s.Duplicate("dark")
	if err != nil || len(catalog.Packages) != 2 {
		t.Fatal(catalog, err)
	}
	var duplicate Package
	for _, p := range catalog.Packages {
		if p.ID != pkg.ID {
			duplicate = p
		}
	}
	if duplicate.Themes[0].ID != "default" || duplicate.Name != "Dark copy" {
		t.Fatal(duplicate)
	}
	catalog, err = s.Remove(pkg.ID + "/" + pkg.Themes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range catalog.Packages {
		if p.ID == pkg.ID && len(p.Themes) != len(pkg.Themes)-1 {
			t.Fatal("did not remove only requested theme")
		}
	}
	if _, err = s.Remove("dark"); err == nil {
		t.Fatal("deleted builtin")
	}
	if _, err = s.Remove("../outside"); err == nil {
		t.Fatal("accepted invalid reference")
	}
	if _, err = s.Remove(duplicate.ID); err != nil {
		t.Fatal(err)
	}
	// One malformed package must not hide the remaining valid catalog.
	os.WriteFile(filepath.Join(s.directory, "broken.json"), []byte("{"), 0600)
	catalog, err = s.List()
	if err != nil || len(catalog.Warnings) != 1 || len(catalog.Packages) != 1 {
		t.Fatal(catalog, err)
	}
	reopened, err := NewStore(s.directory)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err = reopened.List()
	if err != nil || catalog.Packages[0].ID != pkg.ID {
		t.Fatal(catalog, err)
	}
}
func TestManifestAndUnsafePaths(t *testing.T) {
	root := t.TempDir()
	definition := Builtin().Themes[0]
	writeJSON(t, filepath.Join(root, "dark.json"), definition)
	pkg := testPackage()
	data, _ := json.Marshal(pkg)
	var manifest map[string]any
	json.Unmarshal(data, &manifest)
	entry := map[string]any{"id": "custom", "name": "Custom", "appearance": "dark", "path": "dark.json"}
	manifest["themes"] = []any{entry}
	path := filepath.Join(root, "manifest.json")
	writeJSON(t, path, manifest)
	p, err := Inspect(path)
	if err != nil || p.Themes[0].ID != "custom" {
		t.Fatal(p, err)
	}
	for _, unsafe := range []string{"../outside.json", filepath.Join(root, "dark.json"), "linked.json", "sub/file.json"} {
		if unsafe == "linked.json" {
			os.Symlink(filepath.Join(root, "dark.json"), filepath.Join(root, unsafe))
		}
		if unsafe == "sub/file.json" {
			os.Symlink(root, filepath.Join(root, "sub"))
		}
		entry["path"] = unsafe
		writeJSON(t, path, manifest)
		if _, err := Inspect(path); err == nil {
			t.Fatalf("accepted %s", unsafe)
		}
	}
}
func TestValidationDoesNotReplaceInstalledPackage(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	pkg := testPackage()
	if _, err := s.InstallPackage(pkg); err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(pkg)
	cases := []func(map[string]any){
		func(m map[string]any) { m["id"] = "aterm.reserved" }, func(m map[string]any) { m["schemaVersion"] = 2 },
		func(m map[string]any) { m["license"] = nil }, func(m map[string]any) { m["themes"] = []any{} },
		func(m map[string]any) {
			m["themes"].([]any)[0].(map[string]any)["colors"] = map[string]any{"bad.token": "#ffffff"}
		},
		func(m map[string]any) {
			m["themes"].([]any)[0].(map[string]any)["terminal"].(map[string]any)["background"] = "#ffffffaa"
		},
		func(m map[string]any) {
			m["themes"].([]any)[0].(map[string]any)["terminal"].(map[string]any)["ansi"] = []string{"#ffffff"}
		},
		func(m map[string]any) {
			m["themes"].([]any)[1].(map[string]any)["id"] = m["themes"].([]any)[0].(map[string]any)["id"]
		},
	}
	for _, modify := range cases {
		var m map[string]any
		json.Unmarshal(original, &m)
		modify(m)
		path := filepath.Join(t.TempDir(), "bad.json")
		writeJSON(t, path, m)
		if _, err := s.Install(path); err == nil {
			t.Fatal("invalid package accepted")
		}
	}
	catalog, err := s.List()
	if err != nil || len(catalog.Packages) != 1 || catalog.Packages[0].ID != pkg.ID {
		t.Fatal(catalog, err)
	}
	oversized := filepath.Join(t.TempDir(), "huge.json")
	os.WriteFile(oversized, make([]byte, MaxBytes+1), 0600)
	if _, err := Inspect(oversized); err == nil {
		t.Fatal("oversized import accepted")
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("original"), 0600)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(target, link)
	if err := s.Export(pkg.ID, link); err == nil {
		t.Fatal("export followed symlink")
	}
	bytes, _ := os.ReadFile(target)
	if string(bytes) != "original" {
		t.Fatal("export changed linked target")
	}
}
