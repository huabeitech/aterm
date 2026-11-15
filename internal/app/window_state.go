package app

import (
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"ssh-connect/internal/desktop/windowstate"
)

type mainWindowPlacement struct {
	app     *App
	window  *application.WebviewWindow
	once    sync.Once
	ready   atomic.Bool
	closed  atomic.Bool
	mu      sync.Mutex
	timer   *time.Timer
	tracker *windowstate.Tracker
}

func (a *App) bindMainWindowPlacement(window *application.WebviewWindow) *mainWindowPlacement {
	placement := &mainWindowPlacement{app: a, window: window}
	a.mainWindowPlacement = placement
	for _, event := range []events.WindowEventType{events.Common.WindowDidMove, events.Common.WindowDidResize, events.Common.WindowMaximise, events.Common.WindowUnMaximise, events.Common.WindowRestore} {
		window.OnWindowEvent(event, func(*application.WindowEvent) { placement.schedule() })
	}
	return placement
}
func (p *mainWindowPlacement) restore() {
	p.once.Do(func() {
		var saved *windowstate.State
		if p.app.store != nil {
			value, err := p.app.store.GetAppMetadata(windowstate.MetadataKey)
			if err == nil {
				saved = windowstate.Decode(value)
			} else {
				slog.Warn("Unable to read main window state", "error", err)
			}
		}
		screens := p.app.wailsApp.Screen.GetAll()
		areas := []windowstate.Rect{}
		// Electron puts the primary display first, preserving platform order after it.
		for _, primary := range []bool{true, false} {
			for _, screen := range screens {
				if screen.IsPrimary == primary {
					area := screen.WorkArea
					areas = append(areas, windowstate.Rect{X: area.X, Y: area.Y, Width: area.Width, Height: area.Height})
				}
			}
		}
		fitted := windowstate.Fit(saved, areas)
		// Match the original BrowserWindow construction's minimum constraints.
		fitted.Width = max(1100, fitted.Width)
		fitted.Height = max(680, fitted.Height)
		initial := windowstate.State{Rect: fitted}
		if saved != nil {
			initial.Maximized = saved.Maximized
		}
		p.tracker = windowstate.New(initial, func(state windowstate.State) error {
			encoded, err := json.Marshal(state)
			if err != nil {
				return err
			}
			if p.app.store == nil {
				return nil
			}
			return p.app.store.SetAppMetadata(windowstate.MetadataKey, string(encoded))
		})
		p.window.SetBounds(application.Rect{X: fitted.X, Y: fitted.Y, Width: fitted.Width, Height: fitted.Height})
		if initial.Maximized {
			p.window.Maximise()
		}
		p.mu.Lock()
		if !p.closed.Load() {
			p.ready.Store(true)
		}
		p.mu.Unlock()
	})
}
func (p *mainWindowPlacement) schedule() {
	if !p.ready.Load() || p.closed.Load() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		return
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(250*time.Millisecond, p.capture)
}
func (p *mainWindowPlacement) capture() {
	if !p.ready.Load() || p.closed.Load() {
		return
	}
	// Never hold the timer/state mutex while dispatching a synchronous native read.
	minimized, fullscreen := p.window.IsMinimised(), p.window.IsFullscreen()
	if minimized || fullscreen {
		return
	}
	maximized := p.window.IsMaximised()
	bounds := p.window.Bounds()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	if err := p.tracker.Observe(windowstate.Rect{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}, maximized, false, false); err != nil {
		slog.Warn("Unable to save main window state", "error", err)
	}
}
func (p *mainWindowPlacement) close() {
	if p == nil || p.closed.Swap(true) {
		return
	}
	p.mu.Lock()
	if p.timer != nil {
		p.timer.Stop()
	}
	p.mu.Unlock()
	if p.ready.Load() {
		if err := p.tracker.Close(); err != nil {
			slog.Warn("Unable to save main window state", "error", err)
		}
	}
}
