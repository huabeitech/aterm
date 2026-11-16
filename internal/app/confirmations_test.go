package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type confirmationTestWindow struct {
	application.Window
	id     uint
	events chan confirmationEvent
}

func (w *confirmationTestWindow) ID() uint { return w.id }

func (w *confirmationTestWindow) Name() string { return "confirmation-test" }

func (w *confirmationTestWindow) DispatchWailsEvent(event *application.CustomEvent) {
	w.events <- event.Data.(confirmationEvent)
}

func TestWindowConfirmationAnswersAreScopedAndSingleUse(t *testing.T) {
	app := &App{}
	owner := &confirmationTestWindow{id: 1, events: make(chan confirmationEvent, 2)}
	other := &confirmationTestWindow{id: 2}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	answer := make(chan confirmationChoice, 1)
	go func() {
		answer <- app.askConfirmation(ctx, owner, windowConfirmation{
			ConfirmLabel: "Save and Close",
			ExtraLabel:   "Don't Save",
		})
	}()

	request := <-owner.events
	id, _ := json.Marshal(request.ID)
	reply := func(window application.Window, action string) error {
		value, _ := json.Marshal(action)
		windowCtx := context.WithValue(ctx, application.WindowKey, window)
		return app.answerConfirmation(windowCtx, []json.RawMessage{id, value})
	}
	if err := reply(other, "extra"); err == nil {
		t.Fatal("another window answered the confirmation")
	}
	if err := reply(owner, "unknown"); err == nil {
		t.Fatal("an undeclared action was accepted")
	}
	if err := reply(owner, "extra"); err != nil {
		t.Fatal(err)
	}
	if got := <-answer; got != confirmationExtra {
		t.Fatalf("answer = %q, want extra", got)
	}
	if event := <-owner.events; event.Type != "close" || event.ID != request.ID {
		t.Fatalf("wrong close event: %v", event)
	}
	if err := reply(owner, "accept"); err == nil {
		t.Fatal("confirmation accepted a second answer")
	}
}

func TestWindowCloseCancelsPendingConfirmation(t *testing.T) {
	app := &App{}
	window := &confirmationTestWindow{id: 1, events: make(chan confirmationEvent, 2)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	answer := make(chan confirmationChoice, 1)
	go func() {
		answer <- app.askConfirmation(ctx, window, windowConfirmation{
			ConfirmLabel: "OK",
		})
	}()

	<-window.events
	app.cancelWindowConfirmations(window.ID())
	if got := <-answer; got != confirmationCancel || ctx.Err() != nil {
		t.Fatalf("window close returned %q, context error %v", got, ctx.Err())
	}
}
