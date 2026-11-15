package db

import (
	"fmt"
	"strings"
	"time"
)

func normalizeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	next := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		next = append(next, id)
	}
	return next
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isUniqueAliasError(err error) bool {
	return strings.Contains(fmt.Sprint(err), "UNIQUE constraint failed: hosts.alias")
}

func isUniqueDirectoryNameError(err error) bool {
	text := fmt.Sprint(err)
	return strings.Contains(text, "UNIQUE constraint failed: directories.name") ||
		strings.Contains(text, "UNIQUE constraint failed: directories.parent_id, directories.name") ||
		strings.Contains(text, "idx_directories_parent_name")
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
