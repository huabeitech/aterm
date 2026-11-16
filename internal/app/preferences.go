package app

import (
	"encoding/json"
	"errors"
	"runtime"
	"ssh-connect/internal/desktop"
	"ssh-connect/internal/desktop/product"
	"ssh-connect/internal/service"
)

const desktopPreferencesKey = desktop.PreferencesMetadataKey

func (a *App) desktopInfo() map[string]any {
	info := product.Info()
	return map[string]any{"name": info.Name, "version": info.Version, "platform": desktop.RendererPlatform(runtime.GOOS), "macAppStoreBuild": service.IsMacAppStoreBuild(), "localTerminalAvailable": service.LocalTerminalAvailable()}
}
func (a *App) desktopPreferences() (desktop.Preferences, error) {
	if a.store == nil {
		return nil, errors.New("application storage is not initialized")
	}
	a.desktopPreferencesMu.Lock()
	defer a.desktopPreferencesMu.Unlock()
	for attempt := 0; attempt < 3; attempt++ {
		value, err := a.store.GetAppMetadata(desktopPreferencesKey)
		if err != nil {
			return nil, err
		}
		if value == "" {
			return desktop.DefaultPreferences(), nil
		}
		result := desktop.Preferences{}
		decodeErr := json.Unmarshal([]byte(value), &result)
		if decodeErr == nil && result != nil {
			validated, err := desktop.ValidatePreferences(result)
			if err == nil {
				return validated, nil
			}
		}
		// Only malformed/invalid preference content is recovered. Database errors
		// remain visible, and the exact original value is preserved transactionally.
		replacement := desktop.DefaultPreferences()
		encoded, err := json.Marshal(replacement)
		if err != nil {
			return nil, err
		}
		recovered, err := a.store.RecoverAppMetadata(desktopPreferencesKey, value, string(encoded))
		if err != nil {
			return nil, err
		}
		if recovered {
			a.desktopPreferencesRecovered.Store(true)
			return replacement, nil
		}
	}
	return nil, errors.New("preferences changed while recovering; please try again")
}
func (a *App) desktopBootstrap() (any, error) {
	revision := a.desktopLocaleRevision.Load()
	preferences, err := a.desktopPreferences()
	if err != nil {
		return nil, err
	}
	appearance := "light"
	if preferences["theme"] == "dark" {
		appearance = "dark"
	}
	var recoveryNotice any
	if a.desktopPreferencesRecovered.Load() {
		recoveryNotice = "Your saved preferences could not be loaded. Defaults were restored and the original settings were preserved."
	}
	return map[string]any{"info": a.desktopInfo(), "preferences": preferences, "locale": desktopLocale(preferences, revision), "resolvedTheme": appearance, "recoveryNotice": recoveryNotice}, nil
}
func (a *App) desktopSavePreferences(args []json.RawMessage) (any, error) {
	input, err := argument[desktop.Preferences](args, 0)
	if err != nil {
		return nil, err
	}
	preferences, err := desktop.ValidatePreferences(input)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(preferences)
	if err != nil {
		return nil, err
	}
	a.desktopPreferencesMu.Lock()
	err = a.store.SetAppMetadata(desktopPreferencesKey, string(encoded))
	var revision uint64
	if err == nil {
		revision = a.desktopLocaleRevision.Add(1)
	}
	a.desktopPreferencesMu.Unlock()
	if err != nil {
		return nil, err
	}
	if a.desktopPluginRuntime != nil {
		locale, _ := preferences["language"].(string)
		a.desktopPluginRuntime.SetLocale(locale)
	}
	a.emitEvent("locale:changed", desktopLocale(preferences, revision))
	a.installDesktopMenu(preferences)
	return preferences, nil
}

// System preferences are resolved using navigator.languages in each renderer.
func desktopLocale(preferences desktop.Preferences, revision uint64) map[string]any {
	language := preferences["language"]
	if language == "system" {
		language = "en"
	}
	return map[string]any{"preference": preferences["language"], "language": language, "revision": revision}
}
