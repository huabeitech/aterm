package terminal

import (
	"context"
	"errors"
	"testing"
	"time"

	"ssh-connect/internal/service"
)

func TestAsyncTerminalReturnsAnOpeningSnapshotBeforeConnectorMutatesState(t *testing.T) {
	events := make(chan OutputEvent, 100)
	manager := NewManager(context.Background(), func(_ string, payload any) { events <- payload.(OutputEvent) })
	defer manager.CloseAll()
	for i := 0; i < 100; i++ {
		session, err := manager.openSSHWithConnector("fixture", func(context.Context, *Session) (*service.NativeSSHClient, string, error) {
			return nil, "", errors.New("fixture connection failed")
		})
		if err != nil {
			t.Fatal(err)
		}
		if session.State != "connecting" {
			t.Fatalf("opening snapshot was mutated by connector: %q", session.State)
		}
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for i := 0; i < 100; i++ {
		select {
		case event := <-events:
			if event.State != "exited" {
				t.Fatalf("unexpected connection result %q", event.State)
			}
		case <-timer.C:
			t.Fatal("asynchronous connection results were lost")
		}
	}
}
