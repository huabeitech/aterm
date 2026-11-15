package terminalmeta

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func metadataSSH(t *testing.T, respond func(ssh.Channel, *ssh.Request)) *ssh.Client {
	t.Helper()
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
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			go func() {
				defer channel.Close()
				for request := range requests {
					respond(channel, request)
				}
			}()
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}
func TestRemoteDirectoryUsesIndependentExecOnExistingSSHConnection(t *testing.T) {
	client := metadataSSH(t, func(channel ssh.Channel, request *ssh.Request) {
		var value struct{ Command string }
		if err := ssh.Unmarshal(request.Payload, &value); err != nil || request.Type != "exec" || value.Command != RemoteDirectoryCommand {
			t.Error("unexpected metadata operation", request.Type, value.Command)
			request.Reply(false, nil)
			return
		}
		request.Reply(true, nil)
		channel.Write([]byte("/work/space 中文\n"))
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
		channel.Close()
	})
	if got := ReadRemoteDirectory(context.Background(), client); got != "/work/space 中文" {
		t.Fatalf("remote directory %q", got)
	}
	// Only the probe channel closes; the authenticated parent stays usable.
	session, err := client.NewSession()
	if err != nil {
		t.Fatal("metadata probe closed parent connection", err)
	}
	session.Close()
}
func TestRemoteDirectoryProbeTimeoutAndOutputBound(t *testing.T) {
	stalled := metadataSSH(t, func(channel ssh.Channel, request *ssh.Request) {})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if got := ReadRemoteDirectory(ctx, stalled); got != "" || time.Since(start) > time.Second {
		t.Fatal("stalled metadata probe did not fall back promptly", got)
	}
	oversized := metadataSSH(t, func(channel ssh.Channel, request *ssh.Request) {
		request.Reply(true, nil)
		channel.Write([]byte("/" + strings.Repeat("x", 70*1024)))
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
		channel.Close()
	})
	if got := ReadRemoteDirectory(context.Background(), oversized); got != "" {
		t.Fatal("oversized remote metadata accepted")
	}
}
