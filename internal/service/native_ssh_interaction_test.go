package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func authenticationServer(t *testing.T, config *ssh.ServerConfig) (string, ssh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		server, channels, requests, err := ssh.NewServerConn(connection, config)
		if err != nil {
			connection.Close()
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for channel := range channels {
			channel.Reject(ssh.Prohibited, "test server has no shell")
		}
	}()
	return listener.Addr().String(), signer.PublicKey()
}
func TestNativeSSHOneTimePasswordIsRequestedAndNotPersisted(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	var asked atomic.Int32
	config, owner, err := nativeAuthTestConfig(t, context.Background(), Host{HostName: "test", User: "fixture"}, t.TempDir(), "", NativeSSHAuthOptions{Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
		asked.Add(1)
		if len(prompt.Fields) != 1 || !prompt.Fields[0].Secret {
			t.Error("password field is not secret")
		}
		return []string{"one-time-password"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	address, key := authenticationServer(t, &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "one-time-password" {
			return nil, errors.New("wrong password")
		}
		return &ssh.Permissions{}, nil
	}})
	config.HostKeyCallback = ssh.FixedHostKey(key)
	config.Timeout = 3 * time.Second
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if asked.Load() != 1 {
		t.Fatalf("password prompts: %d", asked.Load())
	}
}
func TestNativeSSHKeyboardInteractivePreservesFieldSecrecy(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	var received atomic.Bool
	config, _, err := nativeAuthTestConfig(t, context.Background(), Host{}, t.TempDir(), "", NativeSSHAuthOptions{Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
		if len(prompt.Fields) != 2 || prompt.Fields[0].Secret || !prompt.Fields[1].Secret {
			t.Errorf("interactive fields: %#v", prompt.Fields)
		}
		received.Store(true)
		return []string{"fixture", "123456"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	address, key := authenticationServer(t, &ssh.ServerConfig{KeyboardInteractiveCallback: func(user ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		answers, err := challenge("fixture", "Enter your login and token", []string{"Login", "Verification code"}, []bool{true, false})
		if err != nil {
			return nil, err
		}
		if len(answers) != 2 || answers[0] != "fixture" || answers[1] != "123456" {
			return nil, errors.New("incorrect challenge response")
		}
		return &ssh.Permissions{}, nil
	}})
	config.HostKeyCallback = ssh.FixedHostKey(key)
	config.Timeout = 3 * time.Second
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if !received.Load() {
		t.Fatal("keyboard-interactive prompt was not used")
	}
}
func TestNativeSSHManagedEncryptedIdentityPromptsOnceAndAuthenticates(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "managed fixture", []byte("correct passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	material := pem.EncodeToMemory(block)
	defer clear(material)
	var prompts int
	config, _, err := nativeAuthTestConfig(t, context.Background(), Host{IdentityFile: "aterm-key://fixture"}, t.TempDir(), "", NativeSSHAuthOptions{
		ReadIdentity: func(identity string) ([]byte, error) {
			if identity != "aterm-key://fixture" {
				t.Fatal("wrong managed identity")
			}
			return append([]byte{}, material...), nil
		},
		Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
			prompts++
			if len(prompt.Fields) != 1 || !prompt.Fields[0].Secret {
				t.Fatal("passphrase is not secret")
			}
			return []string{"correct passphrase"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := ssh.NewPublicKey(public)
	address, key := authenticationServer(t, &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(expected) {
			return nil, errors.New("unexpected identity")
		}
		return &ssh.Permissions{}, nil
	}})
	config.HostKeyCallback = ssh.FixedHostKey(key)
	config.Timeout = 3 * time.Second
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if prompts != 1 {
		t.Fatalf("passphrase attempts: %d", prompts)
	}
}
func TestNativeSSHCancelledPassphraseDoesNotProceedToAuthentication(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "fixture", []byte("passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	material := pem.EncodeToMemory(block)
	err = dialNativeAuthTest(t, Host{IdentityFile: "aterm-key://fixture"}, t.TempDir(), "", NativeSSHAuthOptions{ReadIdentity: func(string) ([]byte, error) { return append([]byte{}, material...), nil }, Prompt: func(context.Context, CredentialPrompt) ([]string, error) { return nil, context.Canceled }}, &ssh.ServerConfig{PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
		return nil, errors.New("no key allowed")
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
