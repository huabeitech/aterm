package agentconn

import (
	"context"
	"errors"
	"testing"
)

func TestEndpointSelectionHonorsExplicitSocket(t *testing.T) {
	for _, test := range []struct{ platform, socket, want string }{
		{"darwin", "", ""}, {"linux", "", ""}, {"windows", "", WindowsOpenSSHPipe},
		{"windows", `\\.\pipe\custom-agent`, `\\.\pipe\custom-agent`}, {"windows", "pageant", "pageant"}, {"darwin", "/tmp/fixture agent.sock", "/tmp/fixture agent.sock"},
	} {
		if got := defaultEndpoint(test.platform, test.socket); got != test.want {
			t.Fatalf("endpoint for %s/%q = %q", test.platform, test.socket, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Dial(ctx, "fixture"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled dial touched endpoint", err)
	}
}
