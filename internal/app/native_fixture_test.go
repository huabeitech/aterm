package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"ssh-connect/internal/terminal"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"ssh-connect/internal/db"
	"ssh-connect/internal/service"
)

// Opt-in acceptance against scripts/parity-fixture. Ordinary unit tests never
// read personal hosts or make SSH connections.
func TestNativeParitySFTPFixture(t *testing.T) {
	profile := os.Getenv("ATERM_PARITY_FIXTURE_PROFILE")
	if profile == "" {
		t.Skip("start the disposable parity fixture to enable native SSH acceptance")
	}
	store, err := db.Open(filepath.Join(profile, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc := service.NewAppService(service.AppServiceOptions{Store: store, SSHDir: filepath.Join(profile, "ssh"), DatabasePath: filepath.Join(profile, "app.db"), ConfigPath: filepath.Join(profile, "ssh", "config"), GeneratedConfigPath: filepath.Join(profile, "ssh", "generated.conf")})
	client, _, err := svc.OpenNativeSSH(ctx, "parity-sftp")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	files, err := sftp.NewClient(client.RawClient())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	states := make(chan terminal.OutputEvent, 32)
	manager := terminal.NewManager(ctx, func(_ string, data any) { states <- data.(terminal.OutputEvent) })
	defer manager.CloseAll()
	entry, err := manager.OpenSSHAsync("parity-sftp", func(connectCtx context.Context, confirmer service.HostKeyConfirmer) (*service.NativeSSHClient, string, error) {
		connection, host, err := svc.OpenNativeSSHWithHostKeyConfirmer(connectCtx, "parity-sftp", confirmer)
		return connection, host.Alias, err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = entry
	select {
	case event := <-states:
		if event.State != "running" {
			t.Fatalf("terminal event %#v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	shell, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Close()
	if err := shell.RequestPty("xterm-256color", 24, 80, nil); err != nil {
		t.Fatal(err)
	}
	if err := shell.Shell(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ATERM_EXPECT_NATIVE_EDIT") == "1" {
		file, err := files.Open("/config.txt")
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(io.LimitReader(file, 1024*1024))
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "native_edit=verified") || !strings.Contains(string(content), "unsaved=must_survive") {
			t.Fatal("native editor changes were not saved on the SFTP server")
		}
	}
	entries, err := files.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatal("fixture files were not listed")
	}
}
