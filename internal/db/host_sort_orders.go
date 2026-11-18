package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"ssh-connect/internal/domain"
)

func (s *Store) attachHostSortOrders(hosts []domain.Host) error {
	ids := make([]string, 0, len(hosts))
	for _, host := range hosts {
		ids = append(ids, host.ID)
	}
	sortOrders, err := s.hostSortOrdersByHostID(ids)
	if err != nil {
		return err
	}
	for i := range hosts {
		hosts[i].SortOrders = sortOrders[hosts[i].ID]
	}
	return nil
}

func (s *Store) hostSortOrdersByHostID(ids []string) (map[string]map[string]int, error) {
	result := make(map[string]map[string]int, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
		result[id] = make(map[string]int)
	}
	rows, err := s.db.Query(fmt.Sprintf(`
SELECT scope_type, scope_id, host_id, sort_order
FROM host_sort_orders
WHERE host_id IN (%s);
`, placeholders(len(ids))), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var scopeType string
		var scopeID string
		var hostID string
		var sortOrder int
		if err := rows.Scan(&scopeType, &scopeID, &hostID, &sortOrder); err != nil {
			return nil, err
		}
		if _, ok := result[hostID]; !ok {
			result[hostID] = make(map[string]int)
		}
		result[hostID][hostSortScopeKey(scopeType, scopeID)] = sortOrder
	}
	return result, rows.Err()
}

func nextHostSortOrderInScope(queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}, scopeType string, scopeID string) (int, error) {
	var order int
	err := queryer.QueryRow(`SELECT COALESCE(MAX(sort_order) + 1, 0) FROM host_sort_orders WHERE scope_type = ? AND scope_id = ?;`, scopeType, scopeID).Scan(&order)
	return order, err
}

