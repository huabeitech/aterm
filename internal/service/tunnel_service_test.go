package service

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ssh-connect/internal/db"
	"strings"
	"testing"
	"time"

	"ssh-connect/internal/domain"
	"ssh-connect/internal/tunnel"
)

func TestTunnelEncryptedBackupRestoresConfigWithoutRuntime(t *testing.T) {
	source, _, store := managedBackupService(t)
	ctx := context.Background()
	if _, err := source.SetUpSecureData(ctx, "fixture-password"); err != nil {
		t.Fatal(err)
	}
	host, err := store.CreateHost(HostInput{Alias: "fixture", HostName: "localhost", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	config, err := source.SaveTunnel(ctx, "", domain.TunnelInput{Name: "Fixture database", HostID: host.ID, LocalPort: 13306, TargetAddress: "db.internal", TargetPort: 3306})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "vault.enc")
	if _, err := source.ExportEncryptedBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	destination, _, _ := managedBackupService(t)
	// A running listener must be stopped even when incoming host/tunnel IDs differ.
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := local.Addr().(*net.TCPAddr).Port
	local.Close()
	active := domain.Tunnel{ID: "previous", TunnelInput: domain.TunnelInput{Name: "Previous tunnel", HostID: "previous-host", LocalAddress: "127.0.0.1", LocalPort: port, TargetAddress: "127.0.0.1", TargetPort: 3306}}
	entered := make(chan struct{})
	if err := destination.TunnelService.manager.Start(active, func(ctx context.Context) (tunnel.Transport, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(destination.CloseTunnels)
	<-entered
	if _, err := destination.RestoreEncryptedBackup(ctx, backup, "fixture-password", func(SyncMetadata) bool { return true }); err != nil {
		t.Fatal(err)
	}
	restored, err := destination.ListTunnels(ctx)
	if err != nil || len(restored) != 1 || restored[0].Tunnel != config {
		t.Fatalf("restored = %+v, %v", restored, err)
	}
	if restored[0].State != "stopped" || restored[0].ActiveConfig != nil {
		t.Fatal("backup restored runtime")
	}
	released, err := net.Listen("tcp", tunnel.Address(active.LocalAddress, active.LocalPort))
	if err != nil {
		t.Fatal("restore leaked old listener", err)
	}
	released.Close()
}

// Opt-in integration uses the disposable parity fixture, never personal servers.
func TestTunnelParityFixtureThroughJumpHost(t *testing.T) {
	profile := os.Getenv("ATERM_PARITY_FIXTURE_PROFILE")
	if profile == "" {
		t.Skip("start parity-fixture -tunnels to verify native forwarding through ProxyJump")
	}
	raw, err := os.ReadFile(filepath.Join(profile, "tunnel-fixture.json"))
	if os.IsNotExist(err) {
		t.Skip("fixture was started without -tunnels")
	}
	if err != nil {
		t.Fatal(err)
	}
	var config domain.Tunnel
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	svc, store := fixtureTunnelService(t, profile)
	defer store()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := svc.StartTunnel(ctx, config.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	defer svc.CloseTunnels()
	waitTunnelState(t, ctx, svc, "running")
	verifyFixtureHTTP(t, config)
	if err := svc.StopTunnel(ctx, config.ID); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", tunnel.Address(config.LocalAddress, config.LocalPort))
	if err != nil {
		t.Fatal("port not released", err)
	}
	listener.Close()
}

func fixtureTunnelService(t *testing.T, profile string) (*AppService, func()) {
	t.Helper()
	store, err := db.Open(filepath.Join(profile, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewAppService(AppServiceOptions{Store: store, PasswordStore: newMemoryPasswordStore(), SSHDir: filepath.Join(profile, "ssh"), DatabasePath: filepath.Join(profile, "app.db"), HostKeyStorePath: filepath.Join(profile, "ssh-host-keys.json")})
	return svc, func() { svc.CloseTunnels(); store.Close() }
}
func waitTunnelState(t *testing.T, ctx context.Context, svc *AppService, state string) {
	t.Helper()
	for {
		statuses, err := svc.ListTunnels(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(statuses) == 1 {
			if statuses[0].State == state {
				return
			}
			if statuses[0].State == "failed" {
				t.Fatalf("SSH tunnel failed: %s", statuses[0].Error)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
func verifyFixtureHTTP(t *testing.T, config domain.Tunnel) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + tunnel.Address(config.LocalAddress, config.LocalPort))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || !strings.Contains(string(body), "aTerm tunnel fixture reached through SSH") {
		t.Fatalf("body=%q, err=%v", body, err)
	}
}
