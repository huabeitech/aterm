package agentconn

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func protocolQuery(keyring agent.Agent) agentQuery {
	return func(ctx context.Context, request []byte) ([]byte, error) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		stop := context.AfterFunc(ctx, func() { client.Close(); server.Close() })
		defer stop()
		go func() { _ = agent.ServeAgent(keyring, server) }()
		if _, err := client.Write(request); err != nil {
			return nil, err
		}
		return readAgentFrame(client)
	}
}
func TestQueryConnectionSupportsPipelinedAgentAndConcurrentSigning(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err = keyring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	connection := newQueryConnection(protocolQuery(keyring))
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	if IsPageant(connection) {
		t.Fatal("generic query transport was classified as Pageant")
	}
	if !IsPageant(&pageantConnection{connection}) {
		t.Fatal("Pageant capability was lost")
	}
	client := agent.NewClient(connection)
	parsed, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	var work sync.WaitGroup
	failures := make(chan error, 16)
	for i := 0; i < 16; i++ {
		work.Add(1)
		go func() {
			defer work.Done()
			keys, err := client.List()
			if err != nil {
				failures <- err
				return
			}
			if len(keys) != 1 {
				failures <- errors.New("unexpected IPC key list")
				return
			}
			payload := []byte("Pageant protocol stream fixture")
			signature, err := client.Sign(keys[0], payload)
			if err == nil {
				err = parsed.Verify(payload, signature)
			}
			if err != nil {
				failures <- err
			}
		}()
	}
	work.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	connection.Close()
	select {
	case <-connection.done:
	case <-time.After(time.Second):
		t.Fatal("IPC worker leaked")
	}
}
func TestQueryConnectionReaderBlocksUntilResponseAndAcceptsFragmentedWrites(t *testing.T) {
	response := []byte{0, 0, 0, 1, 6}
	connection := newQueryConnection(func(context.Context, []byte) ([]byte, error) { return bytes.Clone(response), nil })
	defer connection.Close()
	read := make(chan error, 1)
	go func() {
		actual := make([]byte, len(response))
		_, err := io.ReadFull(connection, actual)
		if err == nil && !bytes.Equal(actual, response) {
			err = errors.New("IPC response differs")
		}
		read <- err
	}()
	select {
	case err := <-read:
		t.Fatal("idle reader returned before any request", err)
	case <-time.After(10 * time.Millisecond):
	}
	request := []byte{0, 0, 0, 1, 11}
	for _, value := range request {
		if _, err := connection.Write([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("fragmented request stalled")
	}
}
func TestQueryConnectionCloseAndReadDeadlineCancelQueries(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "deadline"}[timeout], func(t *testing.T) {
			started := make(chan struct{})
			connection := newQueryConnection(func(ctx context.Context, _ []byte) ([]byte, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			})
			defer connection.Close()
			if timeout {
				_ = connection.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
			}
			result := make(chan error, 1)
			go func() { _, err := agent.NewClient(connection).List(); result <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("IPC request did not begin")
			}
			if !timeout {
				connection.Close()
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("aborted request succeeded")
				}
				if timeout {
					// x/crypto/agent formats transport errors using %v, so its API
					// intentionally exposes the timeout as text rather than an unwrap.
					if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
						t.Fatal("deadline cause lost", err)
					}
				}
			case <-time.After(time.Second):
				t.Fatal("agent did not stop")
			}
			select {
			case <-connection.done:
			case <-time.After(time.Second):
				t.Fatal("canceled IPC worker leaked")
			}
		})
	}
}
func TestQueryConnectionRejectsOversizedRequestsAndMalformedResponses(t *testing.T) {
	connection := newQueryConnection(func(context.Context, []byte) ([]byte, error) {
		t.Error("oversized request reached IPC")
		return nil, nil
	})
	defer connection.Close()
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, pageantFrameLimit)
	_, _ = connection.Write(prefix)
	if _, err := connection.Read(make([]byte, 4)); err == nil {
		t.Fatal("oversized request accepted")
	}
	for _, response := range [][]byte{nil, {0, 0, 0, 0}, {0, 0, 0, 4, 6}, make([]byte, pageantFrameLimit+1)} {
		connection := newQueryConnection(func(context.Context, []byte) ([]byte, error) { return bytes.Clone(response), nil })
		client := agent.NewClient(connection)
		if _, err := client.List(); err == nil {
			t.Fatal("malformed response accepted")
		}
		connection.Close()
		select {
		case <-connection.done:
		case <-time.After(time.Second):
			t.Fatal("failed worker leaked")
		}
	}
}

func TestQueryConnectionRawReadRetainsTimeoutType(t *testing.T) {
	connection := newQueryConnection(func(ctx context.Context, _ []byte) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() })
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	if _, err := connection.Write([]byte{0, 0, 0, 1, 11}); err != nil {
		t.Fatal(err)
	}
	_, err := connection.Read(make([]byte, 4))
	var deadline net.Error
	if !errors.As(err, &deadline) || !deadline.Timeout() {
		t.Fatal("raw transport lost timeout type", err)
	}
}