func hostDirectoryIDsByHostID(queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, ids []string) (map[string]string, error) {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := queryer.Query(fmt.Sprintf(`SELECT id, directory_id FROM hosts WHERE id IN (%s);`, placeholders(len(ids))), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make(map[string]bool, len(ids))
	directoryIDs := make(map[string]string, len(ids))
	for rows.Next() {
		var id string
		var directoryID string
		if err := rows.Scan(&id, &directoryID); err != nil {
			return nil, err
		}
		found[id] = true
		directoryIDs[id] = directoryID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !found[id] {
			return nil, domain.ErrHostNotFound
		}
	}
	return directoryIDs, nil
}

func ensureHostsExist(queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := queryer.Query(fmt.Sprintf(`SELECT id FROM hosts WHERE id IN (%s);`, placeholders(len(ids))), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := make(map[string]bool, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if !found[id] {
			return domain.ErrHostNotFound
		}
	}
	return nil
}

func orderedHostIDsInScope(queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, scopeType string, scopeID string) ([]string, error) {
	rows, err := queryer.Query(`
SELECT id
FROM host_sort_orders
JOIN hosts ON hosts.id = host_sort_orders.host_id
WHERE scope_type = ? AND scope_id = ?
ORDER BY host_sort_orders.sort_order ASC, hosts.name ASC;
`, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func normalizeHostSortScope(scopeType string, scopeID string) (string, string) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	switch scopeType {
	case domain.HostSortScopeAll, domain.HostSortScopeFavorites, domain.HostSortScopeUncategorized:
		return scopeType, ""
	case domain.HostSortScopeDirectory:
		return scopeType, scopeID
	default:
		return domain.HostSortScopeAll, ""
	}
}

func hostSortScopeKey(scopeType string, scopeID string) string {
	scopeType, scopeID = normalizeHostSortScope(scopeType, scopeID)
	return scopeType + ":" + scopeID
}

func defaultHostScopeType(host domain.Host) string {
	return defaultScopeTypeForDirectoryID(host.DirectoryID)
}

func defaultHostScopeID(host domain.Host) string {
	return defaultScopeIDForDirectoryID(host.DirectoryID)
}

func defaultScopeTypeForDirectoryID(directoryID string) string {
	if strings.TrimSpace(directoryID) == "" {
		return domain.HostSortScopeUncategorized
	}
	return domain.HostSortScopeDirectory
}

func defaultScopeIDForDirectoryID(directoryID string) string {
	return strings.TrimSpace(directoryID)
}

func deriveHostSortOrders(hosts []domain.Host) []domain.HostSortOrder {
	sortOrders := make([]domain.HostSortOrder, 0, len(hosts)*2)
	allHosts := append([]domain.Host(nil), hosts...)
	favoriteOrder := 0
	for order, host := range allHosts {
		sortOrders = append(sortOrders, domain.HostSortOrder{
			ScopeType: domain.HostSortScopeAll,
			ScopeID:   "",
			HostID:    host.ID,
			SortOrder: order,
			CreatedAt: host.CreatedAt,
			UpdatedAt: host.UpdatedAt,
		})
		if host.Favorite {
			sortOrders = append(sortOrders, domain.HostSortOrder{
				ScopeType: domain.HostSortScopeFavorites,
				ScopeID:   "",
				HostID:    host.ID,
				SortOrder: favoriteOrder,
				CreatedAt: host.CreatedAt,
				UpdatedAt: host.UpdatedAt,
			})
			favoriteOrder++
		}
	}
	byScope := make(map[string][]domain.Host)
	for _, host := range hosts {
		scopeType, scopeID := defaultHostScopeType(host), defaultHostScopeID(host)
		key := hostSortScopeKey(scopeType, scopeID)
		byScope[key] = append(byScope[key], host)
	}
	for key, scopedHosts := range byScope {
		scopeType, scopeID := splitHostSortScopeKey(key)
		for order, host := range scopedHosts {
			sortOrders = append(sortOrders, domain.HostSortOrder{
				ScopeType: scopeType,
				ScopeID:   scopeID,
				HostID:    host.ID,
				SortOrder: order,
				CreatedAt: host.CreatedAt,
				UpdatedAt: host.UpdatedAt,
			})
		}
	}
	return sortOrders
}

func splitHostSortScopeKey(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 1 {
		return normalizeHostSortScope(parts[0], "")
	}
	return normalizeHostSortScope(parts[0], parts[1])
}

func sortHostsByScope(hosts []domain.Host, scopeType string, scopeID string) {
	key := hostSortScopeKey(scopeType, scopeID)
	sort.SliceStable(hosts, func(i, j int) bool {
		iOrder, iOK := hosts[i].SortOrders[key]
		jOrder, jOK := hosts[j].SortOrders[key]
		if iOK && jOK && iOrder != jOrder {
			return iOrder < jOrder
		}
		if iOK != jOK {
			return iOK
		}
		return hosts[i].DisplayName() < hosts[j].DisplayName()
	})
}

func seedHostSortOrdersForHost(exec sqlReadWriter, host domain.Host) error {
	now := host.CreatedAt
	if now == "" {
		now = host.UpdatedAt
	}
	if err := upsertHostSortOrderAtEnd(exec, defaultHostScopeType(host), defaultHostScopeID(host), host.ID, now); err != nil {
		return err
	}
	allOrder, err := nextHostSortOrderInScope(exec, domain.HostSortScopeAll, "")
	if err != nil {
		return err
	}
	if err := upsertHostSortOrder(exec, domain.HostSortScopeAll, "", host.ID, allOrder, now); err != nil {
		return err
	}
	if host.Favorite {
		if err := upsertHostSortOrderAtEnd(exec, domain.HostSortScopeFavorites, "", host.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func upsertHostSortOrderAtEnd(exec sqlReadWriter, scopeType string, scopeID string, hostID string, updatedAt string) error {
	scopeType, scopeID = normalizeHostSortScope(scopeType, scopeID)
	sortOrder, err := nextHostSortOrderInScope(exec, scopeType, scopeID)
	if err != nil {
		return err
	}
	return upsertHostSortOrder(exec, scopeType, scopeID, hostID, sortOrder, updatedAt)
}

func upsertHostSortOrder(exec sqlExecer, scopeType string, scopeID string, hostID string, sortOrder int, updatedAt string) error {
	scopeType, scopeID = normalizeHostSortScope(scopeType, scopeID)
	_, err := exec.Exec(`
INSERT INTO host_sort_orders (scope_type, scope_id, host_id, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(scope_type, scope_id, host_id) DO UPDATE SET
  sort_order = excluded.sort_order,
  updated_at = excluded.updated_at;
`, scopeType, scopeID, hostID, sortOrder, updatedAt, updatedAt)
	return err
}

func deleteHostSortOrder(exec sqlExecer, scopeType string, scopeID string, hostID string) error {
	scopeType, scopeID = normalizeHostSortScope(scopeType, scopeID)
	_, err := exec.Exec(`DELETE FROM host_sort_orders WHERE scope_type = ? AND scope_id = ? AND host_id = ?;`, scopeType, scopeID, hostID)
	return err
}
