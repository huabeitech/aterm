package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	sdk "ssh-connect/packages/plugin-go"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These tests cross the real native process and reverse gRPC broker, while the
// normal session manager and executor remain the permission/lifetime authority.
func sessionPackage(t *testing.T, name string) Package {
	t.Helper()
	root := filepath.Join("../../../plugins", name)
	raw, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var manifest Manifest
	if e = json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "plugin")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", path, filepath.Join(root, "backend"))
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build: %s %v", out, e)
	}
	binary, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(binary)
	manifest.Backend = &NativeBackend{Platform: runtime.GOOS + "-" + runtime.GOARCH, SHA256: fmt.Sprintf("%x", sum)}
	manifest.Entry = nativeEntry(manifest.Backend.Platform)
	bundle := &Localizations{DefaultLocale: "en", Messages: map[string]map[string]string{}}
	for locale, file := range manifest.I18n.Locales {
		raw, e := os.ReadFile(filepath.Join(root, file))
		if e != nil {
			t.Fatal(e)
		}
		var messages map[string]string
		json.Unmarshal(raw, &messages)
		bundle.Messages[locale] = messages
	}
	views := map[string]string{}
	for _, v := range manifest.Contributes.Views {
		views[v.Entry] = "<div>fixture</div>"
	}
	return Package{Manifest: manifest, Binary: binary, Localizations: bundle, Views: views}
}

type sessionTestHost struct {
	manager *Manager
	id      string
}

func (h *sessionTestHost) Close() error { return nil }
func (h *sessionTestHost) Call(ctx context.Context, m string, p json.RawMessage) (json.RawMessage, error) {
	out, e := h.manager.Request(ctx, h.id, m, p)
	return sdk.JSON(out), e
}
func (h *sessionTestHost) Open(context.Context, string, json.RawMessage) (io.ReadWriteCloser, error) {
	return nil, fmt.Errorf("unused")
}
func TestNativeSessionSharesWorkerReleasesAndRestoresLocale(t *testing.T) {
	p := sessionPackage(t, "resource-monitor")
	root := t.TempDir()
	registry, e := NewRegistry(RegistryOptions{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	if e = registry.Install(p, registry.Revision()); e != nil {
		t.Fatal(e)
	}
	var manager *Manager
	var starts atomic.Int32
	var hybrid *SessionRuntime
	native := NewNativeManager(NativeOptions{Registry: registry, IdleTimeout: 35 * time.Millisecond, Host: func(context.Context, string, Manifest) (NativeHost, error) {
		starts.Add(1)
		return &sessionTestHost{manager, p.Manifest.ID}, nil
	}, Failed: func(id string, generation uint64, reason string) { manager.RuntimeFailed(id, generation, reason) }})
	hybrid = NewSessionRuntime(nil, native)
	executor := NewExecutor(ExecutionOptions{Audit: NewAudit(filepath.Join(root, "audit.json")), Run: func(ctx context.Context, s Session, r ProcessRequest, p ExecutionPolicy, on ProcessData) (ProcessResult, error) {
		code := 0
		return ProcessResult{Stdout: "ATERM_SYSTEM\nfixture\n6.1\nPRETTY_NAME=Linux\nATERM_UPTIME\n100 0\nATERM_CPU\ncpu 1 0 0 9\nATERM_MEMORY\nMemTotal: 1000 kB\nMemAvailable: 500 kB\nATERM_END\n", ExitCode: &code}, nil
	}, StreamEvent: func(id string, e StreamEvent) error { return hybrid.StreamEvent(id, e) }})
	statuses := make(chan Status, 200)
	var prompts atomic.Int32
	manager = NewManager(ManagerOptions{Registry: registry, Runtime: hybrid, Storage: NewStorage(filepath.Join(root, "data")), Capabilities: executor, RefreshTimeout: 3 * time.Second, Authorize: func(context.Context, PermissionRequest) bool { prompts.Add(1); return true }, ResolveSession: func(owner uint, id string) (Session, error) {
		return Session{ID: id, Kind: "ssh", Label: id, Running: true, ExecutionSupported: true, Target: GrantTarget{ID: id, HostName: id, User: "fixture"}}, nil
	}, SendStatus: func(_ uint, s Status) { statuses <- s }})
	defer native.Close()
	defer manager.Close()
	wait := func(session, label string) Status {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case s := <-statuses:
				if s.SessionID == session && s.State == "ready" && (label == "" || s.Items[1].Label == label) {
					return s
				}
			case <-deadline:
				t.Fatalf("missing ready status %s %s", session, label)
				return Status{}
			}
		}
	}
	id := p.Manifest.ID
	if e = manager.Visibility(1, id, "one", "panel", true, false); e != nil {
		t.Fatal(e)
	}
	one := wait("one", "Memory")
	if len(one.Items) != 5 || !strings.Contains(string(one.Data), "\"available\":512000") {
		t.Fatal(string(one.Data))
	}
	manager.Visibility(1, id, "one", "duplicate", true, false)
	manager.Visibility(1, id, "two", "panel", true, false)
	wait("two", "Memory")
	if starts.Load() != 1 || prompts.Load() != 2 {
		t.Fatal("duplicate worker or missing scope authorization", starts.Load(), prompts.Load())
	}
	hybrid.SetLocale("zh-CN")
	wait("one", "内存")
	if err := manager.Refresh(1, id, "two"); err != nil {
		t.Fatal(err)
	}
	wait("two", "内存")
	manager.Visibility(1, id, "one", "panel", false, false)
	manager.Visibility(1, id, "one", "duplicate", false, false)
	manager.Visibility(1, id, "two", "panel", false, false)
	deadline := time.Now().Add(3 * time.Second)
	for {
		slot, _ := native.slot(id)
		slot.op.Lock()
		stopped := slot.rpc == nil
		consumers := len(slot.consumers)
		slot.op.Unlock()
		if stopped && consumers == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker/consumers survived hidden sessions: %d", consumers)
		}
		time.Sleep(10 * time.Millisecond)
	}
	manager.Visibility(1, id, "one", "reopen", true, false)
	wait("one", "内存")
	if starts.Load() != 2 {
		t.Fatal("idle worker did not restart once", starts.Load())
	}
	slot, _ := native.slot(id)
	slot.op.Lock()
	slot.command.Process.Kill()
	slot.op.Unlock()
	deadline = time.Now().Add(3 * time.Second)
	for {
		manager.mu.Lock()
		jobs := len(manager.jobs)
		manager.mu.Unlock()
		if jobs == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crash retained session authority")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
