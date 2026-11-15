package domain

import (
	"fmt"
	"strings"
)

const (
	ThemeSystem                     = "system"
	ThemeLight                      = "light"
	ThemeDark                       = "dark"
	TerminalThemeDefaultDark        = "default-dark"
	TerminalThemeDefaultLight       = "default-light"
	TerminalThemeSolarizedDark      = "solarized-dark"
	TerminalThemeSolarizedLight     = "solarized-light"
	TerminalThemeGruvboxDark        = "gruvbox-dark"
	TerminalThemeDracula            = "dracula"
	DefaultHomeHosts                = "hosts"
	DefaultHomeLocalTerminal        = "local-terminal"
	DefaultCommandPaletteShortcut   = "meta+k"
	DefaultNewTerminalShortcut      = "meta+t"
	DefaultCloseTerminalShortcut    = "meta+w"
	DefaultPreviousTerminalShortcut = "meta+arrowleft"
	DefaultNextTerminalShortcut     = "meta+arrowright"
	DefaultMoveTabLeftShortcut      = "meta+shift+arrowleft"
	DefaultMoveTabRightShortcut     = "meta+shift+arrowright"
	DefaultTerminalFontSize         = 13
	DefaultSplitVerticalShortcut    = "meta+d"
	DefaultSplitHorizontalShortcut  = "meta+shift+d"
)

func DefaultAppSettings() AppSettings {
	return AppSettings{
		Theme:                    ThemeSystem,
		TerminalTheme:            TerminalThemeDefaultDark,
		DefaultHome:              DefaultHomeHosts,
		CommandPaletteShortcut:   DefaultCommandPaletteShortcut,
		NewTerminalShortcut:      DefaultNewTerminalShortcut,
		CloseTerminalShortcut:    DefaultCloseTerminalShortcut,
		PreviousTerminalShortcut: DefaultPreviousTerminalShortcut,
		NextTerminalShortcut:     DefaultNextTerminalShortcut,
		MoveTabLeftShortcut:      DefaultMoveTabLeftShortcut,
		MoveTabRightShortcut:     DefaultMoveTabRightShortcut,
		TerminalFontSize:         DefaultTerminalFontSize,
		TerminalWordNavigation:   "shell",
		SplitVerticalShortcut:    DefaultSplitVerticalShortcut,
		SplitHorizontalShortcut:  DefaultSplitHorizontalShortcut,
	}
}

