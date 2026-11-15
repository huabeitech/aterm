//go:build !windows

package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestNativeSSHKeySourcesFollowElectronOrderAndFallback(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		for _, acceptFile := range []bool{true, false} {
			t.Run(fmt.Sprintf("explicit=%v/fileAccepted=%v", explicit, acceptFile), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				directory, err := os.MkdirTemp("/tmp", "aterm-key-order-")
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(directory)
				_, agentPrivate, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				_, filePrivate, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				agentSigner, err := ssh.NewSignerFromKey(agentPrivate)
				if err != nil {
					t.Fatal(err)
				}
				fileSigner, err := ssh.NewSignerFromKey(filePrivate)
				if err != nil {
					t.Fatal(err)
				}
				block, err := ssh.MarshalPrivateKey(filePrivate, "disposable key-order fixture")
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(directory, "id_ed25519")
				if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
					t.Fatal(err)
				}
				socket := filepath.Join(directory, "agent.sock")
				listener, err := net.Listen("unix", socket)
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				t.Setenv("SSH_AUTH_SOCK", socket)
				keyring := agent.NewKeyring()
				if err := keyring.Add(agent.AddedKey{PrivateKey: agentPrivate}); err != nil {
					t.Fatal(err)
				}
				var agentConnections atomic.Int32
				agentDone := make(chan struct{})
				go func() {
					defer close(agentDone)
					connection, err := listener.Accept()
					if err != nil {
						return
					}
					agentConnections.Add(1)
					defer connection.Close()
					_ = agent.ServeAgent(keyring, connection)
				}()
				host := Host{}
				if explicit {
					host.IdentityFile = path
				}
				clientConfig, agentConnection, err := nativeAuthTestConfig(t, ctx, host, directory, "", NativeSSHAuthOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if agentConnection == nil {
					t.Fatal("authentication resource owner missing")
				}
				defer agentConnection.Close()
				if agentConnections.Load() != 0 {
					t.Fatal("agent contacted during configuration")
				}
				accepted := agentSigner.PublicKey()
				if acceptFile {
					accepted = fileSigner.PublicKey()
				}
				expectedFirst := agentSigner.PublicKey()
				if explicit {
					expectedFirst = fileSigner.PublicKey()
				}
				var first string
				config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
					if first == "" {
						first = ssh.FingerprintSHA256(key)
					}
					if bytes.Equal(key.Marshal(), accepted.Marshal()) {
						return nil, nil
					}
					return nil, fmt.Errorf("fixture key not accepted")
				}}
				config.AddHostKey(fileSigner)
				serverListener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer serverListener.Close()
				type result struct {
					err   error
					first string
				}
				serverDone := make(chan result, 1)
				go func() {
					connection, err := serverListener.Accept()
					if err != nil {
						serverDone <- result{err: err}
						return
					}
					defer connection.Close()
					connection.SetDeadline(time.Now().Add(5 * time.Second))
					server, _, _, err := ssh.NewServerConn(connection, config)
					if server != nil {
						server.Close()
					}
					serverDone <- result{err: err, first: first}
				}()
				connection, err := net.DialTimeout("tcp", serverListener.Addr().String(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				client, err := nativeSSHHandshake(ctx, connection, serverListener.Addr().String(), clientConfig, 2*time.Second, agentConnection)
				if client != nil {
					client.Close()
				}
				if err != nil {
					t.Errorf("authorized key source was not reached: %v", err)
				}
				outcome := <-serverDone
				if outcome.err != nil {
					t.Errorf("server authentication: %v", outcome.err)
				}
				if outcome.first != ssh.FingerprintSHA256(expectedFirst) {
					t.Errorf("wrong first key: explicit=%v", explicit)
				}
				if explicit && acceptFile && agentConnections.Load() != 0 {
					t.Error("accepted explicit file still contacted agent")
				}
				agentConnection.Close()
				listener.Close() // An explicit accepted file must never contact the agent.
				select {
				case <-agentDone:
				case <-ctx.Done():
					t.Error("agent connection leaked")
				}
			})
		}
	}
}
