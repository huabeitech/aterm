package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "ssh-connect/packages/plugin-go"
)

var nativeTestBinary []byte
var nativeBuild sync.Once
var nativeBuildError error

func nativeTestPackage(t *testing.T) Package {
	t.Helper()
	nativeBuild.Do(func() {
		root := t.TempDir()
		path := filepath.Join(root, filepath.Base(nativeEntry(runtime.GOOS+"-"+runtime.GOARCH)))
		cmd := exec.Command("go", "build", "-o", path, "../../../plugins/ssh-tunnels/backend")
		output, err := cmd.CombinedOutput()
		if err != nil {
			nativeBuildError = fmt.Errorf("build plugin: %s: %w", output, err)
			return
		}
		nativeTestBinary, nativeBuildError = os.ReadFile(path)
	})
	if nativeBuildError != nil {
		t.Fatal(nativeBuildError)
	}
	sum := sha256.Sum256(nativeTestBinary)
	platform := runtime.GOOS + "-" + runtime.GOARCH
	return Package{Manifest: Manifest{SchemaVersion: 1, ID: "huabei.ssh-tunnels", Name: "SSH tunnels", Version: "1.0.0", Runtime: "native", Entry: nativeEntry(platform), Backend: &NativeBackend{Platform: platform, SHA256: fmt.Sprintf("%x", sum)}, Permissions: []string{"native.execute", "hosts.read", "ssh.connect"}, Contributes: Contributions{Views: []View{{ID: "tunnels.workspace", Title: "Tunnels", Entry: "ui/workspace.html", Placement: "app.workspace", Locations: []string{"sidebar", "connection"}}}}}, Binary: nativeTestBinary, Views: map[string]string{"ui/workspace.html": "<p>fixture</p>"}}
}

type fakeNativeHost struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	configs     json.RawMessage
	connections map[string]chan struct{}
}

