package service

import (
	"fmt"
	"strings"
)

const DefaultCommandPaletteShortcut = "meta+k"

const (
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

// NormalizeCommandPaletteShortcut stores a command-palette shortcut in a
// stable modifier order so it can be shared by the persisted settings and UI.
func NormalizeCommandPaletteShortcut(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultCommandPaletteShortcut, nil
	}

	parts := strings.Split(value, "+")
	modifiers := map[string]bool{}
	key := ""
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
