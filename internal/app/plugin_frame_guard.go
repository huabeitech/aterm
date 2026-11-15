package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"ssh-connect/internal/desktop/platform"
)

func (a *App) securePluginFrames(window application.Window) bool {
	secured := platform.SecurePluginFrames(window)
	if secured && a.desktopPluginViews != nil {
		a.desktopPluginViews.SetNativeBridgeGuarded(true)
	}
	return secured
}
