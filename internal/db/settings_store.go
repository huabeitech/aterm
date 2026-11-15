package db

import (
	"database/sql"
	"errors"
	"time"

	"ssh-connect/internal/domain"
)

func (s *Store) GetSettings() (domain.AppSettings, error) {
	row := s.db.QueryRow(`
	SELECT terminal_word_navigation, theme, terminal_theme, default_home, command_palette_shortcut, new_terminal_shortcut, close_terminal_shortcut, previous_terminal_shortcut, next_terminal_shortcut, move_tab_left_shortcut, move_tab_right_shortcut, terminal_font_size, split_vertical_shortcut, split_horizontal_shortcut,
	       ssh_directory_path, ssh_directory_bookmark, ssh_directory_bookmark_created_at,
	       sync_directory_path, sync_directory_bookmark, sync_directory_bookmark_created_at
	FROM app_settings
	WHERE id = 1;
	`)
	settings := domain.AppSettings{}
	if err := row.Scan(
		&settings.TerminalWordNavigation,
		&settings.Theme,
		&settings.TerminalTheme,
		&settings.DefaultHome,
		&settings.CommandPaletteShortcut,
		&settings.NewTerminalShortcut,
		&settings.CloseTerminalShortcut,
		&settings.PreviousTerminalShortcut,
		&settings.NextTerminalShortcut,
		&settings.MoveTabLeftShortcut,
		&settings.MoveTabRightShortcut,
		&settings.TerminalFontSize,
		&settings.SplitVerticalShortcut,
		&settings.SplitHorizontalShortcut,
		&settings.SSHDirectoryPath,
		&settings.SSHDirectoryBookmark,
		&settings.SSHDirectoryBookmarkCreatedAt,
		&settings.SyncDirectoryPath,
		&settings.SyncDirectoryBookmark,
		&settings.SyncDirectoryBookmarkCreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DefaultAppSettings(), nil
		}
		return domain.AppSettings{}, err
	}
	return settings, nil
}

func (s *Store) UpdateSettings(input domain.AppSettingsInput) (domain.AppSettings, error) {
	settings, err := domain.NormalizeAppSettingsInput(input)
	if err != nil {
		return domain.AppSettings{}, err
	}
	_, err = s.db.Exec(`
	INSERT INTO app_settings (
	  id, terminal_word_navigation, theme, terminal_theme, default_home, command_palette_shortcut, new_terminal_shortcut, close_terminal_shortcut, previous_terminal_shortcut, next_terminal_shortcut, move_tab_left_shortcut, move_tab_right_shortcut, terminal_font_size, split_vertical_shortcut, split_horizontal_shortcut,
	  ssh_directory_path, ssh_directory_bookmark, ssh_directory_bookmark_created_at, sync_directory_path, sync_directory_bookmark, sync_directory_bookmark_created_at, updated_at
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
	  terminal_word_navigation = excluded.terminal_word_navigation,
	  theme = excluded.theme,
	  terminal_theme = excluded.terminal_theme,
	  default_home = excluded.default_home,
	  command_palette_shortcut = excluded.command_palette_shortcut,
	  new_terminal_shortcut = excluded.new_terminal_shortcut,
	  close_terminal_shortcut = excluded.close_terminal_shortcut,
	  previous_terminal_shortcut = excluded.previous_terminal_shortcut,
	  next_terminal_shortcut = excluded.next_terminal_shortcut,
	  move_tab_left_shortcut = excluded.move_tab_left_shortcut,
	  move_tab_right_shortcut = excluded.move_tab_right_shortcut,
	  terminal_font_size = excluded.terminal_font_size,
	  split_vertical_shortcut = excluded.split_vertical_shortcut,
	  split_horizontal_shortcut = excluded.split_horizontal_shortcut,
	  ssh_directory_path = excluded.ssh_directory_path,
	  ssh_directory_bookmark = excluded.ssh_directory_bookmark,
	  ssh_directory_bookmark_created_at = excluded.ssh_directory_bookmark_created_at,
	  sync_directory_path = excluded.sync_directory_path,
	  sync_directory_bookmark = excluded.sync_directory_bookmark,
	  sync_directory_bookmark_created_at = excluded.sync_directory_bookmark_created_at,
	  updated_at = excluded.updated_at;
	`,
		settings.TerminalWordNavigation,
		settings.Theme,
		settings.TerminalTheme,
		settings.DefaultHome,
		settings.CommandPaletteShortcut,
		settings.NewTerminalShortcut,
		settings.CloseTerminalShortcut,
		settings.PreviousTerminalShortcut,
		settings.NextTerminalShortcut,
		settings.MoveTabLeftShortcut,
		settings.MoveTabRightShortcut,
		settings.TerminalFontSize,
		settings.SplitVerticalShortcut,
		settings.SplitHorizontalShortcut,
		settings.SSHDirectoryPath,
		settings.SSHDirectoryBookmark,
		settings.SSHDirectoryBookmarkCreatedAt,
		settings.SyncDirectoryPath,
		settings.SyncDirectoryBookmark,
		settings.SyncDirectoryBookmarkCreatedAt,
		formatTime(time.Now()),
	)
	if err != nil {
		return domain.AppSettings{}, err
	}
	return settings, nil
}
