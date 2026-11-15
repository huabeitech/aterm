package terminal

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPluginTerminalLaunchOptionsPrecedePublishedSnapshot(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	defer manager.CloseAll()
	options := LaunchOptions{Title: "Fixture Tool", InitialCommand: `printf '\160\154\165\147\151\156\055\154\141\165\156\143\150\055\166\145\162\151\146\151\145\144\n'`}
	session, err := manager.openWithOptions("local", "Local Shell", "", exec.Command("/bin/sh", "-i"), options)
	if err != nil {
		t.Fatal(err)
	}
	if session.Title != "Fixture Tool" || session.State != "running" {
		t.Fatal("plugin launch published an incomplete session", session)
	}
	deadline := time.After(time.Second)
	for {
		manager.mu.RLock()
		output := string(manager.sessions[session.ID].output)
		manager.mu.RUnlock()
		if strings.Contains(output, "plugin-launch-verified") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("initial command was not delivered to the new PTY")
		case <-time.After(time.Millisecond):
		}
	}
	view, err := manager.Snapshot(session.ID)
	if err != nil || view.Title != "Fixture Tool" {
		t.Fatal("shared terminal adapter replaced the plugin title", view, err)
	}
}
