package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"
)

var pluginID = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9.-]*$`)
var semver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var contributionID = regexp.MustCompile(`^[a-z][a-zA-Z0-9.-]{0,99}$`)
var uiEntry = regexp.MustCompile(`^ui/[a-zA-Z0-9_-]+\.html$`)
var programName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,63}$`)
var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
var localeName = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)

func stringLength(s string) int         { return len(utf16.Encode([]rune(s))) }
func validTitle(s string, max int) bool { return strings.TrimSpace(s) != "" && stringLength(s) <= max }

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid trailing JSON")
	}
	return nil
}

// Optional manifest properties reject explicit null, as the SDK validators do.
func rejectOptionalNull(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for _, key := range []string{"execution", "engines", "i18n"} {
		if v, ok := m[key]; ok && bytes.Equal(v, []byte("null")) {
			return fmt.Errorf("invalid plugin %s", key)
		}
	}
	var c map[string]json.RawMessage
	if err := json.Unmarshal(m["contributes"], &c); err != nil || c == nil {
		return errors.New("invalid plugin contributions")
	}
	for _, key := range []string{"terminalStatusBar", "views", "commands"} {
		if v, ok := c[key]; ok && bytes.Equal(v, []byte("null")) {
			return fmt.Errorf("invalid plugin %s", key)
		}
	}
	// Nested optional fields are also typed, but Go pointers alone cannot tell
	// absence from null. Validate their raw JSON representation before decoding.
	if data, ok := m["execution"]; ok {
		var p map[string]json.RawMessage
		json.Unmarshal(data, &p)
		for _, key := range []string{"env", "unsetEnv"} {
			if v, ok := p[key]; ok && bytes.Equal(v, []byte("null")) {
				return errors.New("invalid execution policy")
			}
		}
	}
	for _, collection := range []string{"commands", "views"} {
		var entries []map[string]json.RawMessage
		json.Unmarshal(c[collection], &entries)
		for _, entry := range entries {
			if availability, exists := entry["availability"]; exists && !bytes.Equal(availability, []byte(`"dynamic"`)) {
				return errors.New("invalid plugin view availability")
			}
			for _, key := range []string{"action", "availability"} {
				if v, ok := entry[key]; ok && bytes.Equal(v, []byte("null")) {
					return errors.New("invalid plugin contribution")
				}
			}
		}
	}
	return nil
}
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > 32768 {
		return m, errors.New("plugin manifest exceeds 32 KB")
	}
	if err := rejectOptionalNull(data); err != nil {
		return m, err
	}
	if err := decodeStrict(data, &m); err != nil {
		return m, fmt.Errorf("invalid plugin manifest: %w", err)
	}
	if m.SchemaVersion != 1 || m.Runtime != "sandbox" || !pluginID.MatchString(m.ID) || len(m.ID) > 100 || !validTitle(m.Name, 100) || !semver.MatchString(m.Version) || m.Entry != "plugin.js" || m.Permissions == nil || len(m.Permissions) > 2 {
		return m, errors.New("unsupported plugin manifest")
	}
	seen := map[string]bool{}
	for _, p := range m.Permissions {
		if (p != "process.execute" && p != "terminal.create") || seen[p] {
			return m, errors.New("unsupported plugin permission")
		}
		seen[p] = true
	}
	slices.Sort(m.Permissions)
	if (len(m.Permissions) > 0) != (m.Execution != nil) {
		return m, errors.New("execution permissions require an execution policy")
	}
	if m.Execution != nil {
		if err := validateExecutionPolicy(m.Execution); err != nil {
			return m, err
		}
	}
	if m.Engines != nil && m.Engines.HostAPI != 1 {
		return m, errors.New("incompatible plugin Host API")
	}
	if m.I18n != nil {
		config := m.I18n
		if len(config.Locales) < 1 || len(config.Locales) > 16 {
			return m, errors.New("invalid plugin localization manifest")
		}
		if _, ok := config.Locales[config.DefaultLocale]; !ok {
			return m, errors.New("invalid plugin default locale")
		}
		for locale, path := range config.Locales {
			if !canonicalLocale(locale) || path != "locales/"+locale+".json" {
				return m, errors.New("plugin locale tags and paths must be canonical")
			}
		}
	}
	if status := m.Contributes.TerminalStatusBar; status != nil && status.ID != m.ID+".status" {
		return m, errors.New("invalid terminal status contribution")
	}
	if len(m.Contributes.Views) > 8 {
		return m, errors.New("too many plugin views")
	}
	ids := map[string]bool{}
	sidePanels := map[string]bool{}
	hasWorkspace := false
	for _, view := range m.Contributes.Views {
		if !contributionID.MatchString(view.ID) || ids[view.ID] || !validTitle(view.Title, 150) || (view.Placement != "terminal.sidePanel" && view.Placement != "terminal.workspace") || !uiEntry.MatchString(view.Entry) || (view.Availability != "" && (view.Availability != "dynamic" || view.Placement != "terminal.workspace")) {
			return m, errors.New("invalid plugin view")
		}
		ids[view.ID] = true
		if view.Placement == "terminal.sidePanel" {
			sidePanels[view.ID] = true
		} else {
			hasWorkspace = true
		}
	}
	if len(m.Contributes.Commands) > 32 {
		return m, errors.New("too many plugin commands")
	}
	ids = map[string]bool{}
	for _, command := range m.Contributes.Commands {
		if !contributionID.MatchString(command.ID) || ids[command.ID] || !validTitle(command.Title, 150) {
			return m, errors.New("invalid plugin command")
		}
		ids[command.ID] = true
		if action := command.Action; action != nil {
			if (action.Type != "view.open" && action.Type != "view.toggle" && action.Type != "view.close") || !sidePanels[action.View] {
				return m, errors.New("invalid plugin command action")
			}
			if len(action.Context) > 0 {
				normalized, err := JSONSnapshot(action.Context, 4096)
				if err != nil {
					return m, err
				}
				action.Context = normalized
			}
		}
	}
	if m.Contributes.TerminalStatusBar == nil && !hasWorkspace {
		return m, errors.New("plugin must contribute a status bar or workspace view")
	}
	return m, nil
}
func validateExecutionPolicy(policy *ExecutionPolicy) error {
	list := func(values []string, max int, valid func(string) bool) error {
		if len(values) < 1 || len(values) > max {
			return errors.New("invalid execution policy")
		}
		seen := map[string]bool{}
		for _, v := range values {
			if !valid(v) || seen[v] {
				return errors.New("invalid execution policy")
			}
			seen[v] = true
		}
		slices.Sort(values)
		return nil
	}
	if err := list(policy.Programs, 16, programName.MatchString); err != nil {
		return err
	}
	if err := list(policy.Targets, 2, func(v string) bool { return v == "local" || v == "ssh" }); err != nil {
		return err
	}
	if policy.Env != nil {
		if err := list(policy.Env, 16, envName.MatchString); err != nil {
			return err
		}
	}
	if policy.UnsetEnv != nil {
		if err := list(policy.UnsetEnv, 16, envName.MatchString); err != nil {
			return err
		}
	}
	return nil
}

// JSONSnapshot is the only value accepted across plugin IPC and storage.
func JSONSnapshot(data []byte, maxBytes int) (json.RawMessage, error) {
	if len(data) > maxBytes {
		return nil, errors.New("plugin data limit")
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, errors.New("plugin data must be JSON")
	}
	nodes := 0
	var visit func(any, int) error
	visit = func(v any, depth int) error {
		nodes++
		if nodes > 20000 || depth > 16 {
			return errors.New("plugin data limit")
		}
		switch item := v.(type) {
		case []any:
			for _, child := range item {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		case map[string]any:
			for key, child := range item {
				if key == "__proto__" || key == "constructor" || key == "prototype" {
					return errors.New("invalid plugin data key")
				}
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(value, 0); err != nil {
		return nil, err
	}
	normalized, err := marshalJSON(value)
	if err == nil && len(normalized) > maxBytes {
		err = errors.New("plugin data limit")
	}
	return normalized, err
}
func marshalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}
