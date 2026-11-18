package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"ssh-connect/internal/domain"
	"time"
)

// PluginData is opaque to the host. References provide generic host deletion
// protection without teaching the database any plugin's configuration schema.
type PluginData = domain.PluginData

const pluginDataSchema = `CREATE TABLE plugin_data(plugin_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, host_ids TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(plugin_id,key));
CREATE TRIGGER plugin_host_delete BEFORE DELETE ON hosts WHEN EXISTS(SELECT 1 FROM plugin_data p, json_each(p.host_ids) r WHERE r.value=OLD.id) BEGIN SELECT RAISE(ABORT,'the host is referenced by plugin configurations'); END;`

var pluginDataID = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9.-]{0,98}$`)
var pluginDataKey = regexp.MustCompile(`^[a-zA-Z0-9.-]{1,100}$`)

func validatePluginData(p PluginData) error {
	if !pluginDataID.MatchString(p.PluginID) || !pluginDataKey.MatchString(p.Key) || !json.Valid(p.Value) || len(p.Value) > 256*1024 || len(p.HostIDs) > 1024 {
		return errors.New("invalid plugin data")
	}
	return nil
}
func (s *Store) GetPluginData(id, key string) (json.RawMessage, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM plugin_data WHERE plugin_id=? AND key=?`, id, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return json.RawMessage("null"), nil
	}
	return json.RawMessage(v), err
}
func (s *Store) SetPluginData(p PluginData) error {
	p.UpdatedAt = formatTime(time.Now())
	if err := validatePluginData(p); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range p.HostIDs {
		var count int
		if err = tx.QueryRow(`SELECT count(*) FROM hosts WHERE id=?`, id).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("the SSH host no longer exists")
		}
	}
	if err = insertPluginData(tx, []PluginData{p}); err != nil {
		return err
	}
	return tx.Commit()
}
func insertPluginData(tx sqlExecer, rows []PluginData) error {
	for _, p := range rows {
		if err := validatePluginData(p); err != nil {
			return err
		}
		refs := p.HostIDs
		if refs == nil {
			refs = []string{}
		}
		b, _ := json.Marshal(refs)
		if _, err := tx.Exec(`INSERT INTO plugin_data(plugin_id,key,value,host_ids,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(plugin_id,key) DO UPDATE SET value=excluded.value,host_ids=excluded.host_ids,updated_at=excluded.updated_at`, p.PluginID, p.Key, string(p.Value), string(b), p.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}
func readPluginData(raw *sql.DB) ([]PluginData, error) {
	rows, err := raw.Query(`SELECT plugin_id,key,value,host_ids,updated_at FROM plugin_data ORDER BY plugin_id,key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginData{}
	for rows.Next() {
		var p PluginData
		var value, refs string
		if err = rows.Scan(&p.PluginID, &p.Key, &value, &refs, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Value = json.RawMessage(value)
		if err = json.Unmarshal([]byte(refs), &p.HostIDs); err != nil {
			return nil, err
		}
		if err = validatePluginData(p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
