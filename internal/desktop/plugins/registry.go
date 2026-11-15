package plugins

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"ssh-connect/internal/fsutil"
	"strings"
	"sync"
)

type RegistryOptions struct {
	Root        string
	SafeMode    bool
	MacAppStore bool
}
type registryRecord struct {
	SchemaVersion int               `json:"schemaVersion"`
	Plugins       map[string]Config `json:"plugins"`
}
type registryEntry struct {
	plugin     Package
	config     Config
	state      string
	error      string
	generation uint64
}
type Registry struct {
	mu           sync.Mutex
	packages     *PackageStore
	path         string
	entries      map[string]*registryEntry
	safeMode     bool
	mas          bool
	startupError error
	epoch        uint64
}

func NewRegistry(options RegistryOptions) (*Registry, error) {
	packages, err := NewPackageStore(filepath.Join(options.Root, "packages"))
	if err != nil {
		return nil, err
	}
	r := &Registry{packages: packages, path: filepath.Join(options.Root, "registry.json"), entries: map[string]*registryEntry{}, safeMode: options.SafeMode, mas: options.MacAppStore}
	data, err := regularRead(r.path, 2*1024*1024)
	if os.IsNotExist(err) {
		return r, nil
	}
	var saved registryRecord
	if err == nil {
		err = json.Unmarshal(data, &saved)
		if err == nil && (saved.SchemaVersion != 1 || saved.Plugins == nil) {
			err = errors.New("invalid plugin registry")
		}
	}
	if err != nil {
		r.startupError = errors.New("plugin registry could not be read; restore plugins/registry.json from a local recovery copy")
		r.safeMode = true
		return r, nil
	}
	if r.mas {
		return r, nil
	}
	for id, config := range saved.Plugins {
		if !pluginID.MatchString(id) || strings.HasPrefix(id, "aterm.") || config.Key == "" {
			continue
		}
		if config.Interval != 5 && config.Interval != 10 && config.Interval != 30 {
			config.Interval = 5
		}
		if config.Grants == nil {
			config.Grants = []Grant{}
		}
		plugin, err := packages.Load(config.Key)
		entry := &registryEntry{plugin: plugin, config: config, state: "inactive"}
		if err != nil || plugin.Manifest.ID != id {
			entry.plugin = Package{Manifest: Manifest{SchemaVersion: 1, ID: id, Name: id, Version: "0.0.0", Runtime: "sandbox", Entry: "plugin.js", Permissions: []string{}, Contributes: Contributions{TerminalStatusBar: &StatusContribution{ID: id + ".status"}}}}
			entry.state = "failed"
			entry.error = "Plugin package is missing or its integrity check failed. Uninstall and reinstall it."
		}
		r.entries[id] = entry
	}
	return r, nil
}
func clone[T any](value T) T {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var copy T
	if err = json.Unmarshal(data, &copy); err != nil {
		panic(err)
	}
	return copy
}
func (r *Registry) List() ([]Info, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.startupError != nil {
		return nil, r.startupError
	}
	ids := make([]string, 0, len(r.entries))
	for id := range r.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]Info, 0, len(ids))
	for _, id := range ids {
		e := r.entries[id]
		message := e.error
		if r.safeMode {
			message = "Plugins disabled in safe mode"
		}
		result = append(result, Info{Manifest: e.plugin.Manifest, Localizations: e.plugin.Localizations, BuiltIn: e.plugin.BuiltIn, Enabled: !r.safeMode && e.config.Enabled, Interval: e.config.Interval, Grants: e.config.Grants, State: e.state, CanRollback: e.config.PreviousKey != "", Error: message})
	}
	return clone(result), nil
}
func (r *Registry) save() error {
	if r.startupError != nil {
		return r.startupError
	}
	plugins := map[string]Config{}
	for id, e := range r.entries {
		plugins[id] = e.config
	}
	data, err := marshalJSON(registryRecord{SchemaVersion: 1, Plugins: plugins})
	if err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(r.path, data, 0600)
}
func (r *Registry) Package(id string) (Package, Config, uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil {
		return Package{}, Config{}, 0, errors.New("plugin not found")
	}
	return clone(e.plugin), clone(e.config), e.generation, nil
}
func (r *Registry) InstallationAllowed() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.startupError != nil {
		return r.startupError
	}
	if r.mas || r.safeMode {
		return errors.New("plugin installation unavailable")
	}
	return nil
}
func (r *Registry) Revision() uint64 { r.mu.Lock(); defer r.mu.Unlock(); return r.epoch }

