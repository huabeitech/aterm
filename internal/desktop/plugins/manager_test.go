package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type refreshCall struct {
	runtimeID, token string
	context          any
}
type fakePluginRuntime struct {
	mu            sync.Mutex
	starts, stops int
	refresh       chan refreshCall
	command       func(context.Context) (json.RawMessage, error)
}

func (f *fakePluginRuntime) Start(ctx context.Context, id string, p Package, fault func(string)) (string, error) {
	f.mu.Lock()
	f.starts++
	f.mu.Unlock()
	return id + "-runtime", nil
}
func (f *fakePluginRuntime) Stop(id string) { f.mu.Lock(); f.stops++; f.mu.Unlock() }
func (f *fakePluginRuntime) Refresh(id, token string, context any) error {
	f.refresh <- refreshCall{id, token, context}
	return nil
}
func (f *fakePluginRuntime) Command(ctx context.Context, id, token, command string, args json.RawMessage) (json.RawMessage, error) {
	if f.command != nil {
		return f.command(ctx)
	}
	return json.RawMessage(`{"fixture":true}`), nil
}
func managerFixture(t *testing.T, execution bool, authorize func(context.Context, PermissionRequest) bool, timeout time.Duration) (*Manager, *Registry, *fakePluginRuntime, chan Status) {
	t.Helper()
	root := t.TempDir()
	registry, _ := NewRegistry(RegistryOptions{Root: root})
	plugin := fixturePackage(t)
	plugin.Manifest.Contributes.Commands = append(plugin.Manifest.Contributes.Commands, Command{ID: "compute", Title: "Compute"})
	if execution {
		plugin.Manifest.Permissions = []string{"process.execute"}
		plugin.Manifest.Execution = &ExecutionPolicy{Programs: []string{"printf"}, Targets: []string{"local"}}
	}
	if err := registry.Install(plugin, registry.Revision()); err != nil {
		t.Fatal(err)
	}
	runtime := &fakePluginRuntime{refresh: make(chan refreshCall, 20)}
	statuses := make(chan Status, 40)
	manager := NewManager(ManagerOptions{Registry: registry, Runtime: runtime, Storage: NewStorage(filepath.Join(root, "data")), Authorize: authorize, RefreshTimeout: timeout, ResolveSession: func(owner uint, id string) (Session, error) {
		if owner != 1 || id != "fixture-session" {
			return Session{}, errors.New("session not found")
		}
		return Session{ID: id, Kind: "local", Label: "Fixture", Running: true, ExecutionSupported: true, Target: GrantTarget{ID: "local", HostName: "localhost", User: "fixture"}}, nil
	}, SendStatus: func(owner uint, status Status) { statuses <- status }})
	t.Cleanup(manager.Close)
	return manager, registry, runtime, statuses
}
func awaitRefresh(t *testing.T, runtime *fakePluginRuntime) refreshCall {
	t.Helper()
	select {
	case call := <-runtime.refresh:
		return call
	case <-time.After(time.Second):
		t.Fatal("plugin was not refreshed")
		return refreshCall{}
	}
}
func awaitStatus(t *testing.T, statuses <-chan Status, state string) Status {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case status := <-statuses:
			if status.State == state {
				return status
			}
		case <-timer.C:
			t.Fatalf("missing %s status", state)
			return Status{}
		}
	}
}
func publishFixture(t *testing.T, m *Manager, token, state string, items any, data any) {
	t.Helper()
	input, _ := json.Marshal(map[string]any{"token": token, "state": state, "items": items, "data": data})
	if _, err := m.Request(context.Background(), "example.monitor", "status.publish", input); err != nil {
		t.Fatal(err)
	}
}
func TestManagerPassiveDiscoveryDoesNotPromptAndConsumersShareJob(t *testing.T) {
	var prompts atomic.Int32
	m, registry, runtime, statuses := managerFixture(t, true, func(context.Context, PermissionRequest) bool { prompts.Add(1); return true }, time.Second)
	if err := m.Visibility(1, "example.monitor", "fixture-session", "discovery", true, true); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, statuses, "permission-required")
	if prompts.Load() != 0 {
		t.Fatal("passive consumer prompted")
	}
	if err := m.Visibility(1, "example.monitor", "fixture-session", "panel", true, false); err != nil {
		t.Fatal(err)
	}
	call := awaitRefresh(t, runtime)
	if prompts.Load() != 1 {
		t.Fatal("missing single host approval")
	}
	publishFixture(t, m, call.token, "ready", []map[string]any{{"id": "fixture", "label": "Label", "value": "Ready", "command": "compute"}}, map[string]any{"sample": 1})
	ready := awaitStatus(t, statuses, "ready")
	if ready.UpdatedAt == nil {
		t.Fatal("ready status has no timestamp")
	}
	if err := m.Visibility(1, "example.monitor", "fixture-session", "second", true, false); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, statuses, "ready")
	runtime.mu.Lock()
	starts := runtime.starts
	runtime.mu.Unlock()
	if starts != 1 {
		t.Fatal("multiple surfaces created duplicate runtimes")
	}
	m.Visibility(1, "example.monitor", "fixture-session", "panel", false, false)
	if _, err := m.Request(context.Background(), "example.monitor", "status.publish", json.RawMessage(`{"token":"`+call.token+`","state":"ready","items":[]}`)); err != nil {
		t.Fatal("hiding one of two panels cancelled shared job", err)
	}
	m.Visibility(1, "example.monitor", "fixture-session", "second", false, false)
	if _, err := m.Request(context.Background(), "example.monitor", "status.publish", json.RawMessage(`{"token":"`+call.token+`","state":"ready","items":[]}`)); err == nil {
		t.Fatal("passive discovery retained the interactive job")
	}
	_, config, _, _ := registry.Package("example.monitor")
	if len(config.Grants) != 1 {
		t.Fatal("approval was not persisted")
	}
}
func TestManagerDelayedPreservesLastGoodDataAndRevocationRequiresExplicitRetry(t *testing.T) {
	var prompts atomic.Int32
	m, registry, runtime, statuses := managerFixture(t, true, func(context.Context, PermissionRequest) bool { prompts.Add(1); return true }, time.Second)
	m.Visibility(1, "example.monitor", "fixture-session", "panel", true, false)
	call := awaitRefresh(t, runtime)
	publishFixture(t, m, call.token, "ready", []map[string]string{{"id": "value", "label": "Label", "value": "Original"}}, map[string]int{"sample": 1})
	ready := awaitStatus(t, statuses, "ready")
	publishFixture(t, m, call.token, "delayed", []any{}, nil)
	delayed := awaitStatus(t, statuses, "delayed")
	if delayed.Items[0].Value != "Original" || string(delayed.Data) != string(ready.Data) || *delayed.UpdatedAt != *ready.UpdatedAt {
		t.Fatal("delayed state discarded the last good sample")
	}
	_, config, _, _ := registry.Package("example.monitor")
	grant := config.Grants[0].ID
	registry.Revoke("example.monitor", grant)
	m.Revoked("example.monitor", grant)
	awaitStatus(t, statuses, "permission-required")
	if _, err := m.Request(context.Background(), "example.monitor", "status.publish", json.RawMessage(`{"token":"`+call.token+`","state":"ready","items":[]}`)); err == nil {
		t.Fatal("revoked token retained authority")
	}
	// Allow the reconcile loop to consume its queued wake without prompting again.
	m.Visibility(1, "example.monitor", "fixture-session", "panel", true, false)
	awaitStatus(t, statuses, "permission-required")
	if prompts.Load() != 1 {
		t.Fatal("revocation immediately requested the same permission again")
	}
	if err := m.Authorize(1, "example.monitor", "fixture-session"); err != nil {
		t.Fatal(err)
	}
	awaitRefresh(t, runtime)
	if prompts.Load() != 2 {
		t.Fatal("explicit retry did not authorize")
	}
}
func TestManagerCommandLifetimeAndStorageScope(t *testing.T) {
	m, _, runtime, statuses := managerFixture(t, false, nil, time.Second)
	m.Visibility(1, "example.monitor", "fixture-session", "panel", true, false)
	call := awaitRefresh(t, runtime)
	publishFixture(t, m, call.token, "ready", []any{}, nil)
	awaitStatus(t, statuses, "ready")
	if _, err := m.Request(context.Background(), "example.monitor", "storage.set", json.RawMessage(`{"key":"fixture","value":{"saved":true},"pluginId":"another.plugin"}`)); err != nil {
		t.Fatal(err)
	}
	value, err := m.Request(context.Background(), "example.monitor", "storage.get", json.RawMessage(`{"key":"fixture"}`))
	if err != nil || string(value.(json.RawMessage)) != `{"saved":true}` {
		t.Fatal(value, err)
	}
	if _, err = m.Request(context.Background(), "another.plugin", "storage.get", json.RawMessage(`{"key":"fixture"}`)); err == nil {
		t.Fatal("storage identity was not bound")
	}
	if _, err = m.ExecuteCommand(context.Background(), 2, "example.monitor", "fixture-session", "show", nil); err == nil {
		t.Fatal("foreign owner executed a command")
	}
	result, err := m.ExecuteCommand(context.Background(), 1, "example.monitor", "fixture-session", "show", nil)
	if err != nil || result.(map[string]any)["action"] == nil {
		t.Fatal("declarative view command was lost", err)
	}
	started := make(chan struct{})
	runtime.command = func(ctx context.Context) (json.RawMessage, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := m.ExecuteCommand(context.Background(), 1, "example.monitor", "fixture-session", "compute", nil)
		done <- err
	}()
	<-started
	m.Visibility(1, "example.monitor", "fixture-session", "panel", false, false)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hidden command survived cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("command was not cancelled")
	}
}
func TestManagerRefreshWatchdogAndMalformedContributions(t *testing.T) {
	m, registry, runtime, _ := managerFixture(t, false, nil, 30*time.Millisecond)
	m.Visibility(1, "example.monitor", "fixture-session", "panel", true, false)
	call := awaitRefresh(t, runtime)
	if _, err := m.Request(context.Background(), "example.monitor", "status.publish", json.RawMessage(`{"token":"`+call.token+`","state":"ready","items":[{"id":"x","label":"x","value":"x","command":"undeclared"}]}`)); err == nil {
		t.Fatal("accepted an undeclared status command")
	}
	deadline := time.After(time.Second)
	for {
		list, _ := registry.List()
		if list[0].State == "failed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("unresponsive refresh did not fail")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestManagerRevokingOneHostPreservesOtherHostJob(t *testing.T) {
	registry, _ := NewRegistry(RegistryOptions{Root: t.TempDir()})
	plugin := fixturePackage(t)
	plugin.Manifest.Permissions = []string{"process.execute"}
	plugin.Manifest.Execution = &ExecutionPolicy{Programs: []string{"printf"}, Targets: []string{"ssh"}}
	if err := registry.Install(plugin, registry.Revision()); err != nil {
		t.Fatal(err)
	}
	runtime := &fakePluginRuntime{refresh: make(chan refreshCall, 20)}
	resolver := func(owner uint, id string) (Session, error) {
		return Session{ID: id, Label: id, Kind: "ssh", Running: true, ExecutionSupported: true, Target: GrantTarget{ID: id, HostName: id + ".test", Port: 22, User: "fixture"}}, nil
	}
	manager := NewManager(ManagerOptions{Registry: registry, Runtime: runtime, ResolveSession: resolver, Authorize: func(context.Context, PermissionRequest) bool { return true }})
	defer manager.Close()
	manager.Visibility(1, plugin.Manifest.ID, "host-a", "panel", true, false)
	first := awaitRefresh(t, runtime)
	manager.Visibility(1, plugin.Manifest.ID, "host-b", "panel", true, false)
	second := awaitRefresh(t, runtime)
	publishFixture(t, manager, first.token, "ready", []any{}, nil)
	publishFixture(t, manager, second.token, "ready", []any{}, nil)
	session, _ := resolver(1, "host-a")
	grantID := GrantID(session.Target, plugin.Manifest)
	if err := registry.Revoke(plugin.Manifest.ID, grantID); err != nil {
		t.Fatal(err)
	}
	manager.Revoked(plugin.Manifest.ID, grantID)
	if _, err := manager.Request(context.Background(), plugin.Manifest.ID, "status.publish", json.RawMessage(`{"token":"`+first.token+`","state":"ready","items":[]}`)); err == nil {
		t.Fatal("revoked host token survived")
	}
	publishFixture(t, manager, second.token, "ready", []any{}, nil)
	runtime.mu.Lock()
	stops := runtime.stops
	runtime.mu.Unlock()
	if stops != 0 {
		t.Fatal("revoking one host stopped other hosts' shared runtime")
	}
}
