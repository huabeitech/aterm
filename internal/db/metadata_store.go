package db

import (
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

func (s *Store) GetAppMetadata(key string) (string, error) {
	var value string
	if err := s.db.QueryRow(`SELECT value FROM app_metadata WHERE key = ?`, key).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (s *Store) SetAppMetadata(key, value string) error {
	_, err := s.db.Exec(`
INSERT INTO app_metadata (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = excluded.updated_at;
`, key, value, formatTime(time.Now()))
	return err
}

// RecoverAppMetadata archives an invalid value and installs its replacement in
// one transaction. Compare-and-swap prevents recovery from overwriting a newer
// save from another window/process. A failed archive rolls back the replacement.
func (s *Store) RecoverAppMetadata(key, expected, replacement string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := formatTime(time.Now())
	result, err := tx.Exec(`UPDATE app_metadata SET value = ?, updated_at = ? WHERE key = ? AND value = ?`, replacement, now, key, expected)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	archive := key + ".recovery-" + uuid.NewString()
	if _, err := tx.Exec(`INSERT INTO app_metadata (key,value,updated_at) VALUES (?,?,?)`, archive, expected, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
