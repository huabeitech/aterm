package app

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"ssh-connect/internal/desktop/product"
)

// Keep construction and per-window hooks together: a future activation must use
// exactly the same options and placement behavior as initial application launch.
func (a *App) createMainWindow() (application.Window, error) {
	if a.wailsApp == nil {
		return nil, errors.New("application is not initialized")
	}
	a.mainWindowCreateMu.Lock()
	defer a.mainWindowCreateMu.Unlock()
	if window := a.mainWindow(); window != nil {
		return window, nil
	}
	window := a.wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: product.Info().Name, Width: 1180, Height: 780, MinWidth: 1100, MinHeight: 680,
		EnableFileDrop: true, BackgroundType: application.BackgroundTypeTranslucent,
		BackgroundColour: application.RGBA{Red: 246, Green: 246, Blue: 248, Alpha: 0}, URL: "/",
		Mac: application.MacWindow{TitleBar: application.MacTitleBarHidden, Appearance: application.NSAppearanceNameAqua, Backdrop: application.MacBackdropTranslucent},
	})
	a.setWindow(window)
	placement := a.bindMainWindowPlacement(window)
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { placement.restore(); window.Show(); window.Focus() })
	window.OnWindowEvent(events.Mac.WebViewDidFinishNavigation, func(*application.WindowEvent) { placement.restore(); window.Show() })
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if !a.allowMainClose(window) {
			event.Cancel()
			return
		}
		placement.capture()
		placement.close()
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.clearMainWindow(window.ID())
		a.cancelWindowConfirmations(window.ID())
	})
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		a.emitEvent("files-dropped", map[string]any{"files": event.Context().DroppedFiles(), "details": event.Context().DropTargetDetails()})
	})
	return window, nil
}
