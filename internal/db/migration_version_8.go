package db

import (
	"database/sql"
	"encoding/json"
	"ssh-connect/internal/domain"
)

// This is the only schema-specific adapter. Runtime and new snapshots store
// opaque plugin data; old backups are translated through the same adapter.
func legacyTunnelData(configs []domain.Tunnel) []PluginData {
	if len(configs) == 0 {
		return nil
	}
	refs := []string{}
	seen := map[string]bool{}
	updated := ""
	for _, t := range configs {
		if !seen[t.HostID] {
			refs = append(refs, t.HostID)
			seen[t.HostID] = true
		}
		if t.UpdatedAt > updated {
			updated = t.UpdatedAt
		}
	}
	b, _ := json.Marshal(configs)
	return []PluginData{{PluginID: "huabei.ssh-tunnels", Key: "configs", Value: b, HostIDs: refs, UpdatedAt: updated}}
}
func migration_version_8(tx *sql.Tx) error {
	if _, err := tx.Exec(pluginDataSchema); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT ` + tunnelColumns + ` FROM tunnels`)
	if err != nil {
		return err
	}
	configs := []domain.Tunnel{}
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			rows.Close()
			return err
		}
		configs = append(configs, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if err = insertPluginData(tx, legacyTunnelData(configs)); err != nil {
		return err
	}
	_, err = tx.Exec(`DROP TRIGGER tunnels_host_delete; DROP TABLE tunnels;`)
	return err
}
