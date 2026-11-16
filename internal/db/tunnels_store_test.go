package db

import (
	"database/sql"
	"path/filepath"
	"ssh-connect/internal/domain"
	"strings"
	"testing"
)

func TestTunnelsPersistAndProtectReferencedHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.CreateHost(domain.HostInput{Alias: "fixture", HostName: "localhost", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	input := domain.TunnelInput{Name: "Database", HostID: host.ID, LocalPort: 13306, TargetAddress: "db.internal", TargetPort: 3306}
	tunnel, err := store.CreateTunnel(input)
	if err != nil {
		t.Fatal(err)
	}
	if tunnel.LocalAddress != "127.0.0.1" {
		t.Fatal("missing safe default")
	}
	original, err := store.listHostSortOrders()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteHosts([]string{host.ID}); err == nil || !strings.Contains(err.Error(), "tunnels associated") {
		t.Fatalf("delete referenced host = %v", err)
	}
	after, err := store.listHostSortOrders()
	if err != nil || len(after) != len(original) {
		t.Fatal("failed host deletion changed sort orders", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err := store.GetTunnel(tunnel.ID)
	if err != nil || saved != tunnel {
		t.Fatalf("reopened tunnel = %+v, %v", saved, err)
	}
	input.LocalAddress = "::1"
	input.TargetAddress = "::1"
	updated, err := store.UpdateTunnel(tunnel.ID, input)
	if err != nil || updated.LocalAddress != "::1" {
		t.Fatalf("IPv6 update = %+v, %v", updated, err)
	}
	input.HostID = "missing"
	if _, err := store.UpdateTunnel(tunnel.ID, input); err == nil {
		t.Fatal("accepted missing host")
	}
	input.HostID = host.ID
	input.TargetPort = 0
	if _, err := store.CreateTunnel(input); err == nil {
		t.Fatal("accepted invalid port")
	}
	if err := store.DeleteTunnel(tunnel.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTunnelSnapshotRoundTripAndAtomicValidation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	host, err := store.CreateHost(domain.HostInput{Alias: "fixture", HostName: "localhost", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.CreateTunnel(domain.TunnelInput{Name: "Fixture", HostID: host.ID, LocalPort: 13306, TargetAddress: "db.internal", TargetPort: 3306})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	if _, err := store.ExportHostConfigSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTunnel(config.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportHostConfigSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	saved, err := store.GetTunnel(config.ID)
	if err != nil || saved != config {
		t.Fatalf("restored tunnel = %+v, %v", saved, err)
	}
	incoming, err := sql.Open("sqlite", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := incoming.Exec(`UPDATE tunnels SET target_address = 'https://invalid'`); err != nil {
		t.Fatal(err)
	}
	incoming.Close()
	if _, err := store.ImportHostConfigSnapshot(snapshot); err == nil {
		t.Fatal("accepted malformed tunnel")
	}
	if saved, err = store.GetTunnel(config.ID); err != nil || saved != config {
		t.Fatal("failed restore mutated live tunnel")
	}
}