func NormalizeAppSettingsInput(input AppSettingsInput) (AppSettings, error) {
	settings := AppSettings{
		Theme:                  strings.ToLower(strings.TrimSpace(input.Theme)),
		TerminalTheme:          strings.ToLower(strings.TrimSpace(input.TerminalTheme)),
		DefaultHome:            strings.ToLower(strings.TrimSpace(input.DefaultHome)),
		CommandPaletteShortcut: strings.TrimSpace(input.CommandPaletteShortcut), NewTerminalShortcut: strings.TrimSpace(input.NewTerminalShortcut),
		CloseTerminalShortcut: strings.TrimSpace(input.CloseTerminalShortcut), PreviousTerminalShortcut: strings.TrimSpace(input.PreviousTerminalShortcut),
		NextTerminalShortcut: strings.TrimSpace(input.NextTerminalShortcut), SSHDirectoryPath: strings.TrimSpace(input.SSHDirectoryPath),
		MoveTabLeftShortcut: strings.TrimSpace(input.MoveTabLeftShortcut), MoveTabRightShortcut: strings.TrimSpace(input.MoveTabRightShortcut),
		TerminalFontSize:       input.TerminalFontSize,
		TerminalWordNavigation: input.TerminalWordNavigation,
		SplitVerticalShortcut:  strings.TrimSpace(input.SplitVerticalShortcut), SplitHorizontalShortcut: strings.TrimSpace(input.SplitHorizontalShortcut),
		SSHDirectoryBookmark: strings.TrimSpace(input.SSHDirectoryBookmark), SSHDirectoryBookmarkCreatedAt: strings.TrimSpace(input.SSHDirectoryBookmarkCreatedAt),
		SyncDirectoryPath: strings.TrimSpace(input.SyncDirectoryPath), SyncDirectoryBookmark: strings.TrimSpace(input.SyncDirectoryBookmark),
		SyncDirectoryBookmarkCreatedAt: strings.TrimSpace(input.SyncDirectoryBookmarkCreatedAt),
	}
	if settings.Theme == "" {
		settings.Theme = ThemeSystem
	}
	if settings.Theme != ThemeSystem && settings.Theme != ThemeLight && settings.Theme != ThemeDark {
		return AppSettings{}, fmt.Errorf("invalid theme: %s", input.Theme)
	}
	if settings.TerminalTheme == "" {
		settings.TerminalTheme = TerminalThemeDefaultDark
	}
	if !isTerminalTheme(settings.TerminalTheme) {
		return AppSettings{}, fmt.Errorf("invalid terminal theme: %s", input.TerminalTheme)
	}
	if settings.DefaultHome == "" {
		settings.DefaultHome = DefaultHomeHosts
	}
	if settings.DefaultHome != DefaultHomeHosts && settings.DefaultHome != DefaultHomeLocalTerminal {
		return AppSettings{}, fmt.Errorf("invalid default home: %s", input.DefaultHome)
	}
	shortcut, err := NormalizeCommandPaletteShortcut(settings.CommandPaletteShortcut)
	if err != nil {
		return AppSettings{}, err
	}
	settings.CommandPaletteShortcut = shortcut
	shortcuts := []struct {
		value    *string
		fallback string
	}{
		{&settings.NewTerminalShortcut, DefaultNewTerminalShortcut}, {&settings.CloseTerminalShortcut, DefaultCloseTerminalShortcut},
		{&settings.PreviousTerminalShortcut, DefaultPreviousTerminalShortcut}, {&settings.NextTerminalShortcut, DefaultNextTerminalShortcut},
		{&settings.MoveTabLeftShortcut, DefaultMoveTabLeftShortcut}, {&settings.MoveTabRightShortcut, DefaultMoveTabRightShortcut},
		{&settings.SplitVerticalShortcut, DefaultSplitVerticalShortcut}, {&settings.SplitHorizontalShortcut, DefaultSplitHorizontalShortcut},
	}
	for _, item := range shortcuts {
		if *item.value == "" {
			*item.value = item.fallback
		}
		normalized, err := NormalizeCommandPaletteShortcut(*item.value)
		if err != nil {
			return AppSettings{}, err
		}
		*item.value = normalized
	}
	if settings.TerminalWordNavigation == "" {
		settings.TerminalWordNavigation = "shell"
	}
	if settings.TerminalWordNavigation != "shell" && settings.TerminalWordNavigation != "terminal" {
		return AppSettings{}, fmt.Errorf("invalid terminal word navigation")
	}
	seen := map[string]bool{}
	if settings.TerminalFontSize == 0 {
		settings.TerminalFontSize = DefaultTerminalFontSize
	}
	if settings.TerminalFontSize < 10 || settings.TerminalFontSize > 24 {
		return AppSettings{}, fmt.Errorf("terminal font size must be between 10 and 24")
	}
	for _, value := range []string{settings.CommandPaletteShortcut, settings.NewTerminalShortcut, settings.CloseTerminalShortcut, settings.PreviousTerminalShortcut, settings.NextTerminalShortcut, settings.MoveTabLeftShortcut, settings.MoveTabRightShortcut, settings.SplitVerticalShortcut, settings.SplitHorizontalShortcut} {
		if seen[value] {
			return AppSettings{}, fmt.Errorf("keyboard shortcuts must be unique")
		}
		seen[value] = true
	}
	return settings, nil
}

func isTerminalTheme(value string) bool {
	switch value {
	case TerminalThemeDefaultDark, TerminalThemeDefaultLight, TerminalThemeSolarizedDark, TerminalThemeSolarizedLight, TerminalThemeGruvboxDark, TerminalThemeDracula:
		return true
	default:
		return false
	}
}

func NormalizeCommandPaletteShortcut(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultCommandPaletteShortcut, nil
	}
	parts, modifiers, key := strings.Split(value, "+"), map[string]bool{}, ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "meta", "ctrl", "alt", "shift":
			if modifiers[part] {
				return "", fmt.Errorf("duplicate command palette shortcut modifier: %s", part)
			}
			modifiers[part] = true
		default:
			if key != "" || !isShortcutKey(part) {
				return "", fmt.Errorf("invalid shortcut: %s", value)
			}
			key = part
		}
	}
	if key == "" || (!modifiers["meta"] && !modifiers["ctrl"]) {
		return "", fmt.Errorf("shortcut must include Command or Control")
	}
	normalized := make([]string, 0, 5)
	for _, modifier := range []string{"meta", "ctrl", "alt", "shift"} {
		if modifiers[modifier] {
			normalized = append(normalized, modifier)
		}
	}
	return strings.Join(append(normalized, key), "+"), nil
}

func isShortcutKey(key string) bool {
	if key == "arrowleft" || key == "arrowright" {
		return true
	}
	if len(key) == 1 && ((key[0] >= 'a' && key[0] <= 'z') || (key[0] >= '0' && key[0] <= '9')) {
		return true
	}
	if len(key) >= 2 && key[0] == 'f' {
		for _, char := range key[1:] {
			if char < '0' || char > '9' {
				return false
			}
		}
		return true
	}
	return false
}
