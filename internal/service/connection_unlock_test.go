package service

import (
	"context"
	"errors"
	"testing"
)

func TestConnectionRequiresUnlock(t *testing.T) {
	app, _, store := managedBackupService(t)
	ctx := context.Background()
	create := func(input HostInput) Host {
		t.Helper()
		input.HostName = "fixture.invalid"
		input.Port = 22
		host, err := store.CreateHost(input)
		if err != nil {
			t.Fatal(err)
		}
		return host
	}
	system := create(HostInput{Alias: "system"})
	file := create(HostInput{Alias: "file", IdentityFile: "/tmp/fixture_key"})
	managed := create(HostInput{Alias: "managed", IdentityFile: "aterm-key://fixture"})
	password := create(HostInput{Alias: "password", PasswordAuth: true})
	jump := create(HostInput{Alias: "jump", JumpHostID: managed.ID})
	aliasJump := create(HostInput{Alias: "alias-jump", ProxyJump: "user@managed:22"})
	nested := create(HostInput{Alias: "nested", JumpHostID: jump.ID})
	for _, unlocked := range []bool{false, true} {
		if unlocked {
			if _, err := app.SetUpSecureData(ctx, "fixture-password"); err != nil {
				t.Fatal(err)
			}
		}
		for _, host := range []Host{system, file, managed, password, jump, aliasJump, nested} {
			want := !unlocked && host.ID != system.ID && host.ID != file.ID
			got, err := app.ConnectionRequiresUnlock(ctx, host.ID)
			if err != nil || got != want {
				t.Errorf("%s unlocked=%v: got %v, %v; want %v", host.Alias, unlocked, got, err, want)
			}
		}
	}
	if _, err := app.ConnectionRequiresUnlock(ctx, "missing"); !errors.Is(err, ErrHostNotFound) {
		t.Fatal("missing host:", err)
	}
}

func TestConnectionRequiresUnlockBoundsJumpCycles(t *testing.T) {
	app, _, store := managedBackupService(t)
	host, err := store.CreateHost(HostInput{Alias: "cycle", HostName: "fixture.invalid", Port: 22, ProxyJump: "cycle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ConnectionRequiresUnlock(context.Background(), host.ID); err == nil {
		t.Fatal("expected cycle depth error")
	}
}
