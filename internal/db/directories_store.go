package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"ssh-connect/internal/domain"
)

func (s *Store) ListDirectories() ([]domain.Directory, error) {
	rows, err := s.db.Query(`
SELECT id, parent_id, name, sort_order, created_at, updated_at
FROM directories
ORDER BY parent_id ASC, sort_order ASC, created_at ASC, name ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	directories := make([]domain.Directory, 0)
	for rows.Next() {
		directory, err := scanDirectory(rows)
		if err != nil {
			return nil, err
		}
		directories = append(directories, directory)
	}
	return directories, rows.Err()
}

func (s *Store) CreateDirectory(input domain.DirectoryInput) (domain.Directory, error) {
	input = domain.NormalizeDirectoryInput(input)
	if err := domain.ValidateDirectoryInput(input); err != nil {
		return domain.Directory{}, err
	}

	now := formatTime(time.Now())
	sortOrder, err := s.nextDirectorySortOrder(input.ParentID)
	if err != nil {
		return domain.Directory{}, err
	}
	directory := domain.Directory{
		ID:        uuid.NewString(),
		ParentID:  input.ParentID,
		Name:      input.Name,
		SortOrder: sortOrder,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if directory.ParentID != "" {
		if _, err := s.GetDirectory(directory.ParentID); err != nil {
			return domain.Directory{}, err
		}
	}
	_, err = s.db.Exec(`
INSERT INTO directories (id, parent_id, name, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);
`, directory.ID, directory.ParentID, directory.Name, directory.SortOrder, directory.CreatedAt, directory.UpdatedAt)
	if err != nil {
		if isUniqueDirectoryNameError(err) {
			return domain.Directory{}, ErrDuplicateDirectoryName
		}
		return domain.Directory{}, err
	}
	return directory, nil
}

func (s *Store) UpdateDirectory(id string, input domain.DirectoryInput) (domain.Directory, error) {
	input = domain.NormalizeDirectoryInput(input)
	if err := domain.ValidateDirectoryInput(input); err != nil {
		return domain.Directory{}, err
	}

	existing, err := s.GetDirectory(id)
	if err != nil {
		return domain.Directory{}, err
	}

	directory := existing
	directory.Name = input.Name
	directory.UpdatedAt = formatTime(time.Now())
	_, err = s.db.Exec(`
UPDATE directories
SET name = ?, updated_at = ?
WHERE id = ?;
`, directory.Name, directory.UpdatedAt, directory.ID)
	if err != nil {
		if isUniqueDirectoryNameError(err) {
			return domain.Directory{}, ErrDuplicateDirectoryName
		}
		return domain.Directory{}, err
	}
	return directory, nil
}

func (s *Store) GetDirectory(id string) (domain.Directory, error) {
	row := s.db.QueryRow(`
SELECT id, parent_id, name, sort_order, created_at, updated_at
FROM directories
WHERE id = ?;
`, id)
	return scanDirectory(row)
}

func (s *Store) MoveDirectory(id string, input domain.DirectoryMoveInput) (domain.Directory, error) {
	input.ParentID = strings.TrimSpace(input.ParentID)
	if input.SortOrder < 0 {
		input.SortOrder = 0
	}
	existing, err := s.GetDirectory(id)
	if err != nil {
		return domain.Directory{}, err
	}
	if input.ParentID != "" {
		if _, err := s.GetDirectory(input.ParentID); err != nil {
			return domain.Directory{}, err
		}
		if err := s.ensureDirectoryMoveAllowed(id, input.ParentID); err != nil {
			return domain.Directory{}, err
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return domain.Directory{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := formatTime(time.Now())
	if existing.ParentID == input.ParentID {
		if err = reorderDirectorySiblings(tx, id, input.ParentID, input.SortOrder, now); err != nil {
			return domain.Directory{}, err
		}
	} else {
		if _, err = tx.Exec(`UPDATE directories SET parent_id = ?, sort_order = -1, updated_at = ? WHERE id = ?;`, input.ParentID, now, id); err != nil {
			if isUniqueDirectoryNameError(err) {
				return domain.Directory{}, ErrDuplicateDirectoryName
			}
			return domain.Directory{}, err
		}
		if err = compactDirectorySiblings(tx, existing.ParentID); err != nil {
			return domain.Directory{}, err
		}
		if err = reorderDirectorySiblings(tx, id, input.ParentID, input.SortOrder, now); err != nil {
			return domain.Directory{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.Directory{}, err
	}
	return s.GetDirectory(id)
}

func (s *Store) DeleteDirectory(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	descendantIDs, err := directoryDescendantIDs(tx, id)
	if err != nil {
		return err
	}
	affectedHostIDs, err := hostIDsInDirectories(tx, descendantIDs)
	if err != nil {
		return err
	}
	now := formatTime(time.Now())
	if _, err = tx.Exec(`
WITH RECURSIVE descendants(id) AS (
  SELECT id FROM directories WHERE id = ?
  UNION ALL
  SELECT directories.id FROM directories JOIN descendants ON directories.parent_id = descendants.id
)
UPDATE hosts SET directory_id = '' WHERE directory_id IN (SELECT id FROM descendants);
`, id); err != nil {
		return err
	}
	if len(descendantIDs) > 0 {
		args := make([]any, 0, len(descendantIDs)+1)
		args = append(args, domain.HostSortScopeDirectory)
		for _, directoryID := range descendantIDs {
			args = append(args, directoryID)
		}
		if _, err = tx.Exec(fmt.Sprintf(`DELETE FROM host_sort_orders WHERE scope_type = ? AND scope_id IN (%s);`, placeholders(len(descendantIDs))), args...); err != nil {
			return err
		}
	}
	nextOrder, err := nextHostSortOrderInScope(tx, domain.HostSortScopeUncategorized, "")
	if err != nil {
		return err
	}
	for index, hostID := range affectedHostIDs {
		if err = upsertHostSortOrder(tx, domain.HostSortScopeUncategorized, "", hostID, nextOrder+index, now); err != nil {
			return err
		}
	}
	var parentID string
	_ = tx.QueryRow(`SELECT parent_id FROM directories WHERE id = ?;`, id).Scan(&parentID)
	if _, err = tx.Exec(`
WITH RECURSIVE descendants(id) AS (
  SELECT id FROM directories WHERE id = ?
  UNION ALL
  SELECT directories.id FROM directories JOIN descendants ON directories.parent_id = descendants.id
)
DELETE FROM directories WHERE id IN (SELECT id FROM descendants);
`, id); err != nil {
		return err
	}
	if err = compactDirectorySiblings(tx, parentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) nextDirectorySortOrder(parentID string) (int, error) {
	var order int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(sort_order) + 1, 0) FROM directories WHERE parent_id = ?;`, parentID).Scan(&order)
	return order, err
}

