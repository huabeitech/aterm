package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ssh-connect/internal/db"
	"ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/service"
	sdk "ssh-connect/packages/plugin-go"
)

// Opt-in acceptance uses scripts/parity-fixture's loopback jump host. It never
// reads a personal host library, system credentials or a production server.
func TestNativeTunnelPluginThroughSavedJumpHost(t *testing.T) {
	profile := os.Getenv("ATERM_PARITY_FIXTURE_PROFILE")
	archive := os.Getenv("ATERM_NATIVE_PLUGIN_PACKAGE")
	if profile == "" || archive == "" {
		t.Skip("set disposable parity profile and native package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := db.Open(filepath.Join(profile, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := service.NewAppService(service.AppServiceOptions{Store: store, SSHDir: filepath.Join(profile, "ssh"), HostKeyStorePath: filepath.Join(profile, "ssh-host-keys.json"), PasswordStore: nativeFixtureSecrets{}, LocalVaultPath: filepath.Join(profile, "vault.enc")})
	app := &App{ctx: ctx, store: store, svc: svc}
	registry, err := plugins.NewRegistry(plugins.RegistryOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	p, err := plugins.InspectArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = registry.Install(p, registry.Revision()); err != nil {
		t.Fatal(err)
	}
	m := plugins.NewNativeManager(plugins.NativeOptions{Registry: registry, Host: func(ctx context.Context, id string, manifest plugins.Manifest) (plugins.NativeHost, error) {
		raw, err := app.newNativePluginHost(ctx, id, manifest)
		if err != nil {
			return nil, err
		}
		h := raw.(*nativePluginHost)
		hosts, err := svc.ListHosts(ctx)
		if err != nil {
			return nil, err
		}
		for _, host := range hosts {
			h.grants[plugins.GrantID(pluginHostTarget(host), manifest)] = true
		}
		return h, nil
	}})
	defer m.Close()
	call := func(op string, args map[string]any) json.RawMessage {
		t.Helper()
		if args == nil {
			args = map[string]any{}
		}
		args["op"] = op
		out, err := m.Call(ctx, p.Manifest.ID, "tunnels.request", sdk.JSON(args))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	var saved []struct {
		ID, HostID, LocalAddress, TargetAddress string
		LocalPort, TargetPort                   int
		State, Error                            string
	}
	if err = json.Unmarshal(call("list", nil), &saved); err != nil || len(saved) != 1 {
		t.Fatalf("fixture configs: %s / %v", call("list", nil), err)
	}
	config := saved[0]
	call("start", map[string]any{"id": config.ID})
	for {
		var rows []struct{ State, Error string }
		_ = json.Unmarshal(call("list", nil), &rows)
		if rows[0].State == "running" {
			break
		}
		if rows[0].State == "failed" {
			t.Fatal(rows[0].Error)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	address := net.JoinHostPort(config.LocalAddress, fmt.Sprint(config.LocalPort))
	client := http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + address)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !strings.Contains(string(body), "aTerm tunnel fixture reached through SSH") {
		t.Fatalf("forwarding %q / %v", body, err)
	}
	var result struct{ LocalListener string }
	input := map[string]any{"name": "draft", "hostID": config.HostID, "localAddress": config.LocalAddress, "localPort": config.LocalPort, "targetAddress": config.TargetAddress, "targetPort": config.TargetPort}
	if err = json.Unmarshal(call("test", map[string]any{"id": config.ID, "input": input}), &result); err != nil || result.LocalListener != "existing" {
		t.Fatalf("test running listener: %+v / %v", result, err)
	}
	call("stop", map[string]any{"id": config.ID})
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal("plugin leaked local port", err)
	}
	listener.Close()
}

type nativeFixtureSecrets struct{}

func (nativeFixtureSecrets) Get(string) (string, error) { return "", service.ErrPasswordNotFound }
func (nativeFixtureSecrets) Set(string, string) error   { return nil }
func (nativeFixtureSecrets) Delete(string) error        { return nil }
