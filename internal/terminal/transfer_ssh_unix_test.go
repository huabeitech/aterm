//go:build !windows

package terminal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
	"ssh-connect/internal/desktop/zmodem"
	"ssh-connect/internal/service"
)

// The remote tool is an independent lrzsz process behind an authenticated SSH
// channel. No user's sshd, hosts, agent or files participate in this fixture.
func zmodemSSHPeer(t *testing.T, ctx context.Context, command *exec.Cmd, clientKey ssh.PublicKey) string {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(key.Marshal(), clientKey.Marshal()) {
			return nil, nil
		}
		return nil, fmt.Errorf("unknown fixture key")
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stopListener := context.AfterFunc(ctx, func() { listener.Close() })
	t.Cleanup(func() { stopListener(); listener.Close() })
	done := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("SSH protocol peer did not stop")
		}
	})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		stop := context.AfterFunc(ctx, func() { connection.Close() })
		defer stop()
		server, channels, requests, err := ssh.NewServerConn(connection, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			if incoming.ChannelType() != "session" {
				_ = incoming.Reject(ssh.UnknownChannelType, "session required")
				continue
			}
			channel, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			// A session is used only once in this fixture; shell is a fixed tool, not a
			// shell command constructed from untrusted request payloads.
			for request := range requests {
				switch request.Type {
				case "pty-req", "env", "window-change":
					_ = request.Reply(true, nil)
				case "shell":
					terminal, err := pty.Start(command)
					if err != nil {
						_ = request.Reply(false, nil)
						channel.Close()
						return
					}
					_ = request.Reply(true, nil)
					go func() { _, _ = io.Copy(terminal, channel); terminal.Close() }()
					go ssh.DiscardRequests(requests)
					// Drain the real remote PTY before publishing exit, including the final
					// OO/CRC bytes left in its kernel buffer after the tool exits.
					_, _ = io.Copy(channel, terminal)

					err = command.Wait()
					terminal.Close()
					status := uint32(0)
					if err != nil {
						status = 1
					}
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
					channel.Close()
					return
				default:
					_ = request.Reply(false, nil)
				}
			}
			channel.Close()
		}
	}()
	return listener.Addr().String()
}

func TestTerminalZmodemLrzszOverNativeSSH(t *testing.T) {
	tools := os.Getenv("ATERM_LRZSZ_DIR")
	if tools == "" {
		t.Skip("set ATERM_LRZSZ_DIR for native SSH/lrzsz acceptance")
	}
	t.Setenv("SSH_AUTH_SOCK", "")
	for _, upload := range []bool{false, true} {
		t.Run(map[bool]string{false: "download", true: "upload"}[upload], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			directory := t.TempDir()
			incoming := filepath.Join(directory, "received")
			if err := os.Mkdir(incoming, 0700); err != nil {
				t.Fatal(err)
			}
			payload := make([]byte, 2*1024*1024+37)
			for i := range payload {
				payload[i] = byte(i)
			}
			source := filepath.Join(directory, "SSH 中文.bin")
			if err := os.WriteFile(source, payload, 0600); err != nil {
				t.Fatal(err)
			}
			tool := "lsz"
			args := []string{"--binary", "--quiet", "--timeout", "20", source}
			if upload {
				tool = "lrz"
				args = []string{"--binary", "--quiet", "--timeout", "20"}
			}
			command := exec.CommandContext(ctx, filepath.Join(tools, tool), args...)
			command.Dir = incoming
			_, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			clientSigner, err := ssh.NewSignerFromKey(clientPrivate)
			if err != nil {
				t.Fatal(err)
			}
			keyBlock, err := ssh.MarshalPrivateKey(clientPrivate, "disposable ZMODEM fixture")
			if err != nil {
				t.Fatal(err)
			}
			identity := filepath.Join(directory, "client.key")
			if err = os.WriteFile(identity, pem.EncodeToMemory(keyBlock), 0600); err != nil {
				t.Fatal(err)
			}
			address := zmodemSSHPeer(t, ctx, command, clientSigner.PublicKey())
			hostname, portString, err := net.SplitHostPort(address)
			if err != nil {
				t.Fatal(err)
			}
			port, _ := strconv.Atoi(portString)
			svc := sshServiceFixture(t)
			alias := "zmodem-fixture"
			_, err = svc.CreateHost(ctx, service.HostInput{Name: alias, Alias: alias, HostName: hostname, Port: port, User: "fixture", IdentityFile: identity})
			if err != nil {
				t.Fatal(err)
			}
			var progressMu sync.Mutex
			var last zmodem.Progress
			manager := NewManager(ctx, func(name string, value any) {
				if name != "terminal:event" {
					return
				}
				if event, ok := value.(map[string]any); ok && event["type"] == "zmodem" {
					progressMu.Lock()
					last = event["progress"].(zmodem.Progress)
					progressMu.Unlock()
				}
			})
			defer manager.CloseAll()
			manager.upload = func(ctx context.Context, _ string) ([]zmodem.Upload, error) {
				return zmodem.OpenUploads(ctx, []string{source})
			}
			manager.download = func(ctx context.Context, _ string, offer zmodem.FileOffer) (zmodem.Download, error) {
				return zmodem.NewDownload(ctx, filepath.Join(incoming, offer.Name))
			}
			entry, err := manager.OpenSSHAsync(alias, func(ctx context.Context, _ service.HostKeyConfirmer) (*service.NativeSSHClient, string, error) {
				client, host, err := svc.OpenNativeSSHWithHostKeyConfirmer(ctx, alias, func(context.Context, service.HostKeyInfo) (bool, error) { return true, nil })
				return client, host.Alias, err
			})
			if err != nil {
				t.Fatal(err)
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				manager.mu.RLock()
				session := manager.sessions[entry.ID]
				state := session.State
				replay := bytes.Clone(session.output)
				manager.mu.RUnlock()
				progressMu.Lock()
				progress := last
				progressMu.Unlock()
				if state == "exited" && progress.State == "completed" {
					received, err := os.ReadFile(filepath.Join(incoming, filepath.Base(source)))
					if err != nil || !bytes.Equal(received, payload) {
						t.Fatal("SSH transfer differs", err)
					}
					if bytes.Contains(replay, []byte{zmodem.ZDLE}) || bytes.Contains(replay, []byte{0, 1, 2, 3}) {
						t.Fatalf("SSH protocol leaked to canonical replay: %x", replay)
					}
					if progress.Transferred != int64(len(payload)) {
						t.Fatal("SSH transfer progress differs", progress)
					}
					break
				}
				if progress.State == "failed" || (state == "exited" && progress.State == "") {
					t.Fatalf("SSH transfer failed: %+v, state=%s replay=%q", progress, state, replay)
				}
				select {
				case <-ctx.Done():
					t.Fatal(fmt.Sprintf("SSH transfer incomplete: %v state=%s progress=%+v replay=%q", ctx.Err(), state, progress, replay))
				case <-ticker.C:
				}
			}
		})
	}
}
