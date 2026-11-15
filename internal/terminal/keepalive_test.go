package terminal

import (
	"errors"
	"testing"

	"ssh-connect/internal/service"
)

func TestTerminalConnectionFailureMessageForKeepaliveTimeout(t *testing.T) {
	cause := errors.Join(errors.New("terminal stopped"), service.ErrSSHKeepaliveTimeout)
	if got := terminalConnectionFailureMessage(cause); got != terminalKeepaliveFailureMessage {
		t.Fatalf("message = %q, want %q", got, terminalKeepaliveFailureMessage)
	}
}

func TestTerminalConnectionFailureMessageIgnoresOtherExits(t *testing.T) {
	for _, cause := range []error{nil, errors.New("remote command exited"), errors.New("terminal closed by user")} {
		if got := terminalConnectionFailureMessage(cause); got != "" {
			t.Fatalf("message for %v = %q, want empty", cause, got)
		}
	}
}
