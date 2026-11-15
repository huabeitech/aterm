package terminal

import (
	"context"
	"testing"
	"time"
)

func TestTerminalCloseRemovesExitedSession(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	session := &Session{
		ID:        "terminal-1",
		Title:     "Local Shell",
		Kind:      "local",
		State:     "exited",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		output:    []byte("done\r\n"),
	}
	manager.sessions[session.ID] = session

	if err := manager.Close(session.ID); err != nil {
		t.Fatalf("close exited session: %v", err)
	}
	if _, err := manager.Scrollback(session.ID); err == nil {
		t.Fatal("scrollback succeeded after close; session was not removed")
	}
}

func TestTerminalCloseIsIdempotent(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	if err := manager.Close("missing"); err != nil {
		t.Fatalf("close missing session: %v", err)
	}
}

func TestTerminalCloseCancelsConnectingSessionAndRemovesIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewManager(context.Background(), nil)
	session := &Session{
		ID:        "terminal-connecting",
		Title:     "server",
		Kind:      "ssh",
		State:     "connecting",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		cancel:    cancel,
	}
	manager.sessions[session.ID] = session

	if err := manager.Close(session.ID); err != nil {
		t.Fatalf("close connecting session: %v", err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("close did not cancel connecting session context")
	}
	if _, ok := manager.sessions[session.ID]; ok {
		t.Fatal("connecting session remains in manager after close")
	}
}
