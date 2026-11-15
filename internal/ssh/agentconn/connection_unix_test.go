//go:build !windows

package agentconn

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestUnixAgentProtocolAndConnectionOwnership(t *testing.T) {
	directory, err := os.MkdirTemp("", "aag-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err = keyring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			served <- err
			return
		}
		defer connection.Close()
		served <- agent.ServeAgent(keyring, connection)
	}()
	t.Setenv("SSH_AUTH_SOCK", socket)
	connection, err := Dial(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	// Dial's internal discovery context is already canceled on return. The
	// caller-owned connection must still work until the caller closes it.
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	client := agent.NewClient(connection)
	keys, err := client.List()
	if err != nil || len(keys) != 1 {
		t.Fatal("agent discovery failed", err)
	}
	payload := []byte("aTerm shared agent transport")
	signature, err := client.Sign(keys[0], payload)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ssh.NewPublicKey(public)
	if err != nil || parsed.Verify(payload, signature) != nil {
		t.Fatal("signature differs", err)
	}
	connection.Close()
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("agent connection leaked")
	}
}
func TestUnixNoAgentReturnsExplicitError(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if _, err := Dial(context.Background(), ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
