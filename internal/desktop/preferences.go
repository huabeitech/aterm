// Package desktop defines the platform-independent renderer contract.
package desktop

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
)

//go:embed default-preferences.json
var defaults []byte

const PreferencesMetadataKey = "desktop-preferences-v1"

type Preferences map[string]any

func DefaultPreferences() Preferences {
	return DefaultPreferencesForPlatform(RendererPlatform(runtime.GOOS))
}

// DefaultPreferencesForPlatform follows the Electron renderer shortcut contract.
// Explicit saved bindings are retained by ValidatePreferences.
func DefaultPreferencesForPlatform(platform string) Preferences {
	result := Preferences{}
	if err := json.Unmarshal(defaults, &result); err != nil {
		panic(err)
	}
	result["language"] = "en"
	if platform != "darwin" {
		for key, value := range result {
			if strings.HasSuffix(key, "Shortcut") {
				result[key] = strings.Replace(value.(string), "meta+", "ctrl+", 1)
			}
		}
		result["toggleInspectorShortcut"] = "ctrl+alt+b"
		result["toggleSidebarShortcut"] = "ctrl+shift+b"
	}
	return result
}

// Keep the order aligned with the renderer: migration fills newly introduced
// bindings before validation, without changing any existing user binding.
var shortcutFields = []string{
	"commandPaletteShortcut", "commandSearchShortcut", "toggleSidebarShortcut", "toggleTerminalFilesShortcut",
	"newTerminalShortcut", "closeTerminalShortcut", "previousTerminalShortcut", "nextTerminalShortcut",
	"moveTabLeftShortcut", "moveTabRightShortcut", "splitVerticalShortcut", "splitHorizontalShortcut", "toggleInspectorShortcut",
}

// defaultAddedShortcut mirrors the shared Electron preference migration helpers.
// Sidebar matching normalizes saved bindings; terminal/files retains the source
// contract's exact-string matching. Final validation still rejects duplicates.
func defaultAddedShortcut(input Preferences, field string) string {
	mod := "meta"
	if value, _ := input["newTerminalShortcut"].(string); strings.Contains(value, "ctrl+") {
		mod = "ctrl"
	}
	occupied := map[string]bool{}
	for _, key := range shortcutFields {
		if key == field {
			continue
		}
		value, ok := input[key].(string)
		if !ok {
			continue
		}
		if field != "toggleTerminalFilesShortcut" {
			if normalized, err := normalizeDesktopShortcut(value); err == nil {
				value = normalized
			}
		}
		occupied[value] = true
	}
	var candidates []string
	if field == "toggleSidebarShortcut" {
		if mod == "meta" {
			candidates = append(candidates, "meta+b")
		}
		candidates = append(candidates, mod+"+shift+b", mod+"+alt+b", mod+"+alt+shift+b")
	} else if field == "toggleInspectorShortcut" {
		if mod == "meta" {
			candidates = append(candidates, "meta+shift+b")
		}
		candidates = append(candidates, mod+"+alt+b", mod+"+alt+shift+b")
	} else {
		candidates = append(candidates, mod+"+shift+e", mod+"+alt+e", mod+"+alt+shift+e")
	}
	for index := 1; index <= 24; index++ {
		candidates = append(candidates, fmt.Sprintf("%s+alt+f%d", mod, index))
	}
	for _, value := range candidates {
		if !occupied[value] {
			return value
		}
	}
	return "" // The finite shortcut field count cannot occupy every candidate.
}

func ValidatePreferences(input Preferences) (Preferences, error) {
	result := DefaultPreferences()
	for key, value := range input {
		if _, ok := result[key]; ok {
			result[key] = value
		}
	}
	// Older saves predate these actions. Use their existing modifier convention
	// and select an unoccupied binding instead of merging a conflicting default.
	if input["commandSearchShortcut"] == nil {
		mod := "meta"
		if value, _ := input["commandPaletteShortcut"].(string); strings.Contains(value, "ctrl+") {
			mod = "ctrl"
		}
		result["commandSearchShortcut"] = mod + "+shift+p"
	}
	for _, field := range []string{"toggleSidebarShortcut", "toggleTerminalFilesShortcut"} {
		if input[field] == nil {
			result[field] = defaultAddedShortcut(input, field)
		}
	}
	if input["toggleInspectorShortcut"] == nil {
		result["toggleInspectorShortcut"] = defaultAddedShortcut(result, "toggleInspectorShortcut")
	}
	switch result["language"] {
	case "system", "en", "zh-CN", "zh-TW", "ja", "pt-BR", "es", "de", "fr", "ko":
	default:
		return nil, errors.New("invalid language")
	}
	scale, ok := result["appFontScale"].(float64)
	if !ok || (scale != 90 && scale != 100 && scale != 110 && scale != 120 && scale != 130) {
		return nil, errors.New("invalid app text size")
	}
	size, ok := result["terminalFontSize"].(float64)
	if !ok || size < 10 || size > 24 || size != float64(int(size)) {
		return nil, errors.New("terminal font size must be between 10 and 24")
	}
	for _, key := range []string{"theme", "lightTheme", "darkTheme"} {
		value, ok := result[key].(string)
		if !ok || !isDesktopThemeID(value) || (key != "theme" && value == "system") {
			return nil, fmt.Errorf("invalid %s", key)
		}
	}
	if result["defaultHome"] != "hosts" && result["defaultHome"] != "local-terminal" {
		return nil, errors.New("invalid default home")
	}
	if result["terminalWordNavigation"] != "shell" && result["terminalWordNavigation"] != "terminal" {
		return nil, errors.New("invalid terminal word navigation")
	}
	seen := map[string]bool{}
	for _, key := range shortcutFields {
		value, ok := result[key].(string)
		if !ok || value == "" {
			return nil, fmt.Errorf("invalid %s", key)
		}
		normalized, err := normalizeDesktopShortcut(value)
		if err != nil {
			return nil, err
		}
		if seen[normalized] {
			return nil, errors.New("keyboard shortcuts must be unique")
		}
		seen[normalized] = true
		result[key] = normalized
	}
	return result, nil
}
