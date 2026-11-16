package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"ssh-connect/internal/domain"
)

type sshFixture struct {
	address     string
	connections atomic.Int32
	deny        atomic.Bool
	mu          sync.Mutex
	sockets     []net.Conn
}

func newSSHFixture(t *testing.T) *sshFixture {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &sshFixture{address: listener.Addr().String()}
	t.Cleanup(func() { listener.Close(); fixture.disconnect() })
	go func() {
		for {
			socket, err := listener.Accept()
			if err != nil {
				return
			}
			fixture.mu.Lock()
			fixture.sockets = append(fixture.sockets, socket)
			fixture.mu.Unlock()
			go func() {
				server, channels, requests, err := ssh.NewServerConn(socket, config)
				if err != nil {
					socket.Close()
					return
				}
				fixture.connections.Add(1)
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "direct-tcpip" || fixture.deny.Load() {
						channel.Reject(ssh.Prohibited, "TCP forwarding denied")
						continue
					}
					var destination struct {
						Host       string
						Port       uint32
						Origin     string
						OriginPort uint32
					}
					if err := ssh.Unmarshal(channel.ExtraData(), &destination); err != nil {
						channel.Reject(ssh.ConnectionFailed, "invalid target")
						continue
					}
					remote, err := net.DialTimeout("tcp", net.JoinHostPort(destination.Host, strconv.Itoa(int(destination.Port))), time.Second)
					if err != nil {
						channel.Reject(ssh.ConnectionFailed, err.Error())
						continue
					}
					stream, requests, err := channel.Accept()
					if err != nil {
						remote.Close()
						continue
					}
					go ssh.DiscardRequests(requests)
					go func() {
						defer remote.Close()
						defer stream.Close()
						done := make(chan struct{})
						go func() { io.Copy(remote, stream); remote.(*net.TCPConn).CloseWrite(); close(done) }()
						io.Copy(stream, remote)
						stream.CloseWrite()
						<-done
					}()
				}
			}()
		}
	}()
	return fixture
}

func (f *sshFixture) disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, socket := range f.sockets {
		socket.Close()
	}
}

func (f *sshFixture) connect(ctx context.Context) (Transport, error) {
	socket, err := (&net.Dialer{}).DialContext(ctx, "tcp", f.address)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { socket.Close() })
	defer stop()
	connection, channels, requests, err := ssh.NewClientConn(socket, f.address, &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.InsecureIgnoreHostKey()})
	if err != nil {
		socket.Close()
		return nil, err
	}
	return ssh.NewClient(connection, channels, requests), nil
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}
func configFor(t *testing.T, targetPort int) domain.Tunnel {
	return domain.Tunnel{ID: "fixture", TunnelInput: domain.TunnelInput{Name: "Fixture service", HostID: "host", LocalAddress: "127.0.0.1", LocalPort: freePort(t), TargetAddress: "127.0.0.1", TargetPort: targetPort}}
}
func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}
func startReplyServer(t *testing.T, port int) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", Address("127.0.0.1", port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				connection.SetDeadline(time.Now().Add(5 * time.Second))
				payload, _ := io.ReadAll(connection)
				connection.Write(append([]byte("reply:"), payload...))
			}()
		}
	}()
	return listener
}
func exchange(t *testing.T, config domain.Tunnel) string {
	t.Helper()
	connection, err := net.DialTimeout("tcp", Address(config.LocalAddress, config.LocalPort), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := connection.Write([]byte("hello\x00world")); err != nil {
		t.Fatal(err)
	}
	connection.(*net.TCPConn).CloseWrite()
	data, err := io.ReadAll(connection)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func assertPortReleased(t *testing.T, config domain.Tunnel) {
	t.Helper()
	listener, err := net.Listen("tcp", Address(config.LocalAddress, config.LocalPort))
	if err != nil {
		t.Fatalf("local port was not released: %v", err)
	}
	listener.Close()
}

func TestLocalForwardingOverSSHAndTerminalIndependentLifecycle(t *testing.T) {
	fixture := newSSHFixture(t)
	target := startReplyServer(t, 0)
	config := configFor(t, target.Addr().(*net.TCPAddr).Port)
	manager := NewManager(nil)
	t.Cleanup(manager.Close)
	if err := manager.Start(config, fixture.connect); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return manager.Status(config).State == "running" })
	if got := exchange(t, config); got != "reply:hello\x00world" {
		t.Fatalf("forwarded response = %q", got)
	}
	// No shell or terminal session is ever created, and repeated starts reuse it.
	if err := manager.Start(config, fixture.connect); err != nil {
		t.Fatal(err)
	}
	if fixture.connections.Load() != 1 {
		t.Fatal("duplicate SSH transport")
	}
	edited := config
	edited.LocalPort = freePort(t)
	if active := manager.Status(edited).ActiveConfig; active == nil || active.LocalPort != config.LocalPort {
		t.Fatal("editing replaced the running endpoint")
	}
	manager.Stop(config.ID)
	if manager.Status(config).State != "stopped" {
		t.Fatal("not stopped")
	}
	assertPortReleased(t, config)
	if err := manager.Start(edited, fixture.connect); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return manager.Status(edited).State == "running" })
	if got := exchange(t, edited); got != "reply:hello\x00world" {
		t.Fatal(got)
	}
	manager.Close()
	assertPortReleased(t, edited)
	if err := manager.Start(config, fixture.connect); err == nil {
		t.Fatal("started after shutdown")
	}
}

