package desktop

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRendererPlatformContract(t *testing.T) {
	for goos, want := range map[string]string{"windows": "win32", "darwin": "darwin", "linux": "linux", "freebsd": "freebsd"} {
		if got := RendererPlatform(goos); got != want {
			t.Errorf("%s: got %s, want %s", goos, got, want)
		}
	}
}

func TestPlatformDefaultShortcuts(t *testing.T) {
	for _, platform := range []string{"darwin", "win32", "linux"} {
		defaults := DefaultPreferencesForPlatform(platform)
		mod, sidebar := "ctrl", "ctrl+shift+b"
		if platform == "darwin" {
			mod, sidebar = "meta", "meta+b"
		}
		for field, suffix := range map[string]string{
			"commandPaletteShortcut": "p", "commandSearchShortcut": "shift+p",
			"toggleTerminalFilesShortcut": "shift+e", "newTerminalShortcut": "t",
			"closeTerminalShortcut": "w", "previousTerminalShortcut": "arrowleft",
			"nextTerminalShortcut": "arrowright", "moveTabLeftShortcut": "shift+arrowleft",
			"moveTabRightShortcut": "shift+arrowright", "splitVerticalShortcut": "d", "splitHorizontalShortcut": "shift+d",
		} {
			if got, want := defaults[field], mod+"+"+suffix; got != want {
				t.Errorf("%s %s: got %v, want %s", platform, field, got, want)
			}
		}
		if defaults["toggleSidebarShortcut"] != sidebar {
			t.Errorf("%s sidebar: %v", platform, defaults["toggleSidebarShortcut"])
		}
		if _, err := ValidatePreferences(defaults); err != nil {
			t.Fatalf("%s defaults invalid: %v", platform, err)
		}
	}
}

func TestSavedShortcutSurvivesPlatformDefaults(t *testing.T) {
	input := DefaultPreferencesForPlatform("win32")
	input["newTerminalShortcut"] = "meta+alt+t"
	got, err := ValidatePreferences(input)
	if err != nil {
		t.Fatal(err)
	}
	if got["newTerminalShortcut"] != "meta+alt+t" {
		t.Fatal("saved custom shortcut replaced", got)
	}
}

// Compare the entire default object against the actual reference, rather than
// a second hand-maintained shortcut table. English is a project requirement.
func TestElectronPlatformDefaultsInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron defaults oracle")
	}
	script := filepath.Join(t.TempDir(), "defaults.mts")
	code := `import {pathToFileURL} from 'node:url';const {platformDefaults}=await import(pathToFileURL(process.argv[2]).href);process.stdout.write(JSON.stringify(Object.fromEntries(['darwin','win32','linux'].map(p=>[p,{...platformDefaults(p),language:'en'}]))));`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, filepath.Join(reference, "src/shared/contracts/preferences.ts")).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron defaults oracle: %v: %s", err, output)
	}
	var expected map[string]Preferences
	if err := json.Unmarshal(output, &expected); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"darwin", "win32", "linux"} {
		if got := DefaultPreferencesForPlatform(platform); !reflect.DeepEqual(got, expected[platform]) {
			t.Errorf("%s defaults differ: Go=%v Electron=%v", platform, got, expected[platform])
		}
	}
}
