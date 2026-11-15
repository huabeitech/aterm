package app

import "ssh-connect/internal/desktop/platform"

// Called from ApplicationDidFinishLaunching, when AppKit is ready. The shared
// React terminal already consumes this event and updates xterm dynamically.
func (a *App) startDesktopAccessibility() {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.desktopAccessibilityStop != nil {
		return
	}
	a.desktopAccessibilityStop = platform.ObserveAccessibility(func(enabled bool) {
		if a.wailsApp != nil {
			a.wailsApp.Event.Emit("terminal:event", map[string]any{"type": "accessibility", "enabled": enabled})
		}
	})
}

func (a *App) stopDesktopAccessibility() {
	a.desktopMu.Lock()
	stop := a.desktopAccessibilityStop
	a.desktopAccessibilityStop = nil
	a.desktopMu.Unlock()
	if stop != nil {
		stop()
	}
}
