package app

import (
	"context"
	"encoding/json"
	"errors"
	"ssh-connect/internal/desktop/platform"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) desktopInputMode(ctx context.Context, args []json.RawMessage) (any, error) {
	mode, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	switch mode {
	case "normal", "editable", "recorder", "terminal":
	default:
		return nil, errors.New("invalid workspace input mode")
	}
	window, _ := ctx.Value(application.WindowKey).(application.Window)
	if window == nil {
		return nil, errors.New("input mode requires a window")
	}
	platform.SetInputMode(window, mode)
	return nil, nil
}