func (s *Store) ensureDirectoryMoveAllowed(id string, parentID string) error {
	if id == parentID {
		return domain.ErrDirectoryParentCycle
	}
	rows, err := s.db.Query(`
WITH RECURSIVE descendants(id) AS (
  SELECT id FROM directories WHERE parent_id = ?
  UNION ALL
  SELECT directories.id FROM directories JOIN descendants ON directories.parent_id = descendants.id
)
SELECT id FROM descendants;
`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var descendantID string
		if err := rows.Scan(&descendantID); err != nil {
			return err
		}
		if descendantID == parentID {
			return domain.ErrDirectoryParentCycle
		}
	}
	return rows.Err()
}

func directoryDescendantIDs(queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, id string) ([]string, error) {
	rows, err := queryer.Query(`
WITH RECURSIVE descendants(id) AS (
  SELECT id FROM directories WHERE id = ?
  UNION ALL
  SELECT directories.id FROM directories JOIN descendants ON directories.parent_id = descendants.id
)
SELECT id FROM descendants;
`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var descendantID string
		if err := rows.Scan(&descendantID); err != nil {
			return nil, err
		}
		ids = append(ids, descendantID)
	}
	return ids, rows.Err()
}

func hostIDsInDirectories(queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, directoryIDs []string) ([]string, error) {
	if len(directoryIDs) == 0 {
		return []string{}, nil
	}
	args := make([]any, 0, len(directoryIDs))
	for _, directoryID := range directoryIDs {
		args = append(args, directoryID)
	}
	rows, err := queryer.Query(fmt.Sprintf(`
SELECT id
FROM hosts
WHERE directory_id IN (%s)
ORDER BY directory_id ASC, created_at ASC, alias ASC;
`, placeholders(len(directoryIDs))), args...)
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

func reorderDirectorySiblings(tx *sql.Tx, movingID string, parentID string, requestedOrder int, updatedAt string) error {
	rows, err := tx.Query(`
SELECT id
FROM directories
WHERE parent_id = ? AND id != ?
ORDER BY sort_order ASC, created_at ASC, name ASC;
`, parentID, movingID)
	if err != nil {
		return err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if requestedOrder > len(ids) {
		requestedOrder = len(ids)
	}
	ids = append(ids, "")
	copy(ids[requestedOrder+1:], ids[requestedOrder:])
	ids[requestedOrder] = movingID
	for order, id := range ids {
		if _, err := tx.Exec(`UPDATE directories SET sort_order = ?, updated_at = ? WHERE id = ?;`, order, updatedAt, id); err != nil {
			return err
		}
	}
	return nil
}

func compactDirectorySiblings(tx *sql.Tx, parentID string) error {
	rows, err := tx.Query(`
SELECT id
FROM directories
WHERE parent_id = ?
ORDER BY sort_order ASC, created_at ASC, name ASC;
`, parentID)
	if err != nil {
		return err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := formatTime(time.Now())
	for order, id := range ids {
		if _, err := tx.Exec(`UPDATE directories SET sort_order = ?, updated_at = ? WHERE id = ?;`, order, now, id); err != nil {
			return err
		}
	}
	return nil
}
