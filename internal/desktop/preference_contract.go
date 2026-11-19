package desktop

import (
	"errors"
	"regexp"
	"ssh-connect/internal/domain"
	"strings"
)

var externalThemeID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,95}/[a-z0-9][a-z0-9-]{0,63}$`)
var functionKey = regexp.MustCompile(`^f([1-9]|1[0-9]|2[0-4])$`)

// This is the shared renderer ID contract, not a filesystem path validator.
// An external theme need not be installed while importing saved preferences.
func isDesktopThemeID(value string) bool {
	switch value {
	case "system", "dark", "light", "light-dark-terminal", "solarized-light", "solarized-dark", "gruvbox-dark", "dracula":
		return true
	default:
		return externalThemeID.MatchString(value)
	}
}

// The legacy domain normalizer is shared with the original configuration model.
// Apply the current renderer's stricter key/reservation rules at this boundary.
func normalizeDesktopShortcut(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("invalid shortcut")
	}
	normalized, err := domain.NormalizeCommandPaletteShortcut(value)
	if err != nil {
		return "", err
	}
	parts := strings.Split(normalized, "+")
	key := parts[len(parts)-1]
	if strings.HasPrefix(key, "f") && len(key) > 1 && !functionKey.MatchString(key) {
		return "", errors.New("unsupported function key")
	}
	switch normalized {
	case "meta+q", "ctrl+q", "meta+shift+h", "ctrl+shift+h":
		return "", errors.New("this shortcut is reserved by the application")
	}
	return normalized, nil
}
