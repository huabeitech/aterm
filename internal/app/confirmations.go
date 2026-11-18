package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type confirmationChoice string

const (
	confirmationCancel confirmationChoice = "cancel"
	confirmationAccept confirmationChoice = "accept"
	confirmationExtra  confirmationChoice = "extra"
)

type windowConfirmation struct {
	TitleKey     string            `json:"titleKey,omitempty"`
	MessageKey   string            `json:"messageKey,omitempty"`
	ConfirmKey   string            `json:"confirmKey,omitempty"`
	CancelKey    string            `json:"cancelKey,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
	Title        string            `json:"title"`
	Message      string            `json:"message"`
	ConfirmLabel string            `json:"confirmLabel"`
	CancelLabel  string            `json:"cancelLabel,omitempty"`
	ExtraLabel   string            `json:"extraLabel,omitempty"`
	Danger       bool              `json:"danger,omitempty"`
}

type confirmationEvent struct {
	Type    string              `json:"type"`
	ID      string              `json:"id"`
	Options *windowConfirmation `json:"options,omitempty"`
}

func dispatchConfirmation(window application.Window, event confirmationEvent) {
	window.DispatchWailsEvent(&application.CustomEvent{
		Name: "confirmation:event", Sender: window.Name(), Data: event,
	})
}

type pendingConfirmation struct {
	windowID uint
	hasExtra bool
	answer   chan confirmationChoice
}

func (a *App) confirmAction(ctx context.Context, options windowConfirmation) bool {
	return a.askConfirmation(ctx, a.mainWindow(), options) == confirmationAccept
}

func (a *App) askConfirmation(ctx context.Context, window application.Window, options windowConfirmation) confirmationChoice {
	if window == nil || ctx.Err() != nil {
		return confirmationCancel
	}
	id := uuid.NewString()
	pending := pendingConfirmation{
		windowID: window.ID(),
		hasExtra: options.ExtraLabel != "",
		answer:   make(chan confirmationChoice, 1),
	}
	a.desktopMu.Lock()
	if a.desktopConfirmations == nil {
		a.desktopConfirmations = make(map[string]pendingConfirmation)
	}
	a.desktopConfirmations[id] = pending
	a.desktopMu.Unlock()

	dispatchConfirmation(window, confirmationEvent{Type: "open", ID: id, Options: &options})
	defer func() {
		a.desktopMu.Lock()
		delete(a.desktopConfirmations, id)
		a.desktopMu.Unlock()
		dispatchConfirmation(window, confirmationEvent{Type: "close", ID: id})
	}()
	select {
	case answer := <-pending.answer:
		if ctx.Err() == nil {
			return answer
		}
	case <-ctx.Done():
	}
	return confirmationCancel
}

func (a *App) answerConfirmation(ctx context.Context, args []json.RawMessage) error {
	id, err := argument[string](args, 0)
	if err != nil {
		return err
	}
	choice, err := argument[confirmationChoice](args, 1)
	if err != nil {
		return err
	}
	window, _ := ctx.Value(application.WindowKey).(application.Window)
	if window == nil {
		return errors.New("confirmation requires a window")
	}

	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	pending, ok := a.desktopConfirmations[id]
	if !ok || pending.windowID != window.ID() {
		return errors.New("confirmation is no longer open in this window")
	}
	if choice != confirmationCancel && choice != confirmationAccept &&
		(choice != confirmationExtra || !pending.hasExtra) {
		return errors.New("unknown confirmation action")
	}
	delete(a.desktopConfirmations, id)
	pending.answer <- choice
	return nil
}

func (a *App) cancelWindowConfirmations(windowID uint) {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	for id, pending := range a.desktopConfirmations {
		if pending.windowID == windowID {
			delete(a.desktopConfirmations, id)
			pending.answer <- confirmationCancel
		}
	}
}
