package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestNativeSSHHandshakeCancellationAndTimeout(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		local, peer := net.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		start := time.Now()
		_, err := nativeSSHHandshake(ctx, local, "fixture", &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.InsecureIgnoreHostKey()}, 25*time.Millisecond)
		cancel()
		peer.Close()
		expected := error(context.DeadlineExceeded)
		if cancelled {
			expected = context.Canceled
		}
		if !errors.Is(err, expected) {
			t.Fatalf("handshake error %v, expected %v", err, expected)
		}
		if time.Since(start) > time.Second {
			t.Fatal("handshake ignored cancellation")
		}
	}
}

func TestNativeSSHAgentForwardingRequiresExplicitConfiguration(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if conn, err := nativeSSHAgentForwarding(context.Background(), nil, Host{}); err != nil || conn != nil {
		t.Fatal("disabled forwarding touched the agent", err)
	}
	if _, err := nativeSSHAgentForwarding(context.Background(), nil, Host{ForwardAgent: true}); err == nil {
		t.Fatal("missing agent was accepted")
	}
}

func TestNativeSSHAgentForwardingSignsThroughRemoteChannel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix agent socket fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "aterm-agent-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	socket := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("SSH_AUTH_SOCK", socket)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err = keyring.Add(agent.AddedKey{PrivateKey: private, Comment: "forwarding fixture"}); err != nil {
		t.Fatal(err)
	}
	agentDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			agentDone <- err
			return
		}
		defer connection.Close()
		agentDone <- agent.ServeAgent(keyring, connection)
	}()

	hostSigner, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(hostSigner)
	serverListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverListener.Close()
	verified := make(chan error, 1)
	go func() {
		connection, err := serverListener.Accept()
		if err != nil {
			verified <- err
			return
		}
		server, channels, requests, err := ssh.NewServerConn(connection, serverConfig)
		if err != nil {
			connection.Close()
			verified <- err
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		incoming, ok := <-channels
		if !ok {
			verified <- errors.New("session channel missing")
			return
		}
		if incoming.ChannelType() != "session" {
			incoming.Reject(ssh.UnknownChannelType, "session required")
			verified <- errors.New("unexpected channel")
			return
		}
		session, sessionRequests, err := incoming.Accept()
		if err != nil {
			verified <- err
			return
		}
		defer session.Close()
		request, ok := <-sessionRequests
		if !ok || request.Type != "auth-agent-req@openssh.com" {
			verified <- errors.New("forwarding request missing")
			return
		}
		request.Reply(true, nil)
		forwarded, forwardRequests, err := server.OpenChannel("auth-agent@openssh.com", nil)
		if err != nil {
			verified <- err
			return
		}
		defer forwarded.Close()
		go ssh.DiscardRequests(forwardRequests)
		remote := agent.NewClient(forwarded)
		keys, err := remote.List()
		if err != nil {
			verified <- err
			return
		}
		if len(keys) != 1 {
			verified <- errors.New("unexpected forwarded key count")
			return
		}
		payload := []byte("aterm forwarding protocol fixture")
		signature, err := remote.Sign(keys[0], payload)
		if err != nil {
			verified <- err
			return
		}
		verified <- hostSigner.PublicKey().Verify(payload, signature)
		// Keep the remote connection open until the native client closes it.
		server.Wait()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", serverListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := nativeSSHHandshake(ctx, connection, serverListener.Addr().String(), &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.FixedHostKey(hostSigner.PublicKey())}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	forwarding, err := nativeSSHAgentForwarding(ctx, raw, Host{ForwardAgent: true})
	if err != nil {
		raw.Close()
		t.Fatal(err)
	}
	client := newNativeSSHClient(raw, Host{}, nil, forwarding)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case err = <-verified:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("forwarded signing did not complete")
	}
	client.Close()
	select {
	case <-agentDone:
	case <-ctx.Done():
		t.Fatal("forwarding agent connection leaked after SSH close")
	}
}

func TestNativeSSHConnectTimeoutCancelsHostKeyPrompt(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	address, _ := authenticationServer(t, &ssh.ServerConfig{NoClientAuth: true})
	hostname, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	app := &AppService{}
	asked := false
	start := time.Now()
	_, err = app.dialNativeSSH(context.Background(), Host{Alias: "timeout", HostName: hostname, Port: port, ConnectTimeout: 1}, t.TempDir(), func(ctx context.Context, _ HostKeyInfo) (bool, error) {
		asked = true
		<-ctx.Done()
		return false, ctx.Err()
	}, 0, NativeSSHAuthOptions{Prompt: func(context.Context, CredentialPrompt) ([]string, error) { return []string{"fixture"}, nil }})
	if !asked || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("host-key prompt ignored connection timeout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("connection timeout took %s", elapsed)
	}
}

func TestNativeSSHAgentDiscoveryHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix agent socket fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "aterm-agent-stall-")
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
	t.Setenv("SSH_AUTH_SOCK", socket)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	plan, err := newNativeSSHAuthPlan(ctx, Host{}, t.TempDir(), "", NativeSSHAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	_, err = plan.next(&ssh.ClientAuthContext{AllowedMethods: []string{"publickey"}})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled agent discovery: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("unresponsive agent ignored cancellation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("agent discovery socket leaked")
	}
}

func TestNativeSSHAgentForwardingDenialPreservesConnection(t *testing.T) {
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
		go func() {
			for request := range requests {
				request.Reply(true, nil)
			}
		}()
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			go func() {
				defer channel.Close()
				for request := range requests {
					request.Reply(false, nil)
				}
			}()
		}
	}()
	raw, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	forwarding, peer := net.Pipe()
	defer peer.Close()
	client := newNativeSSHClient(raw, Host{}, nil, forwarding)
	defer client.Close()
	if _, err = client.NewSession(); err == nil || !strings.Contains(err.Error(), "request SSH agent forwarding") {
		t.Fatalf("forwarding rejection was not reported: %v", err)
	}
	ping := make(chan error, 1)
	go func() {
		ok, _, err := raw.SendRequest("keepalive@openssh.com", true, nil)
		if !ok && err == nil {
			err = errors.New("connection no longer responds")
		}
		ping <- err
	}()
	select {
	case err = <-ping:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("session rejection disrupted its parent SSH connection")
	}
}
