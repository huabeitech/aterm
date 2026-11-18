package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	sdk "ssh-connect/packages/plugin-go"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in: only the uniquely named fixture container is changed or removed.
func TestNativeDockerSessionWithLocalEngine(t *testing.T) {
	image := os.Getenv("ATERM_NATIVE_DOCKER_FIXTURE_IMAGE")
	if image == "" {
		t.Skip("set a cached disposable image containing sh and sleep")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name := fmt.Sprintf("aterm-native-acceptance-%d", time.Now().UnixNano())
	output, e := exec.CommandContext(ctx, "docker", "run", "-d", "--network", "none", "--name", name, "--entrypoint", "sh", image, "-c", "echo aterm-native-session-fixture; trap 'exit 0' TERM INT; while :; do sleep 1 & wait; done").CombinedOutput()
	if e != nil {
		t.Fatalf("fixture: %s %v", output, e)
	}
	container := strings.TrimSpace(string(output))
	defer exec.Command("docker", "rm", "-f", container).Run()
	p := sessionPackage(t, "docker")
	registry, _ := NewRegistry(RegistryOptions{Root: t.TempDir()})
	if e = registry.Install(p, registry.Revision()); e != nil {
		t.Fatal(e)
	}
	var manager *Manager
	var hybrid *SessionRuntime
	var starts atomic.Int32
	var opened, closed atomic.Bool
	native := NewNativeManager(NativeOptions{Registry: registry, Host: func(context.Context, string, Manifest) (NativeHost, error) {
		starts.Add(1)
		return &sessionTestHost{manager, p.Manifest.ID}, nil
	}})
	hybrid = NewSessionRuntime(nil, native)
	executor := NewExecutor(ExecutionOptions{Audit: NewAudit(filepath.Join(t.TempDir(), "audit.json")), Run: func(ctx context.Context, _ Session, r ProcessRequest, p ExecutionPolicy, on ProcessData) (ProcessResult, error) {
		return RunLocalProcess(ctx, r, p, on)
	}, StreamEvent: func(id string, e StreamEvent) error { return hybrid.StreamEvent(id, e) }, OpenTerminal: func(ctx context.Context, _ uint, _ Session, r ProcessRequest, p ExecutionPolicy) (string, error) {
		if !strings.Contains(strings.Join(r.Args, " "), "exec -it "+container) || len(r.Checks) != 1 {
			t.Error("missing pinned terminal identity", r)
		}
		opened.Store(true)
		return "fixture-terminal", nil
	}, CloseTerminal: func(uint, string) { closed.Store(true) }})
	statuses := make(chan Status, 200)
	manager = NewManager(ManagerOptions{Registry: registry, Runtime: hybrid, Capabilities: executor, Storage: NewStorage(t.TempDir()), ResolveSession: func(uint, string) (Session, error) {
		return Session{ID: "local", Kind: "local", Label: "Local", Running: true, ExecutionSupported: true, Target: GrantTarget{ID: "local", HostName: "localhost", User: "fixture"}}, nil
	}, Authorize: func(context.Context, PermissionRequest) bool { return true }, SendStatus: func(_ uint, s Status) { statuses <- s }})
	defer native.Close()
	defer manager.Close()
	if e = manager.Visibility(1, p.Manifest.ID, "local", "panel", true, false); e != nil {
		t.Fatal(e)
	}
	for {
		select {
		case s := <-statuses:
			if s.State == "ready" {
				v := s.Views["docker.containers"]
				if !v.Available {
					t.Fatalf("Docker unavailable: %+v", v)
				}
				goto ready
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
ready:
	invoke := func(command string, args any) json.RawMessage {
		t.Helper()
		out, e := manager.ExecuteCommand(ctx, 1, p.Manifest.ID, "local", command, sdk.JSON(args))
		if e != nil {
			t.Fatal(e)
		}
		var envelope struct{ Data json.RawMessage }
		json.Unmarshal(sdk.JSON(out), &envelope)
		var failure struct{ Error string }
		json.Unmarshal(envelope.Data, &failure)
		if failure.Error != "" {
			t.Fatal(failure.Error)
		}
		return envelope.Data
	}
	var connect struct {
		Data struct{ Context, Engine string }
	}
	json.Unmarshal(invoke("docker.request", map[string]string{"operation": "connect"}), &connect)
	if connect.Data.Engine == "" {
		t.Fatal("missing engine identity")
	}
	run := func(op, id string) json.RawMessage {
		return invoke("docker.request", map[string]string{"operation": op, "id": id, "context": connect.Data.Context, "engine": connect.Data.Engine})
	}
	if !strings.Contains(string(run("list", "")), container) || !strings.Contains(string(run("inspect", container)), "RestartPolicy") {
		t.Fatal("container data missing")
	}
	if !strings.Contains(string(run("images.list", "")), "Containers") {
		t.Fatal("image usage missing")
	}
	run("logs", container)
	invoke("docker.logs", map[string]string{"action": "start", "subscription": "fixture", "id": container, "context": connect.Data.Context, "engine": connect.Data.Engine})
	for {
		select {
		case s := <-statuses:
			if strings.Contains(string(s.Data), "aterm-native-session-fixture") {
				goto streamed
			}
		case <-ctx.Done():
			t.Fatal("missing live logs")
		}
	}
streamed:
	invoke("docker.logs", map[string]string{"action": "stop", "subscription": "fixture"})
	run("exec", container)
	if !opened.Load() {
		t.Fatal("interactive terminal capability missing")
	}
	run("stop", container)
	run("start", container)
	run("restart", container)
	run("stop", container)
	run("remove", container)
	manager.Visibility(1, p.Manifest.ID, "local", "panel", false, false)
	deadline := time.Now().Add(2 * time.Second)
	for {
		executor.mu.Lock()
		streams := len(executor.streams)
		executor.mu.Unlock()
		if streams == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hidden workspace retained log streams")
		}
		time.Sleep(10 * time.Millisecond)
	}
	manager.SessionChanged(1, "local", true)
	if !closed.Load() {
		t.Fatal("source terminal close retained exec terminal")
	}
	if starts.Load() != 1 {
		t.Fatal("session commands spawned multiple workers", starts.Load())
	}
}

// A loopback SSH server executes commands in a disposable Linux container, so
// collection crosses both real gRPC and SSH without accessing a personal server.
func TestNativeResourceMonitorOverSSH(t *testing.T) {
	image := os.Getenv("ATERM_NATIVE_DOCKER_FIXTURE_IMAGE")
	if image == "" {
		t.Skip("set a cached Linux fixture image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := fmt.Sprintf("aterm-monitor-acceptance-%d", time.Now().UnixNano())
	output, e := exec.CommandContext(ctx, "docker", "run", "-d", "--network", "none", "--name", name, "--entrypoint", "sh", image, "-c", "trap 'exit 0' TERM INT; while :; do sleep 1 & wait; done").CombinedOutput()
	if e != nil {
		t.Fatalf("fixture: %s %v", output, e)
	}
	container := strings.TrimSpace(string(output))
	defer exec.Command("docker", "rm", "-f", container).Run()
	client := sshExecFixture(t, func(ctx context.Context, command string) *exec.Cmd {
		return exec.CommandContext(ctx, "docker", "exec", container, "sh", "-c", command)
	})
	p := sessionPackage(t, "resource-monitor")
	registry, _ := NewRegistry(RegistryOptions{Root: t.TempDir()})
	if e = registry.Install(p, registry.Revision()); e != nil {
		t.Fatal(e)
	}
	var manager *Manager
	native := NewNativeManager(NativeOptions{Registry: registry, Host: func(context.Context, string, Manifest) (NativeHost, error) {
		return &sessionTestHost{manager, p.Manifest.ID}, nil
	}})
	hybrid := NewSessionRuntime(nil, native)
	executor := NewExecutor(ExecutionOptions{Audit: NewAudit(filepath.Join(t.TempDir(), "audit.json")), Run: func(ctx context.Context, _ Session, r ProcessRequest, p ExecutionPolicy, on ProcessData) (ProcessResult, error) {
		return RunSSHProcess(ctx, client, r, p, on)
	}})
	statuses := make(chan Status, 50)
	manager = NewManager(ManagerOptions{Registry: registry, Runtime: hybrid, Capabilities: executor, Storage: NewStorage(t.TempDir()), Authorize: func(context.Context, PermissionRequest) bool { return true }, ResolveSession: func(uint, string) (Session, error) {
		return Session{ID: "ssh", Kind: "ssh", Label: "Linux fixture", Running: true, ExecutionSupported: true, Target: GrantTarget{ID: "ssh", HostName: "127.0.0.1", User: "fixture"}}, nil
	}, SendStatus: func(_ uint, s Status) { statuses <- s }})
	defer native.Close()
	defer manager.Close()
	manager.Visibility(1, p.Manifest.ID, "ssh", "status", true, false)
	wait := func() json.RawMessage {
		t.Helper()
		for {
			select {
			case s := <-statuses:
				if s.State == "ready" {
					return s.Data
				}
			case <-ctx.Done():
				t.Fatal("no Linux sample", ctx.Err())
				return nil
			}
		}
	}
	var data struct {
		Resources struct {
			System  struct{ OS string }
			Memory  struct{ Total float64 }
			Disks   []json.RawMessage
			Network []json.RawMessage
			CPU     struct{ Usage *float64 }
		}
	}
	if e = json.Unmarshal(wait(), &data); e != nil || data.Resources.Memory.Total <= 0 || data.Resources.System.OS == "" || len(data.Resources.Disks) == 0 || data.Resources.CPU.Usage != nil {
		t.Fatalf("invalid first Linux sample %+v %v", data, e)
	}
	time.Sleep(100 * time.Millisecond)
	manager.Refresh(1, p.Manifest.ID, "ssh")
	if e = json.Unmarshal(wait(), &data); e != nil || data.Resources.CPU.Usage == nil {
		t.Fatal("missing CPU delta after second sample", e)
	}
	manager.SessionChanged(1, "ssh", true)
	session, err := client.NewSession()
	if err != nil {
		t.Fatal("monitor cleanup closed parent SSH transport", err)
	}
	session.Close()
}
