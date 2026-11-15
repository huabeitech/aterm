// Package plugins owns the plugin format, immutable packages and lifecycle.
// Native windows and the terminal implementation are injected at the boundary.
package plugins

import "encoding/json"

type Manifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Runtime       string              `json:"runtime"`
	Entry         string              `json:"entry"`
	Permissions   []string            `json:"permissions"`
	Contributes   Contributions       `json:"contributes"`
	Execution     *ExecutionPolicy    `json:"execution,omitempty"`
	I18n          *LocalizationConfig `json:"i18n,omitempty"`
	Engines       *Engines            `json:"engines,omitempty"`
}
type Contributions struct {
	TerminalStatusBar *StatusContribution `json:"terminalStatusBar,omitempty"`
	Views             []View              `json:"views,omitempty"`
	Commands          []Command           `json:"commands,omitempty"`
}
type StatusContribution struct {
	ID string `json:"id"`
}
type View struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Placement    string `json:"placement"`
	Entry        string `json:"entry"`
	Availability string `json:"availability,omitempty"`
}
type Command struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Action *ViewAction `json:"action,omitempty"`
}
type ViewAction struct {
	Type    string          `json:"type"`
	View    string          `json:"view"`
	Context json.RawMessage `json:"context,omitempty"`
}
type ExecutionPolicy struct {
	Programs []string `json:"programs"`
	Targets  []string `json:"targets"`
	UnsetEnv []string `json:"unsetEnv,omitempty"`
	Env      []string `json:"env,omitempty"`
}
type LocalizationConfig struct {
	DefaultLocale string            `json:"defaultLocale"`
	Locales       map[string]string `json:"locales"`
}
type Engines struct {
	HostAPI int `json:"hostApi"`
}
type Localizations struct {
	DefaultLocale string                       `json:"defaultLocale"`
	Messages      map[string]map[string]string `json:"messages"`
}
type Package struct {
	Manifest      Manifest          `json:"manifest"`
	Source        string            `json:"source"`
	BuiltIn       bool              `json:"builtIn"`
	Localizations *Localizations    `json:"localizations,omitempty"`
	Views         map[string]string `json:"views,omitempty"`
}
type Grant struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Config struct {
	Enabled     bool    `json:"enabled"`
	Interval    int     `json:"interval"`
	Key         string  `json:"key,omitempty"`
	PreviousKey string  `json:"previousKey,omitempty"`
	Grants      []Grant `json:"grants"`
}
type Info struct {
	Manifest      Manifest       `json:"manifest"`
	Localizations *Localizations `json:"localizations,omitempty"`
	BuiltIn       bool           `json:"builtIn"`
	Enabled       bool           `json:"enabled"`
	Interval      int            `json:"interval"`
	Grants        []Grant        `json:"grants"`
	State         string         `json:"state"`
	CanRollback   bool           `json:"canRollback"`
	Error         string         `json:"error,omitempty"`
}

// The public SDK normalizes localization config to its two public properties;
// unlike executable manifest declarations, extra dictionary metadata is ignored.
func (config *LocalizationConfig) UnmarshalJSON(data []byte) error {
	type plain LocalizationConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*config = LocalizationConfig(decoded)
	return nil
}
