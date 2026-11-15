//go:build !windows

package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type authTimingAgent struct {
	key         ssh.PublicKey
	connections atomic.Int32
	done        chan struct{}
	listener    net.Listener
}

func startAuthTimingAgent(t *testing.T) *authTimingAgent {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "aterm-auth-timing-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.RemoveAll(directory)
		t.Fatal(err)
	}
	t.Setenv("SSH_AUTH_SOCK", socket)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	ring := agent.NewKeyring()
	if err := ring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	fixture := &authTimingAgent{key: signer.PublicKey(), listener: listener, done: make(chan struct{})}
	go func() {
		defer close(fixture.done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		fixture.connections.Add(1)
		defer connection.Close()
		_ = agent.ServeAgent(ring, connection)
	}()
	t.Cleanup(func() {
		listener.Close()
		select {
		case <-fixture.done:
		case <-time.After(time.Second):
			t.Error("authentication agent connection leaked")
		}
		os.RemoveAll(directory)
	})
	return fixture
}

func TestNativeSSHAgentAndServerDecideWhetherDefaultKeysAreLoaded(t *testing.T) {
	material := encryptedIdentityFixture(t)
	fileSigner, err := ssh.ParsePrivateKeyWithPassphrase(material, []byte("correct"))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"agent-accepted", "agent-rejected", "agent-partial-password", "password-only", "no-auth", "reject-host", "oversized-default-agent-accepted"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := startAuthTimingAgent(t)
			directory := t.TempDir()
			data := material
			if scenario == "oversized-default-agent-accepted" {
				data = make([]byte, maxNativeIdentityBytes+1)
			}
			if err := os.WriteFile(filepath.Join(directory, "id_ed25519"), data, 0600); err != nil {
				t.Fatal(err)
			}
			var trusted atomic.Bool
			var passphrases, passwords atomic.Int32
			options := NativeSSHAuthOptions{Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
				if !trusted.Load() {
					t.Error("credential prompt preceded host verification")
				}
				if prompt.Title == "Unlock Private Key" {
					passphrases.Add(1)
					return []string{"correct"}, nil
				}
				passwords.Add(1)
				return []string{"one-time"}, nil
			}}
			password := func(_ ssh.ConnMetadata, value []byte) (*ssh.Permissions, error) {
				if string(value) != "one-time" {
					return nil, errors.New("wrong password")
				}
				return nil, nil
			}
			server := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				accepted := fixture.key
				if scenario == "agent-rejected" {
					accepted = fileSigner.PublicKey()
				}
				if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(accepted) {
					return nil, errors.New("fixture key rejected")
				}
				return nil, nil
			}}
			switch scenario {
			case "password-only":
				server.PublicKeyCallback = nil
				server.PasswordCallback = password
			case "no-auth":
				server.NoClientAuth = true
			case "agent-partial-password":
				server.VerifiedPublicKeyCallback = func(ssh.ConnMetadata, ssh.PublicKey, *ssh.Permissions, string) (*ssh.Permissions, error) {
					return nil, &ssh.PartialSuccessError{Next: ssh.ServerAuthCallbacks{PasswordCallback: password}}
				}
			}
			address, hostKey := authenticationServer(t, server)
			config, owner, err := nativeAuthTestConfig(t, context.Background(), Host{}, directory, "", options)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.connections.Load() != 0 || passphrases.Load() != 0 {
				t.Fatal("authentication factory accessed credentials")
			}
			config.HostKeyCallback = func(host string, address net.Addr, key ssh.PublicKey) error {
				if scenario == "reject-host" {
					return errors.New("fixture rejected host")
				}
				if err := ssh.FixedHostKey(hostKey)(host, address, key); err != nil {
					return err
				}
				trusted.Store(true)
				return nil
			}
			connection, err := net.DialTimeout("tcp", address, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			client, err := nativeSSHHandshake(context.Background(), connection, address, config, 3*time.Second, owner)
			if client != nil {
				client.Close()
			}
			if scenario == "reject-host" {
				if err == nil {
					t.Fatal("rejected host authenticated")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			wantAgent, wantPassphrase, wantPassword := int32(1), int32(0), int32(0)
			switch scenario {
			case "agent-rejected":
				wantPassphrase = 1
			case "agent-partial-password":
				wantPassword = 1
			case "password-only":
				wantAgent = 0
				wantPassword = 1
			case "no-auth", "reject-host":
				wantAgent = 0
			}
			if fixture.connections.Load() != wantAgent || passphrases.Load() != wantPassphrase || passwords.Load() != wantPassword {
				t.Fatalf("wrong credential use: agent=%d passphrase=%d password=%d", fixture.connections.Load(), passphrases.Load(), passwords.Load())
			}
			owner.Close()
		})
	}
}
