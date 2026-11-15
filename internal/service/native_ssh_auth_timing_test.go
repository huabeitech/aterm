package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestNativeSSHCredentialsFollowHostVerificationAndServerMethods(t *testing.T) {
	material := encryptedIdentityFixture(t)
	for _, scenario := range []string{"reject-host", "key", "password-only", "no-auth"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("SSH_AUTH_SOCK", "")
			var mu sync.Mutex
			var events []string
			record := func(value string) { mu.Lock(); events = append(events, value); mu.Unlock() }
			options := NativeSSHAuthOptions{
				HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error {
					record("host")
					if scenario == "reject-host" {
						return errors.New("fixture host rejected")
					}
					return nil
				},
				ReadIdentity: func(string) ([]byte, error) { record("read"); return append([]byte(nil), material...), nil },
				Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
					if prompt.Title == "Unlock Private Key" {
						record("passphrase")
						return []string{"correct"}, nil
					}
					record("password")
					return []string{"one-time"}, nil
				},
			}
			server := &ssh.ServerConfig{}
			switch scenario {
			case "password-only":
				server.PasswordCallback = func(_ ssh.ConnMetadata, value []byte) (*ssh.Permissions, error) {
					if string(value) != "one-time" {
						return nil, errors.New("wrong password")
					}
					return nil, nil
				}
			case "no-auth":
				server.NoClientAuth = true
			default:
				server.PublicKeyCallback = func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) { record("offer"); return nil, nil }
			}
			err := dialNativeAuthTest(t, Host{IdentityFile: "aterm-key://fixture"}, t.TempDir(), "", options, server)
			if scenario == "reject-host" {
				if err == nil {
					t.Fatal("rejected host authenticated")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			got := append([]string(nil), events...)
			mu.Unlock()
			want := []string{"host"}
			switch scenario {
			case "key":
				want = append(want, "read", "passphrase", "offer")
			case "password-only":
				want = append(want, "password")
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("credential order: got %v want %v", got, want)
			}
		})
	}
}

func TestNativeSSHDefaultKeysSupportTwoPublicKeyFactors(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing-agent.sock"))
	directory := t.TempDir()
	_, first, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	firstSigner, _ := ssh.NewSignerFromKey(first)
	secondSigner, _ := ssh.NewSignerFromKey(second)
	for name, key := range map[string]any{"id_ed25519": first, "id_ecdsa": second} {
		block, err := ssh.MarshalPrivateKey(key, "disposable multi-key fixture")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), pem.EncodeToMemory(block), 0600); err != nil {
			t.Fatal(err)
		}
	}
	server := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(firstSigner.PublicKey()) {
			return nil, errors.New("first factor key required")
		}
		return nil, nil
	}}
	server.VerifiedPublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey, permissions *ssh.Permissions, _ string) (*ssh.Permissions, error) {
		if ssh.FingerprintSHA256(key) == ssh.FingerprintSHA256(firstSigner.PublicKey()) {
			return nil, &ssh.PartialSuccessError{Next: ssh.ServerAuthCallbacks{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(secondSigner.PublicKey()) {
					return nil, errors.New("distinct second key required")
				}
				return nil, nil
			}}}
		}
		return permissions, nil
	}
	if err := dialNativeAuthTest(t, Host{}, directory, "", NativeSSHAuthOptions{}, server); err != nil {
		t.Fatal(err)
	}
}

func TestNativeSSHKeyboardInteractiveDistinguishesPasswordAndVerification(t *testing.T) {
	for _, question := range []string{"Password:", "fixture's password:", "fixture’s password:", "user@host's password:   ", "One-time password:", "One time password:", "Verification code:", "Password for root:", ""} {
		t.Run(question, func(t *testing.T) {
			var prompted bool
			automatic := question == "Password:" || question == "fixture's password:" || question == "fixture’s password:" || question == "user@host's password:   " || question == "One-time password:"
			options := NativeSSHAuthOptions{Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
				prompted = true
				if automatic || question == "" {
					t.Error("unnecessary challenge prompt")
				}
				if prompt.Title != "Authenticate Fixture" || prompt.Message != "fixture@fixture.invalid" || len(prompt.Fields) != 1 || prompt.Fields[0].Label != question || !prompt.Fields[0].Secret {
					t.Error("wrong interactive prompt", prompt)
				}
				return []string{"123456"}, nil
			}}
			server := &ssh.ServerConfig{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
				questions := []string{question}
				echo := []bool{false}
				if question == "" {
					questions = nil
					echo = nil
				}
				answers, err := challenge("fixture", "", questions, echo)
				if err != nil {
					return nil, err
				}
				if question == "" {
					if len(answers) != 0 {
						return nil, errors.New("unexpected response to empty challenge")
					}
					return nil, nil
				}
				want := "123456"
				if automatic {
					want = "saved-password"
				}
				if !reflect.DeepEqual(answers, []string{want}) {
					return nil, errors.New("wrong challenge answer")
				}
				return nil, nil
			}}
			if err := dialNativeAuthTest(t, Host{Alias: "Fixture", HostName: "fixture.invalid", PasswordAuth: true}, t.TempDir(), "saved-password", options, server); err != nil {
				t.Fatal(err)
			}
			if prompted != (!automatic && question != "") {
				t.Fatal("unexpected prompt count", prompted)
			}
		})
	}
}

func TestNativeSSHHandshakeTimeoutCancelsDeferredCredentialPrompt(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	material := encryptedIdentityFixture(t)
	prompted := make(chan struct{})
	options := NativeSSHAuthOptions{
		ReadIdentity: func(string) ([]byte, error) { return append([]byte(nil), material...), nil },
		Prompt: func(ctx context.Context, _ CredentialPrompt) ([]string, error) {
			close(prompted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	address, key := authenticationServer(t, &ssh.ServerConfig{PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) { return nil, nil }})
	config, owner, err := nativeAuthTestConfig(t, context.Background(), Host{IdentityFile: "aterm-key://fixture"}, t.TempDir(), "", options)
	if err != nil {
		t.Fatal(err)
	}
	config.HostKeyCallback = ssh.FixedHostKey(key)
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	client, err := nativeSSHHandshake(context.Background(), connection, address, config, 50*time.Millisecond, owner)
	if client != nil {
		client.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("handshake timeout did not cancel credential wait", err)
	}
	select {
	case <-prompted:
	default:
		t.Fatal("test did not reach deferred credential prompt")
	}
	if time.Since(started) > time.Second {
		t.Fatal("credential prompt ignored handshake cancellation")
	}
}
