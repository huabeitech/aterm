package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"ssh-connect/internal/domain"
	"testing"
)

func TestPluginDataSnapshotAndReferences(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	host, err := s.CreateHost(domain.HostInput{Name: "fixture", Alias: "fixture", HostName: "localhost", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	record := PluginData{PluginID: "example.tool", Key: "configs", Value: json.RawMessage(`{"version":1,"settings":["opaque"]}`), HostIDs: []string{host.ID}}
	if err = s.SetPluginData(record); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteHost(host.ID); err == nil {
		t.Fatal("deleted referenced host")
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	if _, err = s.ExportHostConfigSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	record.Value = json.RawMessage(`[]`)
	record.HostIDs = nil
	if err = s.SetPluginData(record); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportHostConfigSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	value, err := s.GetPluginData("example.tool", "configs")
	if err != nil || string(value) != `{"version":1,"settings":["opaque"]}` {
		t.Fatalf("restored %s: %v", value, err)
	}
	raw, err := sql.Open("sqlite", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`UPDATE plugin_data SET host_ids='["missing"]'`)
	raw.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportHostConfigSnapshot(snapshot); err == nil {
		t.Fatal("accepted invalid references")
	}
	after, _ := s.GetPluginData("example.tool", "configs")
	if string(after) != string(value) {
		t.Fatal("invalid restore changed live data")
	}
}
func TestLegacyTunnelsMigrateOnceAndOldBackupRestores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations {
		if m.version >= 8 {
			break
		}
		tx, err := raw.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err = m.up(tx); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	_, err = raw.Exec(`INSERT INTO hosts(id,alias,host_name,port,created_at,updated_at) VALUES('host','fixture','localhost',22,'2026-11-17','2026-11-17'); INSERT INTO tunnels(id,name,host_id,local_address,local_port,target_address,target_port,created_at,updated_at) VALUES('saved','database','host','127.0.0.1',13306,'db.internal',3306,'2026-11-17','2026-11-17')`)
	if err != nil {
		t.Fatal(err)
	}
	raw.Close()
	// This legacy snapshot has no migration ledger and must remain readable.
	legacy, err := readHostConfigSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.pluginData) != 1 {
		t.Fatal("old snapshot lost configurations")
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE migrations(version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP); INSERT INTO migrations(version) VALUES(1),(2),(3),(4),(5),(6),(7)`)
	raw.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	value, err := s.GetPluginData("huabei.ssh-tunnels", "configs")
	if err != nil {
		t.Fatal(err)
	}
	var configs []domain.Tunnel
	if err = json.Unmarshal(value, &configs); err != nil || len(configs) != 1 || configs[0].ID != "saved" {
		t.Fatalf("migrated %s: %v", value, err)
	}
	exists, err := tableExists(s.db, "tunnels")
	if err != nil || exists {
		t.Fatal("legacy runtime table retained", err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	again, _ := s.GetPluginData("huabei.ssh-tunnels", "configs")
	if string(again) != string(value) {
		t.Fatal("migration ran twice")
	}
}
