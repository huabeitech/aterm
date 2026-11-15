package agentconn

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Uses a private pipe, never the user's Windows OpenSSH agent or system pipe.
func TestWindowsNamedPipeAgentProtocol(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\aterm-agent-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := winio.ListenPipe(endpoint, &winio.PipeConfig{})
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
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		done <- agent.ServeAgent(keyring, connection)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, err := Dial(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	client := agent.NewClient(connection)
	keys, err := client.List()
	if err != nil || len(keys) != 1 {
		t.Fatal("named-pipe discovery failed", err)
	}
	payload := []byte("Windows named pipe fixture")
	signature, err := client.Sign(keys[0], payload)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ssh.NewPublicKey(public)
	if err != nil || parsed.Verify(payload, signature) != nil {
		t.Fatal("named-pipe signature differs", err)
	}
	connection.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("named-pipe connection leaked")
	}
}
