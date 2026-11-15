package db

import "database/sql"

func migration_version_2(tx *sql.Tx) error {
	for _, column := range []struct{ name, definition string }{
		{"move_tab_left_shortcut", "TEXT NOT NULL DEFAULT 'meta+shift+arrowleft'"},
		{"move_tab_right_shortcut", "TEXT NOT NULL DEFAULT 'meta+shift+arrowright'"},
		{"terminal_font_size", "INTEGER NOT NULL DEFAULT 13"},
		{"split_vertical_shortcut", "TEXT NOT NULL DEFAULT 'meta+d'"},
		{"split_horizontal_shortcut", "TEXT NOT NULL DEFAULT 'meta+shift+d'"},
	} {
		if err := ensureColumn(tx, "app_settings", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}
