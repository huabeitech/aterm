//go:build darwin && !mas

package app

import (
	"encoding/json"
	"testing"

	"ssh-connect/internal/service"
)

func TestAgentGrantRouteFingerprint(t *testing.T) {
	target := service.Host{ID: "target", HostName: "target.example", User: "app", JumpHostID: "jump"}
	jump := service.Host{ID: "jump", HostName: "jump.example", IdentityFile: "key"}
	approved := agentRouteFingerprint([]service.Host{target, jump})

	for name, change := range map[string]func(*service.Host, *service.Host){
		"address":  func(target, jump *service.Host) { target.HostName = "other.example" },
		"port":     func(target, jump *service.Host) { target.Port = 2222 },
		"user":     func(target, jump *service.Host) { target.User = "other" },
		"password": func(target, jump *service.Host) { target.PasswordAuth = true },
		"identity": func(target, jump *service.Host) { jump.IdentityFile = "other-key" },
		"jump":     func(target, jump *service.Host) { jump.HostName = "other-jump.example" },
	} {
		t.Run(name, func(t *testing.T) {
			changedTarget, changedJump := target, jump
			change(&changedTarget, &changedJump)
			if agentRouteFingerprint([]service.Host{changedTarget, changedJump}) == approved {
				t.Fatal("connection change reused approval")
			}
		})
	}
	target.Remark = "updated notes"
	target.Port = 22
	if agentRouteFingerprint([]service.Host{target, jump}) != approved {
		t.Fatal("notes and equivalent default port must not invalidate approval")
	}
}

func TestAgentGrantPersistenceAndRevocation(t *testing.T) {
	dir := t.TempDir()
	app := &App{}
	if err := app.startAgents(dir); err != nil {
		t.Fatal(err)
	}
	cfg := agentConfig{Grants: map[string]agentGrant{
		"first":  {Name: "First", Endpoint: "first.example:22", Fingerprint: [32]byte{1}},
		"second": {Name: "Second", Endpoint: "second.example:22", Fingerprint: [32]byte{2}},
	}}
	if err := app.agents.writeConfig(cfg); err != nil {
		t.Fatal(err)
	}

	restarted := &App{}
	if err := restarted.startAgents(dir); err != nil {
		t.Fatal(err)
	}
	if restarted.agents.config.Grants["first"] != cfg.Grants["first"] {
		t.Fatal("approval did not survive restart")
	}
	if _, err := restarted.agentDesktop("save", []json.RawMessage{json.RawMessage(`{"enabled":false}`)}); err != nil {
		t.Fatal(err)
	}
	if len(restarted.agents.config.Grants) != 2 {
		t.Fatal("saving settings cleared approvals")
	}
	if _, err := restarted.agentDesktop("revokeGrant", []json.RawMessage{json.RawMessage(`"first"`)}); err != nil {
		t.Fatal(err)
	}
	if len(restarted.agents.config.Grants) != 1 || restarted.agents.config.Grants["second"] != cfg.Grants["second"] {
		t.Fatal("revocation must remove only the selected approval")
	}
	if _, err := restarted.agentDesktop("revokeGrant", []json.RawMessage{json.RawMessage(`""`)}); err != nil {
		t.Fatal(err)
	}
	cleared := &App{}
	if err := cleared.startAgents(dir); err != nil {
		t.Fatal(err)
	}
	if len(cleared.agents.config.Grants) != 0 {
		t.Fatal("revoked approvals reappeared after restart")
	}
}
