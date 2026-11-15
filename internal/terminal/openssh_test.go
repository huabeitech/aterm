package terminal

import (
	"context"
	"strings"
	"testing"

	"ssh-connect/internal/desktop/openssh"
)

func TestTerminalFileTargetRequiresLiveMatchingContext(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	integration, err := openssh.New("/usr/bin/ssh")
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Close()
	entry := &Session{ID: "local", Kind: "local", State: "running", Remote: &openssh.RemoteContext{ID: "c.current", Host: "fixture", User: "user", Port: 22}, integration: integration}
	manager.sessions[entry.ID] = entry
	for _, target := range []struct{ id, contextID string }{{"missing", "c.current"}, {"local", "c.old"}, {"local", "c.current"}} {
		if _, _, _, err = manager.OpenFiles(context.Background(), target.id, target.contextID); err == nil {
			t.Fatal("unverified or stale context accepted", target)
		}
	}

}
func TestNestedOpenSSHContextCannotExecuteLocalPluginProcess(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	manager.sessions["local"] = &Session{ID: "local", Kind: "local", State: "running", Remote: &openssh.RemoteContext{ID: "c.current"}}
	_, _, _, err := manager.ExecutionContext("local")
	if err == nil || !strings.Contains(err.Error(), "terminal disconnected") {
		t.Fatal("nested SSH context reached local process execution", err)
	}
}
