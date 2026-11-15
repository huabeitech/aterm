package app

import "github.com/wailsapp/wails/v3/pkg/application"

// A published window reference is immutable. Each action captures one snapshot
// so replacing the main window cannot retarget an in-flight dialog/event.
type mainWindowReference struct{ window application.Window }

func (a *App) mainWindow() application.Window {
	reference := a.mainWindowRef.Load()
	if reference == nil {
		return nil
	}
	return reference.window
}

// A delayed close from an old window must not clear a replacement's reference.
func (a *App) clearMainWindow(id uint) bool {
	for {
		reference := a.mainWindowRef.Load()
		if reference == nil || reference.window.ID() != id {
			return false
		}
		if a.mainWindowRef.CompareAndSwap(reference, nil) {
			return true
		}
	}
}
