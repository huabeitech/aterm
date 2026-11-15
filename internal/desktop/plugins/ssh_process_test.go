package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"golang.org/x/crypto/ssh"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func sshExecFixture(t *testing.T) *ssh.Client {
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
	var workers sync.WaitGroup
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
			if incoming.ChannelType() != "session" {
				incoming.Reject(ssh.UnknownChannelType, "session required")
				continue
			}
			channel, requests, err := incoming.Accept()
			if err != nil {
				continue
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer channel.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan struct{})
				started := false
				for request := range requests {
					if request.Type != "exec" || started {
						request.Reply(false, nil)
						continue
					}
					var payload struct{ Command string }
					if ssh.Unmarshal(request.Payload, &payload) != nil {
						request.Reply(false, nil)
						continue
					}
					request.Reply(true, nil)
					started = true
					go func(commandText string) {
						defer close(done)
						command := exec.CommandContext(ctx, "/bin/sh", "-c", commandText)
						cleanupProcess := configureProcess(command)
						defer cleanupProcess()
						command.Stdout = channel
						command.Stderr = channel.Stderr()
						err := command.Run()
						code := uint32(0)
						if err != nil {
							code = 1
							if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() >= 0 {
								code = uint32(exit.ExitCode())
							}
						}
						channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
						channel.Close()
					}(payload.Command)
				}
				cancel()
				if started {
					<-done
				}
			}()
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); workers.Wait() })
	return client
}
func TestSSHProcessQuotingAndCancellationPreservesParentConnection(t *testing.T) {
	client := sshExecFixture(t)
	literal := `literal ' ; $(printf injected) 中文`
	request := ProcessRequest{Executable: "printf", Args: []string{"%s", literal}, TimeoutMS: 1000, MaxOutputBytes: 4096}
	result, err := RunSSHProcess(context.Background(), client, request, ExecutionPolicy{}, nil)
	if err != nil || result.Stdout != literal || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatal(result, err)
	}
	request = ProcessRequest{Executable: "sh", Args: []string{"-c", "sleep 5"}, TimeoutMS: 100, MaxOutputBytes: 1024}
	start := time.Now()
	if _, err = RunSSHProcess(context.Background(), client, request, ExecutionPolicy{}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("SSH command did not cancel promptly")
	}
	request = ProcessRequest{Executable: "printf", Args: []string{"connection-alive"}, TimeoutMS: 1000, MaxOutputBytes: 1024}
	result, err = RunSSHProcess(context.Background(), client, request, ExecutionPolicy{}, nil)
	if err != nil || !strings.Contains(result.Stdout, "connection-alive") {
		t.Fatal("cancelling an exec channel damaged its owning SSH transport", result, err)
	}
}