func TestTargetFailureKeepsTunnelListeningAndRecovers(t *testing.T) {
	fixture := newSSHFixture(t)
	config := configFor(t, freePort(t))
	manager := NewManager(nil)
	t.Cleanup(manager.Close)
	if err := manager.Start(config, fixture.connect); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return manager.Status(config).State == "running" })
	connection, err := net.Dial("tcp", Address(config.LocalAddress, config.LocalPort))
	if err != nil {
		t.Fatal(err)
	}
	connection.SetDeadline(time.Now().Add(2 * time.Second))
	io.ReadAll(connection)
	connection.Close()
	await(t, func() bool { return manager.Status(config).LastError != "" })
	if manager.Status(config).State != "running" {
		t.Fatal("target failure stopped tunnel")
	}
	startReplyServer(t, config.TargetPort)
	if got := exchange(t, config); got != "reply:hello\x00world" {
		t.Fatal(got)
	}
	await(t, func() bool { return manager.Status(config).LastError == "" })
	fixture.deny.Store(true)
	connection, err = net.Dial("tcp", Address(config.LocalAddress, config.LocalPort))
	if err != nil {
		t.Fatal(err)
	}
	connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.ReadAll(connection)
	connection.Close()
	await(t, func() bool { return strings.Contains(manager.Status(config).LastError, "denied") })
}

func TestSSHDisconnectFailsTunnelAndReleasesListener(t *testing.T) {
	fixture := newSSHFixture(t)
	config := configFor(t, freePort(t))
	manager := NewManager(nil)
	t.Cleanup(manager.Close)
	if err := manager.Start(config, fixture.connect); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return manager.Status(config).State == "running" })
	fixture.disconnect()
	await(t, func() bool { return manager.Status(config).State == "failed" })
	await(t, func() bool {
		listener, err := net.Listen("tcp", Address(config.LocalAddress, config.LocalPort))
		if err != nil {
			return false
		}
		listener.Close()
		return true
	})
	if manager.Status(config).Error == "" {
		t.Fatal("missing failure reason")
	}
}

func TestPortConflictDoesNotPromptOrChooseAnotherPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config := configFor(t, freePort(t))
	config.LocalPort = listener.Addr().(*net.TCPAddr).Port
	manager := NewManager(nil)
	t.Cleanup(manager.Close)
	var called atomic.Bool
	if err := manager.Start(config, func(context.Context) (Transport, error) {
		called.Store(true)
		return nil, errors.New("unexpected authentication")
	}); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return manager.Status(config).State == "failed" })
	if called.Load() {
		t.Fatal("authentication attempted for occupied port")
	}
	if !strings.Contains(manager.Status(config).Error, Address(config.LocalAddress, config.LocalPort)) {
		t.Fatal("missing occupied address")
	}
}

func TestStopCancelsAuthenticationAndClosesActiveConnections(t *testing.T) {
	config := configFor(t, freePort(t))
	manager := NewManager(nil)
	t.Cleanup(manager.Close)
	entered := make(chan struct{})
	if err := manager.Start(config, func(ctx context.Context) (Transport, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	<-entered
	manager.Stop(config.ID)
	assertPortReleased(t, config)
	if manager.Status(config).State != "stopped" {
		t.Fatal("cancel became failure")
	}
	fixture := newSSHFixture(t)
	listener := startReplyServer(t, 0)
	config.TargetPort = listener.Addr().(*net.TCPAddr).Port
	if err := manager.Start(config, fixture.connect); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return manager.Status(config).State == "running" })
	connection, err := net.Dial("tcp", Address(config.LocalAddress, config.LocalPort))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.Write([]byte("still running"))
	manager.StopAll()
	connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("active connection survived StopAll")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("active connection was not closed")
	}
	assertPortReleased(t, config)
}
