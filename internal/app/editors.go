package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	desktopfiles "ssh-connect/internal/desktop/files"
)

type editorCloseRequest struct {
	done   chan struct{}
	closed bool
}

type editorWindow struct {
	mu            sync.Mutex
	window        application.Window
	sessionID     string
	editor        *desktopfiles.Editor
	closing       bool
	closeRequest  *editorCloseRequest
	rendererReady bool
}
type editorWindows struct {
	quitPending  bool
	quitApproved bool
	mainPending  bool
	mainApproved bool
	mu           sync.Mutex
	opening      sync.Mutex
	windows      map[uint]*editorWindow
}

func (a *App) editors() *editorWindows {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.desktopEditors == nil {
		a.desktopEditors = &editorWindows{windows: map[uint]*editorWindow{}}
	}
	return a.desktopEditors
}
func (a *App) openEditor(sessionID, name string) error {
	if a.wailsApp == nil {
		return errors.New("application is not initialized")
	}
	windows := a.editors()
	windows.opening.Lock()
	defer windows.opening.Unlock()
	windows.mu.Lock()
	for _, entry := range windows.windows {
		if entry.sessionID == sessionID && entry.editor.State().Document.Path == name {
			window := entry.window
			windows.mu.Unlock()
			window.Show()
			window.Focus()
			return nil
		}
	}
	windows.mu.Unlock()
	session, err := a.fileManager().Session(sessionID)
	if err != nil {
		return err
	}
	document, err := a.fileManager().ReadText(sessionID, name)
	if err != nil {
		return err
	}
	id := "file-editor-" + uuid.NewString()
	window := a.wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: id, Title: path.Base(name) + " — " + session.Info.Title, Width: 960, Height: 720, MinWidth: 560, MinHeight: 360, URL: "/#file-editor", Hidden: true,
	})
	entry := &editorWindow{window: window, sessionID: sessionID, editor: desktopfiles.NewEditor(document, session.Info.Title, func(document desktopfiles.Text) (desktopfiles.Text, error) {
		result, err := a.fileManager().SaveText(sessionID, document)
		if err == nil {
			a.emitEvent("files:event", map[string]any{"type": "saved", "sessionID": sessionID, "path": document.Path})
		}
		return result, err
	})}
	windows.mu.Lock()
	windows.windows[window.ID()] = entry
	windows.mu.Unlock()
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		entry.mu.Lock()
		closing := entry.closing
		entry.mu.Unlock()
		if closing {
			return
		}
		event.Cancel()
		a.requestEditorClose(entry)
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		a.cancelWindowConfirmations(window.ID())
		windows.mu.Lock()
		delete(windows.windows, window.ID())
		windows.mu.Unlock()
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) { window.Show(); window.Focus() })
	window.Show()
	window.Focus()
	return nil
}
func (a *App) editorFor(window application.Window) (*editorWindow, error) {
	if window == nil {
		return nil, errors.New("no file editor belongs to this window")
	}
	windows := a.editors()
	windows.mu.Lock()
	defer windows.mu.Unlock()
	entry := windows.windows[window.ID()]
	if entry == nil {
		return nil, errors.New("no file editor belongs to this window")
	}
	return entry, nil
}
func (a *App) publishEditor(entry *editorWindow) {
	state := entry.editor.State()
	prefix := ""
	if state.Text != state.Document.Text {
		prefix = "● "
	}
	entry.window.SetTitle(prefix + path.Base(state.Document.Path) + " — " + state.Host)
	entry.window.EmitEvent("file-editor:state", state)
}
func (a *App) closeEditor(entry *editorWindow) error {
	defer func() {
		entry.mu.Lock()
		request := entry.closeRequest
		entry.closeRequest = nil
		if request != nil {
			request.closed = entry.closing
			close(request.done)
		}
		entry.mu.Unlock()
	}()
	state := entry.editor.State()
	if state.Saving {
		return errors.New("wait for the file to finish saving")
	}
	if state.Text != state.Document.Text {
		entry.window.Focus()
		choice := a.askConfirmation(a.ctx, entry.window, windowConfirmation{
			Title:        "Save changes?",
			Message:      fmt.Sprintf("Save changes to %s before closing? Your changes will be lost if you close without saving.", path.Base(state.Document.Path)),
			ConfirmLabel: "Save and Close",
			ExtraLabel:   "Don't Save",
		})
		if choice == confirmationCancel {
			return nil
		}
		if choice == confirmationAccept {
			if err := entry.editor.Save(func() { a.publishEditor(entry) }); err != nil {
				return err
			}
		}
	}
	entry.mu.Lock()
	entry.closing = true
	entry.mu.Unlock()
	entry.window.Close()
	return nil
}
func (a *App) disconnectEditors(sessionID string) {
	windows := a.editors()
	windows.mu.Lock()
	entries := []*editorWindow{}
	for _, entry := range windows.windows {
		if entry.sessionID == sessionID {
			entries = append(entries, entry)
		}
	}
	windows.mu.Unlock()
	for _, entry := range entries {
		entry.editor.Disconnect()
		a.publishEditor(entry)
	}
}
func (a *App) desktopFileEditor(ctx context.Context, raw []json.RawMessage) (any, error) {
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	if op == "open" {
		sessionID, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		name, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return nil, a.openEditor(sessionID, name)
	}
	window, _ := ctx.Value(application.WindowKey).(application.Window)
	entry, err := a.editorFor(window)
	if err != nil {
		return nil, err
	}
	switch op {
	case "state":
		entry.mu.Lock()
		entry.rendererReady = true
		entry.mu.Unlock()
		return entry.editor.State(), nil
	case "change":
		text, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		if err := entry.editor.Change(text); err != nil {
			return nil, err
		}
		a.publishEditor(entry)
		return nil, nil
	case "save":
		return nil, entry.editor.Save(func() { a.publishEditor(entry) })
	case "close":
		return nil, a.closeEditor(entry)
	case "quit":
		a.shouldQuitEditors()
		return nil, nil
	default:
		return nil, errors.New("unknown file editor operation")
	}
}