func newFakeNativeHost(ctx context.Context) *fakeNativeHost {
	ctx, cancel := context.WithCancel(ctx)
	return &fakeNativeHost{ctx: ctx, cancel: cancel, configs: sdk.JSON([]any{}), connections: map[string]chan struct{}{}}
}
func (h *fakeNativeHost) Close() error { h.cancel(); return nil }
func (h *fakeNativeHost) Call(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Key   string
		Value json.RawMessage
		ID    string
	}
	_ = json.Unmarshal(raw, &p)
	switch method {
	case "storage.get":
		h.mu.Lock()
		defer h.mu.Unlock()
		return append(json.RawMessage{}, h.configs...), nil
	case "storage.set":
		h.mu.Lock()
		defer h.mu.Unlock()
		h.configs = append(json.RawMessage{}, p.Value...)
		return sdk.JSON(nil), nil
	case "ssh.open":
		h.mu.Lock()
		defer h.mu.Unlock()
		id := fmt.Sprint(len(h.connections))
		h.connections[id] = make(chan struct{})
		return sdk.JSON(map[string]string{"id": id}), nil
	case "ssh.close":
		h.mu.Lock()
		if c := h.connections[p.ID]; c != nil {
			close(c)
			delete(h.connections, p.ID)
		}
		h.mu.Unlock()
		return sdk.JSON(nil), nil
	case "ssh.wait":
		h.mu.Lock()
		c := h.connections[p.ID]
		h.mu.Unlock()
		select {
		case <-c:
			return sdk.JSON(nil), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.ctx.Done():
			return nil, h.ctx.Err()
		}
	case "hosts.list":
		return sdk.JSON([]any{}), nil
	}
	return nil, fmt.Errorf("unsupported %s", method)
}
func (h *fakeNativeHost) Open(ctx context.Context, method string, raw json.RawMessage) (io.ReadWriteCloser, error) {
	var p struct{ Address string }
	_ = json.Unmarshal(raw, &p)
	return (&net.Dialer{}).DialContext(ctx, "tcp", p.Address)
}
func nativeManagerFixture(t *testing.T, idle time.Duration) (*NativeManager, *Registry, *atomic.Int32) {
	t.Helper()
	r, err := NewRegistry(RegistryOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Install(nativeTestPackage(t), r.Revision()); err != nil {
		t.Fatal(err)
	}
	starts := new(atomic.Int32)
	m := NewNativeManager(NativeOptions{Registry: r, IdleTimeout: idle, Host: func(ctx context.Context, _ string, _ Manifest) (NativeHost, error) {
		starts.Add(1)
		return newFakeNativeHost(ctx), nil
	}})
	t.Cleanup(m.Close)
	return m, r, starts
}
func callNative(t *testing.T, m *NativeManager, op string, args map[string]any) json.RawMessage {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["op"] = op
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	value, err := m.Call(ctx, "huabei.ssh-tunnels", "tunnels.request", sdk.JSON(args))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestNativeConcurrentActivationAndReplacement(t *testing.T) {
	m, r, starts := nativeManagerFixture(t, time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); callNative(t, m, "list", nil) }()
	}
	wg.Wait()
	if starts.Load() != 1 {
		t.Fatalf("started %d backends", starts.Load())
	}
	var before struct{ PID int }
	raw, err := m.Call(context.Background(), "huabei.ssh-tunnels", "runtime.status", nil)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(raw, &before)
	if before.PID == 0 {
		t.Fatal("missing real child PID")
	}
	if err = m.Change("huabei.ssh-tunnels", func() error { return r.Configure("huabei.ssh-tunnels", false, 5) }); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Call(context.Background(), "huabei.ssh-tunnels", "runtime.status", nil); err == nil {
		t.Fatal("disabled plugin started")
	}
	if err = m.Change("huabei.ssh-tunnels", func() error { return r.Configure("huabei.ssh-tunnels", true, 5) }); err != nil {
		t.Fatal(err)
	}
	raw, err = m.Call(context.Background(), "huabei.ssh-tunnels", "runtime.status", nil)
	if err != nil {
		t.Fatal(err)
	}
	var after struct{ PID int }
	json.Unmarshal(raw, &after)
	if before.PID == after.PID || starts.Load() != 2 {
		t.Fatal("old process reused after replacement")
	}
}
func TestNativeTunnelSurvivesViewsAndReleasesPort(t *testing.T) {
	m, _, starts := nativeManagerFixture(t, 40*time.Millisecond)
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); data, _ := io.ReadAll(c); _, _ = c.Write(append([]byte("reply:"), data...)) }()
		}
	}()
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	raw := callNative(t, m, "save", map[string]any{"input": map[string]any{"name": "fixture", "hostID": "host", "localAddress": "127.0.0.1", "localPort": port, "targetAddress": "127.0.0.1", "targetPort": target.Addr().(*net.TCPAddr).Port}})
	var config struct{ ID string }
	json.Unmarshal(raw, &config)
	if err = m.Visibility("huabei.ssh-tunnels", "view:one", true); err != nil {
		t.Fatal(err)
	}
	callNative(t, m, "start", map[string]any{"id": config.ID})
	_ = m.Visibility("huabei.ssh-tunnels", "view:one", false)
	address := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		var rows []struct {
			State string
			Error string
		}
		json.Unmarshal(callNative(t, m, "list", nil), &rows)
		if rows[0].State == "running" {
			break
		}
		if rows[0].State == "failed" || time.Now().After(deadline) {
			t.Fatalf("did not start: %+v", rows)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	c, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = c.Write([]byte("half-close"))
	_ = c.(*net.TCPConn).CloseWrite()
	reply, err := io.ReadAll(c)
	c.Close()
	if err != nil || string(reply) != "reply:half-close" {
		t.Fatalf("stream/half close = %q, %v", reply, err)
	}
	if starts.Load() != 1 {
		t.Fatalf("task restarted: %d", starts.Load())
	}
	m.Reset()
	l, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal("reset leaked listener", err)
	}
	l.Close()
}
func TestNativeIdleAndClose(t *testing.T) {
	m, _, starts := nativeManagerFixture(t, 20*time.Millisecond)
	callNative(t, m, "list", nil)
	time.Sleep(100 * time.Millisecond)
	callNative(t, m, "list", nil)
	if starts.Load() != 2 {
		t.Fatalf("idle worker was not reclaimed: %d", starts.Load())
	}
	m.Close()
	if _, err := m.Call(context.Background(), "huabei.ssh-tunnels", "runtime.status", nil); err == nil {
		t.Fatal("started after shutdown")
	}
}
func TestPluginProfileLock(t *testing.T) {
	root := t.TempDir()
	first, err := LockProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := LockProfile(root); err == nil {
		second.Close()
		t.Fatal("accepted duplicate profile")
	}
	first.Close()
	next, err := LockProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestNativeLeaseDisconnectExitsWorker(t *testing.T) {
	m, _, _ := nativeManagerFixture(t, time.Second)
	callNative(t, m, "list", nil)
	s, err := m.slot("huabei.ssh-tunnels")
	if err != nil {
		t.Fatal(err)
	}
	s.op.Lock()
	client := s.client
	s.lease.Close()
	s.op.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for !client.Exited() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !client.Exited() {
		t.Fatal("worker survived loss of parent lease")
	}
}
func TestNativeCrashRequiresExplicitRetry(t *testing.T) {
	m, r, starts := nativeManagerFixture(t, 50*time.Millisecond)
	callNative(t, m, "list", nil)
	s, _ := m.slot("huabei.ssh-tunnels")
	s.op.Lock()
	if err := s.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	s.op.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for r.Available("huabei.ssh-tunnels") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.Call(context.Background(), "huabei.ssh-tunnels", "runtime.status", nil); err == nil {
		t.Fatal("crashed plugin silently restarted")
	}
	if starts.Load() != 1 {
		t.Fatal("crash loop")
	}
}
func TestNativeBinaryTamperingRejected(t *testing.T) {
	r, err := NewRegistry(RegistryOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	p := nativeTestPackage(t)
	if err = r.Install(p, r.Revision()); err != nil {
		t.Fatal(err)
	}
	path, err := r.Executable(p.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Executable(p.Manifest.ID); err == nil {
		t.Fatal("executed modified native binary")
	}
}

func TestSecondNativePluginNeedsNoHostBusinessCode(t *testing.T) {
	binary := filepath.Join(t.TempDir(), filepath.Base(nativeEntry(runtime.GOOS+"-"+runtime.GOARCH)))
	cmd := exec.Command("go", "build", "-o", binary, "./testdata/native-tool")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	bytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	p := nativeTestPackage(t)
	p.Binary = bytes
	p.Manifest.ID = "example.inspector"
	p.Manifest.Name = "Connection inspector"
	sum := sha256.Sum256(bytes)
	p.Manifest.Backend.SHA256 = fmt.Sprintf("%x", sum)
	p.Manifest.Contributes.Commands = []Command{{ID: "tool.inspect", Title: "Inspect connections"}}
	p.Manifest.Contributes.Views[0].ID = "tool.workspace"
	registry, err := NewRegistry(RegistryOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err = registry.Install(p, registry.Revision()); err != nil {
		t.Fatal(err)
	}
	m := NewNativeManager(NativeOptions{Registry: registry, Host: func(ctx context.Context, _ string, _ Manifest) (NativeHost, error) {
		return newFakeNativeHost(ctx), nil
	}})
	defer m.Close()
	raw, err := m.Call(context.Background(), p.Manifest.ID, "tool.inspect", nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Extension string
		Hosts     []any
	}
	if err = json.Unmarshal(raw, &result); err != nil || result.Extension != "connection-inspector" || len(result.Hosts) != 0 {
		t.Fatalf("second extension = %s, %v", raw, err)
	}
}

func TestWorkspacePauseBlocksNewPluginAcquisitions(t *testing.T) {
	m, _, starts := nativeManagerFixture(t, time.Second)
	callNative(t, m, "list", nil)
	resume := m.Pause()
	defer resume()
	result := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), "huabei.ssh-tunnels", "tunnels.request", sdk.JSON(map[string]string{"op": "list"}))
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("started during workspace restore: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if starts.Load() != 1 {
		t.Fatal("new backend started while paused")
	}
	resume()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not resume after restore")
	}
	if starts.Load() != 2 {
		t.Fatal("old backend retained after restore")
	}
}
