package db

import (
	"database/sql"
	"errors"
	"ssh-connect/internal/domain"
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
