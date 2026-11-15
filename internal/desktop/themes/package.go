// Package themes owns the versioned theme format and local package catalog.
// No native-window or frontend runtime dependencies belong in this package.
package themes

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"
)

const MaxBytes = 1024 * 1024

//go:embed color-tokens.json
var tokenData []byte

//go:embed builtin-themes.json
var builtinData []byte
var packageID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,95}$`)
var themeID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var color = regexp.MustCompile(`(?i)^#(?:[0-9a-f]{6}|[0-9a-f]{8})$`)
var tokens = func() map[string]bool {
	var names []string
	if err := json.Unmarshal(tokenData, &names); err != nil {
		panic(err)
	}
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}()

type Package struct {
	SchemaVersion   int          `json:"schemaVersion"`
	ThemeAPIVersion int          `json:"themeApiVersion"`
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Version         string       `json:"version"`
	Author          string       `json:"author"`
	License         string       `json:"license,omitempty"`
	Themes          []Definition `json:"themes"`
}
type Definition struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Appearance string            `json:"appearance"`
	Colors     map[string]string `json:"colors"`
	Terminal   *Terminal         `json:"terminal,omitempty"`
}
type Terminal struct {
	Background          string   `json:"background"`
	Foreground          string   `json:"foreground"`
	Cursor              string   `json:"cursor,omitempty"`
	SelectionBackground string   `json:"selectionBackground,omitempty"`
	ANSI                []string `json:"ansi,omitempty"`
}
type Catalog struct {
	Packages []Package `json:"packages"`
	Warnings []string  `json:"warnings"`
}

func text(v any, nonempty bool) bool {
	s, ok := v.(string)
	return ok && (!nonempty || s != "") && len(utf16.Encode([]rune(s))) <= 200
}
func validColor(v any) bool     { s, ok := v.(string); return ok && color.MatchString(s) }
func invalid(path string) error { return fmt.Errorf("invalid theme package: %s", path) }

// Parse normalizes unknown properties exactly like the public Electron contract.
func Parse(data []byte, builtIn bool) (Package, error) {
	var input map[string]any
	if len(data) > MaxBytes {
		return Package{}, fmt.Errorf("theme package exceeds 1 MB")
	}
	if err := json.Unmarshal(data, &input); err != nil || input == nil {
		return Package{}, invalid("expected an object")
	}
	if input["schemaVersion"] != float64(1) || input["themeApiVersion"] != float64(1) {
		return Package{}, invalid("unsupported schemaVersion or themeApiVersion")
	}
	id, ok := input["id"].(string)
	if !ok || !packageID.MatchString(id) || (!builtIn && strings.HasPrefix(id, "aterm.")) {
		return Package{}, invalid("id")
	}
	for _, key := range []string{"name", "author", "version"} {
		if !text(input[key], true) {
			return Package{}, invalid(key)
		}
	}
	if !version.MatchString(input["version"].(string)) {
		return Package{}, invalid("version")
	}
	if v, exists := input["license"]; exists && !text(v, false) {
		return Package{}, invalid("license")
	}
	entries, ok := input["themes"].([]any)
	if !ok || len(entries) < 1 || len(entries) > 64 {
		return Package{}, invalid("themes")
	}
	pkg := Package{SchemaVersion: 1, ThemeAPIVersion: 1, ID: id, Name: input["name"].(string), Version: input["version"].(string), Author: input["author"].(string), Themes: []Definition{}}
	pkg.License, _ = input["license"].(string)
	ids := map[string]bool{}
	for i, value := range entries {
		path := fmt.Sprintf("themes[%d]", i)
		entry, ok := value.(map[string]any)
		if !ok {
			return Package{}, invalid(path)
		}
		id, ok := entry["id"].(string)
		if !ok || !themeID.MatchString(id) || ids[id] {
			return Package{}, invalid(path + ".id")
		}
		ids[id] = true
		if !text(entry["name"], true) {
			return Package{}, invalid(path + ".name")
		}
		appearance, ok := entry["appearance"].(string)
		if !ok || (appearance != "light" && appearance != "dark") {
			return Package{}, invalid(path + ".appearance")
		}
		def := Definition{ID: id, Name: entry["name"].(string), Appearance: appearance, Colors: map[string]string{}}
		if v, exists := entry["colors"]; exists {
			values, ok := v.(map[string]any)
			if !ok {
				return Package{}, invalid(path + ".colors")
			}
			for k, v := range values {
				if !tokens[k] || !validColor(v) {
					return Package{}, invalid(path + ".colors." + k)
				}
				def.Colors[k] = v.(string)
			}
		}
		if v, exists := entry["terminal"]; exists {
			values, ok := v.(map[string]any)
			if !ok {
				return Package{}, invalid(path + ".terminal")
			}
			for _, k := range []string{"background", "foreground"} {
				if !validColor(values[k]) || len(values[k].(string)) != 7 {
					return Package{}, invalid(path + ".terminal." + k)
				}
			}
			term := &Terminal{Background: values["background"].(string), Foreground: values["foreground"].(string)}
			for _, k := range []string{"cursor", "selectionBackground"} {
				if v, exists := values[k]; exists && !validColor(v) {
					return Package{}, invalid(path + ".terminal." + k)
				}
			}
			term.Cursor, _ = values["cursor"].(string)
			term.SelectionBackground, _ = values["selectionBackground"].(string)
			if v, exists := values["ansi"]; exists {
				arr, ok := v.([]any)
				if !ok || len(arr) != 16 {
					return Package{}, invalid(path + ".terminal.ansi")
				}
				for _, c := range arr {
					if !validColor(c) {
						return Package{}, invalid(path + ".terminal.ansi")
					}
					term.ANSI = append(term.ANSI, c.(string))
				}
			}
			def.Terminal = term
		}
		if len(def.Colors) == 0 && def.Terminal == nil {
			return Package{}, invalid(path + ": colors or terminal required")
		}
		pkg.Themes = append(pkg.Themes, def)
	}
	return pkg, nil
}
func Builtin() Package {
	p, err := Parse(builtinData, true)
	if err != nil {
		panic(err)
	}
	return p
}
