package terminal

import (
	"context"
	"ssh-connect/internal/desktop/openssh"
	"ssh-connect/internal/service"
	"testing"
)

func TestSnapshotDoesNotExposeMutableSessionMetadata(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	code := -1
	manager.sessions["fixture"] = &Session{ID: "fixture", HostID: "host", Kind: "local", State: "exited", ExitCode: &code, Remote: &openssh.RemoteContext{ID: "remote", Host: "original"}}
	manager.Present("fixture", "host", true)
	manager.SetSize("fixture", 120, 30)
	snapshot, err := manager.Snapshot("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.HostID != "host" || snapshot.LocalNumber != 1 || snapshot.Columns != 120 || snapshot.Rows != 30 {
		t.Fatal("incomplete snapshot", snapshot)
	}
	snapshot.Remote.Host = "changed"
	*snapshot.ExitCode = 0
	next, err := manager.Snapshot("fixture")
	if err != nil || next.Remote.Host != "original" || *next.ExitCode != -1 {
		t.Fatal("snapshot mutated live state", next, err)
	}
}

func TestExecutionContextRetainsConnectionIdentityAndRejectsNestedSSH(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	identity := service.Host{ID: "saved-host", HostName: "original"}
	manager.sessions["ssh"] = &Session{ID: "ssh", Kind: "ssh", State: "running", connectionIdentity: &identity}
	_, _, host, err := manager.ExecutionContext("ssh")
	if err != nil || host == nil || host.ID != identity.ID {
		t.Fatal("missing connection identity", host, err)
	}
	host.HostName = "changed"
	if identity.HostName != "original" {
		t.Fatal("execution context exposed mutable identity")
	}
	manager.sessions["ssh"].Remote = &openssh.RemoteContext{ID: "nested"}
	if _, _, _, err := manager.ExecutionContext("ssh"); err == nil {
		t.Fatal("nested SSH accepted for plugin execution")
	}
	if _, _, _, err := manager.ExecutionContext("missing"); err == nil {
		t.Fatal("missing session accepted")
	}
}
