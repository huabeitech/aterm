//go:build !windows

package terminal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"ssh-connect/internal/service"
)

func TestTerminalExitSnapshotIncludesRealPTYTail(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "") // This fixture must never contact the user's agent.
	for _, kind := range []string{"local", "ssh"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			directory := t.TempDir()
			payload := append(bytes.Repeat([]byte("x"), 2*1024*1024), []byte("最后的输出")...)
			path := filepath.Join(directory, "output.txt")
			if err := os.WriteFile(path, payload, 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(ctx, "/bin/cat", path)
			command.Dir = directory
			exits := make(chan OutputEvent, 1)
			manager := NewManager(ctx, func(_ string, payload any) {
				event, ok := payload.(OutputEvent)
				if ok && event.State == "exited" {
					exits <- event
				}
			})
			defer manager.CloseAll()
			var session Session
			var err error
			if kind == "local" {
				session, err = manager.open("local", "tail", "", command)
			} else {
				_, private, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				signer, err := ssh.NewSignerFromKey(private)
				if err != nil {
					t.Fatal(err)
				}
				block, err := ssh.MarshalPrivateKey(private, "disposable exit fixture")
				if err != nil {
					t.Fatal(err)
				}
				identity := filepath.Join(directory, "identity")
				if err := os.WriteFile(identity, pem.EncodeToMemory(block), 0600); err != nil {
					t.Fatal(err)
				}
				address := terminalSSHPeer(t, ctx, command, signer.PublicKey(), nil)
				hostname, portString, err := net.SplitHostPort(address)
				if err != nil {
					t.Fatal(err)
				}
				port, err := strconv.Atoi(portString)
				if err != nil {
					t.Fatal(err)
				}
				svc := sshServiceFixture(t)
				const alias = "exit-fixture"
				if _, err := svc.CreateHost(ctx, service.HostInput{Name: alias, Alias: alias, HostName: hostname, Port: port, User: "fixture", IdentityFile: identity}); err != nil {
					t.Fatal(err)
				}
				session, err = manager.OpenSSHAsync(alias, func(ctx context.Context, _ service.HostKeyConfirmer) (*service.NativeSSHClient, string, error) {
					client, host, err := svc.OpenNativeSSHWithHostKeyConfirmer(ctx, alias, func(context.Context, service.HostKeyInfo) (bool, error) { return true, nil })
					return client, host.Alias, err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case event := <-exits:
				if event.ExitCode == nil || *event.ExitCode != 0 {
					t.Fatal(event)
				}
			case <-ctx.Done():
				t.Fatal("terminal did not exit after output")
			}
			snapshot, err := manager.OutputSnapshot(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := base64.StdEncoding.DecodeString(snapshot.Data)
			if err != nil {
				t.Fatal(err)
			}
			want := payload[len(payload)-terminalReplayLimit:]
			if !bytes.Equal(replay, want) || snapshot.Offset < uint64(len(payload)) {
				t.Fatalf("exit replay lost tail: got=%d offset=%d", len(replay), snapshot.Offset)
			}
		})
	}
}
