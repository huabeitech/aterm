package openssh

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	desktopfiles "ssh-connect/internal/desktop/files"
)

func TestParseSSHConfig(t *testing.T) {
	for _, text := range []string{"hostname host\nuser fixture\nport 2222\n", "hostname ::1\nuser fixture\nport 22\n"} {
		if got := ParseConfig(text); got == nil || got.User != "fixture" {
			t.Fatal("valid config rejected", got)
		}
	}
	for _, text := range []string{"hostname host\nport 22", "hostname host\nuser user\nport 0", "hostname host\nuser user\nport 65536"} {
		if ParseConfig(text) != nil {
			t.Fatal("invalid config accepted")
		}
	}
}
func fixtureBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell/OpenSSH fixture")
	}
	file := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(file, []byte("#!/bin/sh\nexec "+quote(os.Args[0])+" -test.run='^TestOpenSSHProcessFixture$' -test.timeout=10s\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return file
}

type fixtureIO struct{}

func (fixtureIO) Read(data []byte) (int, error)  { return os.Stdin.Read(data) }
func (fixtureIO) Write(data []byte) (int, error) { return os.Stdout.Write(data) }
func (fixtureIO) Close() error                   { return nil }
func TestOpenSSHProcessFixture(t *testing.T) {
	switch os.Getenv("ATERM_OPENSSH_TEST_ROLE") {
	case "sftp":
		server := sftp.NewRequestServer(fixtureIO{}, sftp.InMemHandler())
		_ = server.Serve()
		server.Close()
		os.Exit(0)
	case "stall":
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
}
func TestMultiplexedSFTPLifetimeAndHandshakeCancellation(t *testing.T) {
	binary := fixtureBinary(t)
	t.Setenv("ATERM_OPENSSH_TEST_ROLE", "sftp")
	integration, err := New(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Close()
	integration.update(&RemoteContext{ID: "c.fixture", Host: "fixture", User: "user", Port: 22, Source: "openssh", State: "ready"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, owner, err := integration.OpenFiles(ctx, "c.fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	defer client.Close()
	manager := desktopfiles.NewManager()
	defer manager.CloseAll()
	info, err := manager.Add(client, owner, desktopfiles.SessionInfo{Title: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Mkdir("/verified"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Stat("/verified"); err != nil {
		t.Fatal(err)
	}
	integration.update(nil)
	result := make(chan error, 1)
	go func() { _, err := client.Getwd(); result <- err }()
	select {
	case err = <-result:
		if err == nil {
			t.Fatal("expired SSH context remained usable")
		}
	case <-ctx.Done():
		t.Fatal("expired file channel did not close")
	}
	if err = manager.Close(info.ID); err != nil {
		t.Fatal("expired workspace close reported a cleanup error", err)
	}
	if _, _, err = integration.OpenFiles(ctx, "c.fixture"); err == nil {
		t.Fatal("stale context was accepted")
	}
	t.Setenv("ATERM_OPENSSH_TEST_ROLE", "stall")
	integration.update(&RemoteContext{ID: "c.second", Host: "fixture", User: "user", Port: 22, Source: "openssh", State: "ready"})
	bounded, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	defer stop()
	if _, _, err = integration.OpenFiles(bounded, "c.second"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("SFTP handshake ignored cancellation", err)
	}
}
func TestDiscoveryRejectsAmbiguousAndCommandContexts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix control sockets")
	}
	binary := filepath.Join(t.TempDir(), "ssh")
	os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700)
	integration, err := New(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Close()
	add := func(id, config string) {
		directory := filepath.Join(integration.directory, "connections", id)
		os.Mkdir(directory, 0700)
		os.WriteFile(filepath.Join(directory, "config"), []byte(config), 0600)
		listener, err := net.Listen("unix", filepath.Join(directory, "socket"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { listener.Close() })
	}
	config := "hostname fixture\nuser user\nport 22\n"
	add("c.first", config)
	if got := integration.discover(); got == nil || got.ID != "c.first" {
		t.Fatal("active master was not discovered", got)
	}
	add("c.second", config)
	if got := integration.discover(); got != nil {
		t.Fatal("ambiguous SSH destinations selected", got)
	}
	os.WriteFile(filepath.Join(integration.directory, "connections", "c.second", "config"), []byte(config+"remotecommand uptime\n"), 0600)
	if got := integration.discover(); got == nil || got.ID != "c.first" {
		t.Fatal("command-only context became a file workspace", got)
	}
}
func TestZshStartupPreservesOriginalDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("zsh fixture")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("zsh unavailable")
	}
	integration, err := New("/usr/bin/ssh")
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Close()
	original := t.TempDir()
	os.WriteFile(filepath.Join(original, ".zshrc"), []byte("export ATERM_FIXTURE_STARTUP=loaded\n"), 0600)
	env, args, err := integration.ShellEnvironment("/bin/zsh", append(os.Environ(), "ZDOTDIR="+original))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/zsh", append(args, "-i", "-c", "print -r -- \"$ATERM_FIXTURE_STARTUP|$ZDOTDIR\"; functions ssh")...)
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh startup: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "loaded|"+original) || !strings.Contains(string(output), integration.directory+"/ssh") {
		t.Fatalf("original startup or integration lost: %s", output)
	}
}

func TestWrapperPreservesUnsupportedOpenSSHModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix wrapper")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "real-ssh")
	log := filepath.Join(root, "calls")
	script := "#!/bin/sh\n{ printf 'CALL\\n'; printf '%s\\n' \"$@\"; } >> " + quote(log) + "\nexit 0\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	integration, err := New(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Close()
	cases := [][]string{{"-G", "fixture"}, {"-N", "fixture"}, {"fixture", "uptime"}, {"-o", "ControlMaster=auto", "fixture"}, {"-S", "/tmp/socket", "fixture"}, {"--", "fixture"}, {"-W", "target:22", "jump"}}
	for _, args := range cases {
		os.Remove(log)
		if output, err := exec.Command(filepath.Join(integration.directory, "ssh"), args...).CombinedOutput(); err != nil {
			t.Fatalf("wrapper %v: %v: %s", args, err, output)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), "CALL\n"+strings.Join(args, "\n")+"\n"; got != want {
			t.Fatalf("unsupported mode arguments changed: %v: %q", args, got)
		}
		entries, _ := os.ReadDir(filepath.Join(integration.directory, "connections"))
		if len(entries) != 0 {
			t.Fatal("unsupported SSH mode acquired an integration directory")
		}
	}
}