// requestEditorClose asks React to drain its change queue before Go snapshots
// the draft or asks the unsaved-changes question. Completion is broadcast
// so owner close and application quit can share one in-flight request.
func (a *App) requestEditorClose(entry *editorWindow) *editorCloseRequest {
	entry.mu.Lock()
	if entry.closeRequest != nil {
		request := entry.closeRequest
		entry.mu.Unlock()
		return request
	}
	request := &editorCloseRequest{done: make(chan struct{})}
	if entry.closing {
		request.closed = true
		close(request.done)
		entry.mu.Unlock()
		return request
	}
	entry.closeRequest = request
	ready := entry.rendererReady
	entry.mu.Unlock()
	if ready {
		entry.window.EmitEvent("file-editor:close-requested")
	} else {
		go func() { _ = a.closeEditor(entry) }()
	}
	return request
}
func (a *App) closeAllEditors() bool {
	windows := a.editors()
	windows.mu.Lock()
	entries := []*editorWindow{}
	for _, entry := range windows.windows {
		entries = append(entries, entry)
	}
	windows.mu.Unlock()
	for _, entry := range entries {
		request := a.requestEditorClose(entry)
		<-request.done
		if !request.closed {
			return false
		}
	}
	return true
}

// Native quit/close hooks must return promptly. Blocking their main-thread
// callback would prevent the editor renderer from flushing its final keystroke.
func (a *App) shouldQuitEditors() bool {
	windows := a.editors()
	windows.mu.Lock()
	if windows.quitApproved || len(windows.windows) == 0 {
		windows.mu.Unlock()
		return true
	}
	if windows.quitPending {
		windows.mu.Unlock()
		return false
	}
	windows.quitPending = true
	windows.mu.Unlock()
	go func() {
		closed := a.closeAllEditors()
		windows.mu.Lock()
		windows.quitPending = false
		windows.quitApproved = closed
		windows.mu.Unlock()
		if closed && a.wailsApp != nil {
			a.wailsApp.Quit()
		}
	}()
	return false
}
func (a *App) allowMainClose(target application.Window) bool {
	windows := a.editors()
	windows.mu.Lock()
	if windows.mainApproved || len(windows.windows) == 0 {
		windows.mu.Unlock()
		return true
	}
	if windows.mainPending {
		windows.mu.Unlock()
		return false
	}
	windows.mainPending = true
	windows.mu.Unlock()
	go func() {
		closed := a.closeAllEditors()
		windows.mu.Lock()
		windows.mainPending = false
		windows.mainApproved = closed
		windows.mu.Unlock()
		if closed && target != nil {
			target.Close()
		}
	}()
	return false
}
func (a *App) dispatchWorkspaceCommand(command string) {
	if a.wailsApp == nil {
		return
	}
	window := a.wailsApp.Window.Current()
	if window == nil {
		return
	}
	if entry, err := a.editorFor(window); err == nil {
		if command == "close-tab" {
			a.requestEditorClose(entry)
		} else if command == "find" {
			window.EmitEvent("workspace:command", command)
		}
		return
	}
	window.EmitEvent("workspace:command", command)
}
