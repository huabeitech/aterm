package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"net/url"
)

func (a *App) desktopCopyText(args []json.RawMessage) (any, error) {
	text, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	if a.wailsApp == nil || !a.wailsApp.Clipboard.SetText(text) {
		return nil, errors.New("unable to write to clipboard")
	}
	return nil, nil
}
func (a *App) desktopExternal(args []json.RawMessage) (any, error) {
	value, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("only HTTP and HTTPS links can be opened")
	}
	if a.wailsApp == nil {
		return nil, errors.New("application is not initialized")
	}
	return nil, a.wailsApp.Browser.OpenURL(value)
}
func (a *App) desktopWindow(ctx context.Context, args []json.RawMessage) (any, error) {
	action, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	window, _ := ctx.Value(application.WindowKey).(application.Window)
	if window == nil {
		return nil, errors.New("window action requires a window")
	}
	switch action {
	case "minimize":
		window.Minimise()
	case "maximize":
		window.ToggleMaximise()
	case "fullscreen":
		window.ToggleFullscreen()
	case "close":
		window.Close()
	default:
		return nil, errors.New("invalid window action")
	}
	return nil, nil
}
