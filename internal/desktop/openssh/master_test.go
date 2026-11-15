package openssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestRealOpenSSHMasterSharesAuthenticationAndExpiresFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix OpenSSH integration")
	}
	if _, err := os.Stat("/usr/bin/ssh"); err != nil {
		t.Skip("system OpenSSH unavailable")
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	endShell := make(chan struct{})
	var connections atomic.Int32
	handlers := sftp.InMemHandler()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			go func() {
				server, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					connection.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						incoming.Reject(ssh.UnknownChannelType, "session required")
						continue
					}
					channel, requests, err := incoming.Accept()
					if err != nil {
						return
					}
					go func() {
						defer channel.Close()
						for request := range requests {
							switch request.Type {
							case "pty-req", "env", "window-change":
								request.Reply(true, nil)
							case "shell":
								request.Reply(true, nil)
								io.WriteString(channel, "fixture shell\r\n")
								select {
								case <-endShell:
								case <-ctx.Done():
								}
								channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								return
							case "subsystem":
								var name struct{ Name string }
								ssh.Unmarshal(request.Payload, &name)
								request.Reply(name.Name == "sftp", nil)
								if name.Name == "sftp" {
									server := sftp.NewRequestServer(channel, handlers)
									_ = server.Serve()
									server.Close()
								}
								return
							default:
								request.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	integration, err := New("/usr/bin/ssh")
	if err != nil {
		t.Fatal(err)
	}
	defer integration.Close()
	known := filepath.Join(t.TempDir(), "known_hosts")
	address := listener.Addr().String()
	if err = os.WriteFile(known, []byte(knownhosts.Line([]string{address}, signer.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Public-key credentials are not needed by this isolated no-auth server, but
	// prevent the client from consulting the user's key files or agent.
	block, err := ssh.MarshalPrivateKey(private, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(t.TempDir(), "identity")
	os.WriteFile(identity, pem.EncodeToMemory(block), 0600)
	_, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	command := exec.CommandContext(ctx, filepath.Join(integration.directory, "ssh"), "-F", "/dev/null", "-tt", "-p", portText, "-i", identity, "-o", "IdentitiesOnly=yes", "-o", "UserKnownHostsFile="+known, "-o", "StrictHostKeyChecking=yes", "-o", "GlobalKnownHostsFile=/dev/null", "fixture@127.0.0.1")
	command.Env = append(os.Environ(), "SSH_AUTH_SOCK=")
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	events := make(chan *RemoteContext, 8)
	integration.Start(func(remote *RemoteContext) { events <- remote })
	var remote *RemoteContext
	select {
	case remote = <-events:
	case <-ctx.Done():
		t.Fatal("OpenSSH master was not discovered")
	}
	if remote == nil || remote.Host != "127.0.0.1" || remote.User != "fixture" || remote.Port != port {
		t.Fatal("incorrect SSH context", remote)
	}
	client, owner, err := integration.OpenFiles(ctx, remote.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	defer client.Close()
	if err = client.Mkdir("/master-reused"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Stat("/master-reused"); err != nil {
		t.Fatal(err)
	}
	if connections.Load() != 1 {
		t.Fatal("SFTP opened another SSH transport", connections.Load())
	}
	close(endShell)
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal("foreground SSH did not exit cleanly", err)
		}
	case <-ctx.Done():
		t.Fatal("file channel retained foreground SSH after shell exit")
	}
	select {
	case remote = <-events:
		if remote != nil {
			t.Fatal("expired master remained selected")
		}
	case <-ctx.Done():
		t.Fatal("SSH context did not expire")
	}
	if _, _, err = integration.OpenFiles(ctx, "c.expired"); err == nil {
		t.Fatal("stale workspace reopened")
	}
	failed := make(chan error, 1)
	go func() { _, err := client.Getwd(); failed <- err }()
	select {
	case err = <-failed:
		if err == nil {
			t.Fatal("file channel survived SSH shell exit")
		}
	case <-ctx.Done():
		t.Fatal("file channel did not disconnect")
	}
}
