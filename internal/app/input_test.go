package app

import (
	"context"
	"encoding/json"
	"testing"
)

func TestInputAndWindowActionsRequireCallingWindow(t *testing.T) {
	app := &App{}
	if _, err := app.desktopInputMode(context.Background(), []json.RawMessage{json.RawMessage(`"terminal"`)}); err == nil {
		t.Fatal("input mode accepted without a calling window")
	}
	if _, err := app.desktopWindow(context.Background(), []json.RawMessage{json.RawMessage(`"close"`)}); err == nil {
		t.Fatal("window close accepted without a calling window")
	}
	if _, err := app.desktopInputMode(context.Background(), []json.RawMessage{json.RawMessage(`"unknown"`)}); err == nil {
		t.Fatal("invalid input mode accepted")
	}
}
