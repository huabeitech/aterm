package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"regexp"
	desktopthemes "ssh-connect/internal/desktop/themes"
	"strings"
)

func (a *App) desktopThemes(raw []json.RawMessage) (any, error) {
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	a.desktopMu.Lock()
	store := a.desktopThemesStore
	a.desktopMu.Unlock()
	if store == nil {
		return nil, errors.New("theme storage is not initialized")
	}
	if op == "list" {
		return store.List()
	}
	if op == "import" {
		paths, err := a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Import Theme Package", Message: "Choose a theme package JSON or manifest.json"})
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			return nil, nil
		}
		if len(paths) != 1 {
			return nil, errors.New("choose one theme package file")
		}
		incoming, err := desktopthemes.Inspect(paths[0])
		if err != nil {
			return nil, err
		}
		catalog, err := store.List()
		if err != nil {
			return nil, err
		}
		for _, existing := range catalog.Packages {
			if existing.ID == incoming.ID && !a.confirmThemeUpdate(existing, incoming) {
				return nil, nil
			}
		}
		result, err := store.InstallPackage(incoming)
		if err == nil {
			a.emitEvent("themes:changed", nil)
		}
		return result, err
	}
	reference, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	switch op {
	case "duplicate":
		result, err := store.Duplicate(reference)
		if err == nil {
			a.emitEvent("themes:changed", nil)
		}
		return result, err
	case "remove":
		result, err := store.Remove(reference)
		if err == nil {
			err = a.resetRemovedThemePreferences(reference)
		}
		if err == nil {
			a.emitEvent("themes:changed", nil)
		}
		return result, err
	case "export":
		// Resolve before showing a panel, including attempts to export built-in IDs.
		catalog, err := store.List()
		if err != nil {
			return nil, err
		}
		found := false
		for _, pkg := range catalog.Packages {
			if pkg.ID == reference {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("unknown theme package")
		}
		safeName := regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(reference, "_") + ".json"
		path, err := a.saveFileDialog(application.SaveFileDialogOptions{Title: "Export Theme Package", Filename: safeName, CanCreateDirectories: true})
		if err != nil || path == "" {
			return nil, err
		}
		return nil, store.Export(reference, path)
	default:
		return nil, fmt.Errorf("unknown theme operation %q", op)
	}
}

func (a *App) confirmThemeUpdate(previous, incoming desktopthemes.Package) bool {
	return a.confirmAction(a.ctx, windowConfirmation{
		Title:        "Update Theme",
		Message:      fmt.Sprintf("%s: %s → %s", previous.Name, previous.Version, incoming.Version),
		ConfirmLabel: "Update Theme",
	})
}
func (a *App) resetRemovedThemePreferences(reference string) error {
	preferences, err := a.desktopPreferences()
	if err != nil {
		return err
	}
	changed := false
	for _, key := range []string{"theme", "lightTheme", "darkTheme"} {
		value, _ := preferences[key].(string)
		if value == reference || (!strings.Contains(reference, "/") && strings.HasPrefix(value, reference+"/")) {
			fallback := "dark"
			if key == "theme" {
				fallback = "system"
			}
			if key == "lightTheme" {
				fallback = "light"
			}
			preferences[key] = fallback
			changed = true
		}
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(preferences)
	if err != nil {
		return err
	}
	return a.store.SetAppMetadata(desktopPreferencesKey, string(encoded))
}