// Install commits an already inspected and runtime-validated package. An epoch
// captured before the native approval/validation prevents stale replacements.
func (r *Registry) Install(plugin Package, expectedEpoch uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.startupError != nil {
		return r.startupError
	}
	if r.mas || r.safeMode {
		return errors.New("plugin installation unavailable")
	}
	if r.epoch != expectedEpoch {
		return errors.New("plugin changed during installation")
	}
	existing := r.entries[plugin.Manifest.ID]
	if existing != nil && existing.plugin.Manifest.Version == plugin.Manifest.Version {
		return errors.New("this plugin version is already installed")
	}
	key, err := r.packages.Install(plugin)
	if err != nil {
		return err
	}
	normalized, err := r.packages.Load(key)
	if err != nil {
		return err
	}
	config := Config{Enabled: true, Interval: 5, Key: key, Grants: []Grant{}}
	generation := uint64(0)
	if existing != nil {
		config.Enabled = existing.config.Enabled
		config.Interval = existing.config.Interval
		generation = existing.generation + 1
		if _, err = r.packages.Load(existing.config.Key); err == nil {
			config.PreviousKey = existing.config.Key
		}
		if PolicyDigest(existing.plugin.Manifest) == PolicyDigest(normalized.Manifest) {
			config.Grants = existing.config.Grants
		}
	}
	r.entries[normalized.Manifest.ID] = &registryEntry{plugin: normalized, config: config, state: "inactive", generation: generation}
	if err = r.save(); err != nil {
		if existing == nil {
			delete(r.entries, normalized.Manifest.ID)
		} else {
			r.entries[normalized.Manifest.ID] = existing
		}
		return err
	}
	r.epoch++
	return nil
}
func (r *Registry) Configure(id string, enabled bool, interval int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil {
		return errors.New("plugin not found")
	}
	if interval != 5 && interval != 10 && interval != 30 {
		return errors.New("invalid plugin settings")
	}
	old := e.config
	e.config = clone(old)
	e.config.Enabled = enabled
	e.config.Interval = interval
	if err := r.save(); err != nil {
		e.config = old
		return err
	}
	e.error = ""
	e.state = "inactive"
	e.generation++
	r.epoch++
	return nil
}
func (r *Registry) Rollback(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.entries[id]
	if old == nil {
		return errors.New("plugin not found")
	}
	if old.config.PreviousKey == "" || old.plugin.BuiltIn {
		return errors.New("no previous plugin version")
	}
	plugin, err := r.packages.Load(old.config.PreviousKey)
	if err != nil {
		return err
	}
	if plugin.Manifest.ID != id {
		return errors.New("plugin ID mismatch")
	}
	config := clone(old.config)
	config.Key, config.PreviousKey = config.PreviousKey, config.Key
	if PolicyDigest(plugin.Manifest) != PolicyDigest(old.plugin.Manifest) {
		config.Grants = []Grant{}
	}
	r.entries[id] = &registryEntry{plugin: plugin, config: config, state: "inactive", generation: old.generation + 1}
	if err = r.save(); err != nil {
		r.entries[id] = old
		return err
	}
	r.epoch++
	return nil
}
func (r *Registry) Uninstall(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.entries[id]
	if old == nil {
		return errors.New("plugin not found")
	}
	if old.plugin.BuiltIn {
		return errors.New("built-in plugins cannot be uninstalled")
	}
	delete(r.entries, id)
	if err := r.save(); err != nil {
		r.entries[id] = old
		return err
	}
	r.epoch++
	return nil
}
func (r *Registry) Grant(id string, grant Grant, expectedGeneration uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil {
		return errors.New("plugin not found")
	}
	if e.generation != expectedGeneration {
		return errors.New("plugin changed during authorization")
	}
	if len(grant.ID) != 64 || !validTitle(grant.Label, 500) {
		return errors.New("invalid plugin grant")
	}
	old := e.config.Grants
	next := []Grant{}
	for _, existing := range old {
		if existing.Label != grant.Label {
			next = append(next, existing)
		}
	}
	for _, existing := range next {
		if existing.ID == grant.ID {
			return nil
		}
	}
	e.config.Grants = append(next, grant)
	if err := r.save(); err != nil {
		e.config.Grants = old
		return err
	}
	r.epoch++
	return nil
}
func (r *Registry) Revoke(id, grantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil {
		return errors.New("plugin not found")
	}
	old := e.config.Grants
	next := []Grant{}
	for _, grant := range old {
		if grant.ID != grantID {
			next = append(next, grant)
		}
	}
	e.config.Grants = next
	if err := r.save(); err != nil {
		e.config.Grants = old
		return err
	}
	r.epoch++
	return nil
}
func (r *Registry) SetRuntimeState(id string, generation uint64, state, message string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil || e.generation != generation {
		return false
	}
	if state != "inactive" && state != "active" && state != "failed" {
		return false
	}
	e.state = state
	e.error = message
	return true
}
func PolicyDigest(manifest Manifest) string {
	permissions := append([]string{}, manifest.Permissions...)
	sort.Strings(permissions)
	var execution any
	if p := manifest.Execution; p != nil {
		programs := append([]string{}, p.Programs...)
		targets := append([]string{}, p.Targets...)
		unset := append([]string{}, p.UnsetEnv...)
		env := append([]string{}, p.Env...)
		sort.Strings(programs)
		sort.Strings(targets)
		sort.Strings(unset)
		sort.Strings(env)
		execution = struct {
			Programs []string `json:"programs"`
			Targets  []string `json:"targets"`
			UnsetEnv []string `json:"unsetEnv"`
			Env      []string `json:"env"`
		}{programs, targets, unset, env}
	}
	data, _ := marshalJSON(struct {
		Permissions []string `json:"permissions"`
		Execution   any      `json:"execution"`
	}{permissions, execution})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

type GrantTarget struct {
	ID         string
	HostName   string
	Port       int
	User       string
	JumpHostID string
	ProxyJump  string
}

func GrantID(target GrantTarget, manifest Manifest) string {
	data, _ := marshalJSON(struct {
		PluginID string `json:"pluginId"`
		Target   []any  `json:"target"`
		Policy   string `json:"policy"`
	}{manifest.ID, []any{target.ID, target.HostName, target.Port, target.User, target.JumpHostID, target.ProxyJump}, PolicyDigest(manifest)})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// Available includes safe mode and runtime faults, rather than only saved config.
func (r *Registry) Available(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	return e != nil && !r.safeMode && r.startupError == nil && e.config.Enabled && e.error == ""
}
