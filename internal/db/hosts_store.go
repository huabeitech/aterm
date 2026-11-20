package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"ssh-connect/internal/domain"
)

func (s *Store) ListHosts() ([]domain.Host, error) {
	rows, err := s.db.Query(`
SELECT id, directory_id, name, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
       compression, server_alive_interval, server_alive_count_max, connect_timeout,
       remark, last_used_at, created_at, updated_at
FROM hosts
ORDER BY directory_id ASC, created_at ASC, alias ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := make([]domain.Host, 0)
	for rows.Next() {
		host, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachHostSortOrders(hosts); err != nil {
		return nil, err
	}
	return hosts, nil
}

func (s *Store) CountHosts() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM hosts;`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CreateHost(input domain.HostInput) (domain.Host, error) {
	input = domain.NormalizeHostInput(input)
	if err := domain.ValidateHostInput(input); err != nil {
		return domain.Host{}, err
	}

	now := formatTime(time.Now())
	host := hostFromInput(uuid.NewString(), input, now, now)
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Host{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.Exec(`
INSERT INTO hosts (
  id, directory_id, name, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
  compression, server_alive_interval, server_alive_count_max, connect_timeout,
  remark, last_used_at, created_at, updated_at
) VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`,
		host.ID, host.DirectoryID, host.Name, host.Alias, host.HostName, host.User, host.Port, host.IdentityFile, boolToInt(host.PasswordAuth), host.ProxyJump, host.JumpHostID,
		boolToInt(host.ForwardAgent), boolToInt(host.Compression), host.ServerAliveInterval,
		host.ServerAliveCountMax, host.ConnectTimeout, host.Remark,
		host.LastUsedAt, host.CreatedAt, host.UpdatedAt,
	)
	if err != nil {
		if isUniqueAliasError(err) {
			return domain.Host{}, ErrDuplicateAlias
		}
		return domain.Host{}, err
	}
	if err = seedHostSortOrdersForHost(tx, host); err != nil {
		return domain.Host{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Host{}, err
	}
	sortOrders, err := s.hostSortOrdersByHostID([]string{host.ID})
	if err != nil {
		return domain.Host{}, err
	}
	host.SortOrders = sortOrders[host.ID]
	return host, nil
}

func (s *Store) UpdateHost(id string, input domain.HostInput) (domain.Host, error) {
	input = domain.NormalizeHostInput(input)
	if err := domain.ValidateHostInput(input); err != nil {
		return domain.Host{}, err
	}

	existing, err := s.GetHost(id)
	if err != nil {
		return domain.Host{}, err
	}

	now := formatTime(time.Now())
	host := hostFromInput(id, input, existing.CreatedAt, now)
	host.LastUsedAt = existing.LastUsedAt
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Host{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.Exec(`
UPDATE hosts
SET directory_id = ?, name = ?, alias = NULLIF(?, ''), host_name = ?, user = ?, port = ?, identity_file = ?, password_auth = ?, proxy_jump = ?, jump_host_id = ?,
    forward_agent = ?, compression = ?, server_alive_interval = ?, server_alive_count_max = ?,
    connect_timeout = ?, remark = ?, updated_at = ?
WHERE id = ?;
`,
		host.DirectoryID, host.Name, host.Alias, host.HostName, host.User, host.Port, host.IdentityFile, boolToInt(host.PasswordAuth), host.ProxyJump, host.JumpHostID,
		boolToInt(host.ForwardAgent), boolToInt(host.Compression), host.ServerAliveInterval,
		host.ServerAliveCountMax, host.ConnectTimeout, host.Remark,
		host.UpdatedAt, host.ID,
	)
	if err != nil {
		if isUniqueAliasError(err) {
			return domain.Host{}, ErrDuplicateAlias
		}
		return domain.Host{}, err
	}
	if input.DirectoryID != existing.DirectoryID {
		if err = deleteHostSortOrder(tx, defaultHostScopeType(existing), defaultHostScopeID(existing), id); err != nil {
			return domain.Host{}, err
		}
		if err = upsertHostSortOrderAtEnd(tx, defaultHostScopeType(host), defaultHostScopeID(host), host.ID, now); err != nil {
			return domain.Host{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.Host{}, err
	}
	sortOrders, err := s.hostSortOrdersByHostID([]string{host.ID})
	if err != nil {
		return domain.Host{}, err
	}
	host.SortOrders = sortOrders[host.ID]
	return host, nil
}

func (s *Store) MoveHosts(ids []string, directoryID string) ([]domain.Host, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return []domain.Host{}, nil
	}
	if directoryID != "" {
		if _, err := s.GetDirectory(directoryID); err != nil {
			return nil, err
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := formatTime(time.Now())
	sourceByHostID, err := hostDirectoryIDsByHostID(tx, ids)
	if err != nil {
		return nil, err
	}
	nextOrder, err := nextHostSortOrderInScope(tx, defaultScopeTypeForDirectoryID(directoryID), defaultScopeIDForDirectoryID(directoryID))
	if err != nil {
		return nil, err
	}
	for index, id := range ids {
		if _, err = tx.Exec(`UPDATE hosts SET directory_id = ?, updated_at = ? WHERE id = ?;`, directoryID, now, id); err != nil {
			return nil, err
		}
		sourceHost := domain.Host{ID: id, DirectoryID: sourceByHostID[id]}
		if err = deleteHostSortOrder(tx, defaultHostScopeType(sourceHost), defaultHostScopeID(sourceHost), id); err != nil {
			return nil, err
		}
		if err = upsertHostSortOrder(tx, defaultScopeTypeForDirectoryID(directoryID), defaultScopeIDForDirectoryID(directoryID), id, nextOrder+index, now); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return s.listHostsByIDs(ids)
}

func (s *Store) ReorderHosts(scopeType string, scopeID string, ids []string) ([]domain.Host, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return []domain.Host{}, nil
	}
	scopeType, scopeID = normalizeHostSortScope(scopeType, scopeID)
	if scopeType == domain.HostSortScopeDirectory {
		if _, err := s.GetDirectory(scopeID); err != nil {
			return nil, err
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err := ensureHostsExist(tx, ids); err != nil {
		return nil, err
	}
	orderedIDs, err := orderedHostIDsInScope(tx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	requestedIDSet := make(map[string]bool, len(ids))
	nextIDs := make([]string, 0, len(orderedIDs))
	for _, id := range ids {
		if requestedIDSet[id] {
			continue
		}
		requestedIDSet[id] = true
		nextIDs = append(nextIDs, id)
	}
	for _, id := range orderedIDs {
		if !requestedIDSet[id] {
			nextIDs = append(nextIDs, id)
		}
	}

	now := formatTime(time.Now())
	for order, id := range nextIDs {
		if _, err = tx.Exec(`
INSERT INTO host_sort_orders (scope_type, scope_id, host_id, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(scope_type, scope_id, host_id) DO UPDATE SET
  sort_order = excluded.sort_order,
  updated_at = excluded.updated_at;
`, scopeType, scopeID, id, order, now, now); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	hosts, err := s.listHostsByIDs(nextIDs)
	if err != nil {
		return nil, err
	}
	sortHostsByScope(hosts, scopeType, scopeID)
	return hosts, nil
}

func (s *Store) GetHost(id string) (domain.Host, error) {
	row := s.db.QueryRow(`
SELECT id, directory_id, name, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
       compression, server_alive_interval, server_alive_count_max, connect_timeout,
       remark, last_used_at, created_at, updated_at
FROM hosts
WHERE id = ?;
`, id)
	host, err := scanHost(row)
	if err != nil {
		return domain.Host{}, err
	}
	sortOrders, err := s.hostSortOrdersByHostID([]string{host.ID})
	if err != nil {
		return domain.Host{}, err
	}
	host.SortOrders = sortOrders[host.ID]
	return host, nil
}

func (s *Store) MarkHostUsed(id string, usedAt string) (domain.Host, error) {
	_, err := s.db.Exec(`UPDATE hosts SET last_used_at = ?, updated_at = ? WHERE id = ?;`, usedAt, usedAt, id)
	if err != nil {
		return domain.Host{}, err
	}
	return s.GetHost(id)
}

func (s *Store) DeleteHost(id string) error {
	return s.DeleteHosts([]string{id})
}

func (s *Store) DeleteHosts(ids []string) error {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM host_sort_orders WHERE host_id IN (%s); DELETE FROM hosts WHERE id IN (%s);`, placeholders(len(ids)), placeholders(len(ids))), append(args, args...)...)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func hostFromInput(id string, input domain.HostInput, createdAt, updatedAt string) domain.Host {
	return domain.Host{
		ID:                  id,
		DirectoryID:         input.DirectoryID,
		Name:                input.Name,
		Alias:               input.Alias,
		HostName:            input.HostName,
		User:                input.User,
		Port:                input.Port,
		IdentityFile:        input.IdentityFile,
		PasswordAuth:        input.PasswordAuth,
		ProxyJump:           input.ProxyJump,
		JumpHostID:          input.JumpHostID,
		ForwardAgent:        input.ForwardAgent,
		Compression:         input.Compression,
		ServerAliveInterval: input.ServerAliveInterval,
		ServerAliveCountMax: input.ServerAliveCountMax,
		ConnectTimeout:      input.ConnectTimeout,
		Remark:              input.Remark,
		LastUsedAt:          "",
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}
}

func (s *Store) listHostsByIDs(ids []string) ([]domain.Host, error) {
	if len(ids) == 0 {
		return []domain.Host{}, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT id, directory_id, name, alias, host_name, user, port, identity_file, password_auth, proxy_jump, jump_host_id, forward_agent,
       compression, server_alive_interval, server_alive_count_max, connect_timeout,
       remark, last_used_at, created_at, updated_at
FROM hosts
WHERE id IN (%s)
ORDER BY directory_id ASC, created_at ASC, alias ASC;
`, placeholders(len(ids))), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := make([]domain.Host, 0)
	for rows.Next() {
		host, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachHostSortOrders(hosts); err != nil {
		return nil, err
	}
	return hosts, nil
}
