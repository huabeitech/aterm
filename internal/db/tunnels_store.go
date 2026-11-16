package db

import (
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"ssh-connect/internal/domain"
	"time"
)

const tunnelColumns = `id, name, host_id, local_address, local_port, target_address, target_port, created_at, updated_at`

func scanTunnel(row hostScanner) (domain.Tunnel, error) {
	var t domain.Tunnel
	err := row.Scan(&t.ID, &t.Name, &t.HostID, &t.LocalAddress, &t.LocalPort, &t.TargetAddress, &t.TargetPort, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = domain.ErrTunnelNotFound
	}
	return t, err
}

func (s *Store) GetTunnel(id string) (domain.Tunnel, error) {
	return scanTunnel(s.db.QueryRow(`SELECT `+tunnelColumns+` FROM tunnels WHERE id = ?`, id))
}

func (s *Store) ListTunnels() ([]domain.Tunnel, error) { return readTunnels(s.db) }

func readTunnels(raw *sql.DB) ([]domain.Tunnel, error) {
	rows, err := raw.Query(`SELECT ` + tunnelColumns + ` FROM tunnels ORDER BY created_at, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Tunnel{}
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		if err := domain.ValidateTunnelInput(t.TunnelInput); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func insertTunnels(exec sqlExecer, tunnels []domain.Tunnel) error {
	for _, t := range tunnels {
		if err := domain.ValidateTunnelInput(t.TunnelInput); err != nil {
			return err
		}
		if _, err := exec.Exec(`INSERT INTO tunnels (`+tunnelColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.Name, t.HostID, t.LocalAddress, t.LocalPort, t.TargetAddress, t.TargetPort, t.CreatedAt, t.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateTunnel(input domain.TunnelInput) (domain.Tunnel, error) {
	input = domain.NormalizeTunnelInput(input)
	if err := domain.ValidateTunnelInput(input); err != nil {
		return domain.Tunnel{}, err
	}
	now := formatTime(time.Now())
	t := domain.Tunnel{TunnelInput: input, ID: uuid.NewString(), CreatedAt: now, UpdatedAt: now}
	return t, insertTunnels(s.db, []domain.Tunnel{t})
}

func (s *Store) UpdateTunnel(id string, input domain.TunnelInput) (domain.Tunnel, error) {
	input = domain.NormalizeTunnelInput(input)
	if err := domain.ValidateTunnelInput(input); err != nil {
		return domain.Tunnel{}, err
	}
	result, err := s.db.Exec(`UPDATE tunnels SET name=?, host_id=?, local_address=?, local_port=?, target_address=?, target_port=?, updated_at=? WHERE id=?`, input.Name, input.HostID, input.LocalAddress, input.LocalPort, input.TargetAddress, input.TargetPort, formatTime(time.Now()), id)
	if err != nil {
		return domain.Tunnel{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return domain.Tunnel{}, err
	}
	if count == 0 {
		return domain.Tunnel{}, domain.ErrTunnelNotFound
	}
	return s.GetTunnel(id)
}

func (s *Store) DeleteTunnel(id string) error {
	result, err := s.db.Exec(`DELETE FROM tunnels WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return domain.ErrTunnelNotFound
	}
	return err
}
