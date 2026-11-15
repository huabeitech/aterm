package desktop

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type preferenceVector struct {
	Field string `json:"field"`
	Value any    `json:"value"`
	Valid bool   `json:"valid"`
}

func preferenceContractVectors() []preferenceVector {
	var vectors []preferenceVector
	for _, field := range []string{"theme", "lightTheme", "darkTheme"} {
		for _, value := range []string{"dark", "light", "solarized-dark", "solarized-light", "gruvbox-dark", "dracula", "example.pack/theme-1", strings.Repeat("a", 96) + "/" + strings.Repeat("b", 64)} {
			vectors = append(vectors, preferenceVector{field, value, true})
		}
		vectors = append(vectors, preferenceVector{field, "system", field == "theme"})
		for _, value := range []any{"unknown", "../dark", "a/b/c", "a/../b", "a/b.c", "A/b", "a/B", "a_b/c", strings.Repeat("a", 97) + "/b", "a/" + strings.Repeat("b", 65), "", nil, 42} {
			vectors = append(vectors, preferenceVector{field, value, false})
		}
	}
	for _, value := range []string{"meta+f1", "ctrl+f24", " META + ALT + F12 ", "ctrl+f", "meta+9", "shift+ctrl+arrowleft"} {
		vectors = append(vectors, preferenceVector{"newTerminalShortcut", value, true})
	}
	for _, value := range []any{"meta+q", "ctrl+q", "meta+shift+h", "ctrl+shift+h", "meta+f0", "meta+f25", "ctrl+f01", "ctrl+f999", "meta+ctrl+ctrl+t", "ctrl++t", "alt+t", "", "meta+w"} {
		vectors = append(vectors, preferenceVector{"newTerminalShortcut", value, false})
	}
	return vectors
}
func TestDesktopPreferenceContractBoundaries(t *testing.T) {
	for index, vector := range preferenceContractVectors() {
		input := DefaultPreferencesForPlatform("darwin")
		input[vector.Field] = vector.Value
		_, err := ValidatePreferences(input)
		if (err == nil) != vector.Valid {
			t.Errorf("vector %d %s=%v: valid=%v error=%v", index, vector.Field, vector.Value, vector.Valid, err)
		}
	}
}
func TestElectronPreferenceValidationInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron validation oracle")
	}
	directory := t.TempDir()
	script := filepath.Join(directory, "validation.mts")
	code := `import {readFileSync} from 'node:fs';import {pathToFileURL} from 'node:url';const {validatePreferences,platformDefaults}=await import(pathToFileURL(process.argv[2]).href);process.stdout.write(JSON.stringify(JSON.parse(readFileSync(process.argv[3],'utf8')).map(v=>{try{return {preferences:{...validatePreferences({...platformDefaults('darwin'),[v.field]:v.value}),language:'en'}}}catch{return {error:true}}})));`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	vectors := preferenceContractVectors()
	data, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "vectors.json")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, filepath.Join(reference, "src/shared/contracts/preferences.ts"), input).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron validator: %v: %s", err, output)
	}
	var expected []struct {
		Preferences Preferences `json:"preferences"`
		Error       bool        `json:"error"`
	}
	if err := json.Unmarshal(output, &expected); err != nil {
		t.Fatal(err)
	}
	if len(expected) != len(vectors) {
		t.Fatal("invalid oracle result count")
	}
	for index, vector := range vectors {
		input := DefaultPreferencesForPlatform("darwin")
		input[vector.Field] = vector.Value
		got, err := ValidatePreferences(input)
		if (err != nil) != expected[index].Error {
			t.Errorf("vector %d %s=%v: Go error=%v Electron error=%v", index, vector.Field, vector.Value, err, expected[index].Error)
			continue
		}
		if err == nil && !reflect.DeepEqual(got, expected[index].Preferences) {
			t.Errorf("vector %d: normalized preferences differ", index)
		}
	}
}
