package desktop

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func preferenceMigrationVectors() []Preferences {
	var vectors []Preferences
	for _, platform := range []string{"darwin", "win32", "linux"} {
		for scenario := 0; scenario < 5; scenario++ {
			input := DefaultPreferencesForPlatform(platform)
			delete(input, "toggleSidebarShortcut")
			delete(input, "toggleTerminalFilesShortcut")
			delete(input, "commandSearchShortcut")
			mod := "ctrl"
			if platform == "darwin" {
				mod = "meta"
			}
			switch scenario {
			case 1:
				input["newTerminalShortcut"] = mod + "+shift+b"
				input["closeTerminalShortcut"] = mod + "+shift+e"
			case 2:
				input["newTerminalShortcut"] = mod + "+alt+b"
				input["closeTerminalShortcut"] = mod + "+shift+e"
				input["splitVerticalShortcut"] = mod + "+alt+e"
				input["splitHorizontalShortcut"] = mod + "+alt+shift+e"
			case 3:
				input["toggleSidebarShortcut"] = nil
				input["toggleTerminalFilesShortcut"] = nil
				input["commandSearchShortcut"] = nil
			case 4:
				input["newTerminalShortcut"] = mod + "+shift+b"
				input["closeTerminalShortcut"] = mod + "+alt+b"
				input["splitVerticalShortcut"] = mod + "+alt+shift+b"
				input["splitHorizontalShortcut"] = mod + "+b"
			}
			vectors = append(vectors, input)
		}
	}
	return vectors
}

func TestMissingPreferenceBindingsMigrateWithoutChangingSavedBindings(t *testing.T) {
	for index, input := range preferenceMigrationVectors() {
		before, _ := json.Marshal(input)
		result, err := ValidatePreferences(input)
		if err != nil {
			t.Fatalf("vector %d: %v", index, err)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatalf("vector %d mutated input", index)
		}
		for field, value := range input {
			if value != nil && result[field] != value {
				t.Errorf("vector %d changed %s from %v to %v", index, field, value, result[field])
			}
		}
	}
}

func TestExplicitDuplicatePreferencesStillRejected(t *testing.T) {
	input := DefaultPreferences()
	input["toggleSidebarShortcut"] = input["newTerminalShortcut"]
	if _, err := ValidatePreferences(input); err == nil {
		t.Fatal("explicit conflicting binding silently changed")
	}
}

func TestElectronPreferenceMigrationInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron preference oracle")
	}
	directory := t.TempDir()
	script := filepath.Join(directory, "preferences.mts")
	code := `import {readFileSync} from 'node:fs';import {pathToFileURL} from 'node:url';const {validatePreferences}=await import(pathToFileURL(process.argv[2]).href);process.stdout.write(JSON.stringify(JSON.parse(readFileSync(process.argv[3],'utf8')).map(v=>({...validatePreferences(v),language:'en'}))));`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	vectors := preferenceMigrationVectors()
	data, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "input.json")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, filepath.Join(reference, "src/shared/contracts/preferences.ts"), input).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron preferences oracle: %v: %s", err, output)
	}
	var expected []Preferences
	if err := json.Unmarshal(output, &expected); err != nil {
		t.Fatal(err)
	}
	if len(expected) != len(vectors) {
		t.Fatal("invalid oracle vector count")
	}
	for index, input := range vectors {
		got, err := ValidatePreferences(input)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, expected[index]) {
			t.Errorf("vector %d differs: Go=%v Electron=%v", index, got, expected[index])
		}
	}
}
