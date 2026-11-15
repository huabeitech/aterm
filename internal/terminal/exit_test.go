package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"sync"
	"testing"
	"time"

	"ssh-connect/internal/desktop/openssh"
)

type exitFixtureProcess struct {
	reader               *io.PipeReader
	output               *io.PipeWriter
	exit, waited, closed chan struct{}
	exitOnce, closeOnce  sync.Once
}

func newExitFixtureProcess() *exitFixtureProcess {
	reader, writer := io.Pipe()
	return &exitFixtureProcess{reader: reader, output: writer, exit: make(chan struct{}), waited: make(chan struct{}), closed: make(chan struct{})}
}
func (p *exitFixtureProcess) Read(data []byte) (int, error)  { return p.reader.Read(data) }
func (p *exitFixtureProcess) Write(data []byte) (int, error) { return len(data), nil }
func (p *exitFixtureProcess) PID() int                       { return 1 }
func (p *exitFixtureProcess) Resize(uint16, uint16) error    { return nil }
func (p *exitFixtureProcess) Kill() error                    { p.exitOnce.Do(func() { close(p.exit) }); return nil }
func (p *exitFixtureProcess) Close() error {
	p.closeOnce.Do(func() { p.reader.Close(); p.output.Close(); close(p.closed) })
	return nil
}

type fixtureExitError struct{}

func (fixtureExitError) Error() string    { return "fixture exited" }
func (fixtureExitError) ExitCode() int    { return 7 }
func (p *exitFixtureProcess) Wait() error { <-p.exit; close(p.waited); return fixtureExitError{} }

func exitFixture(t *testing.T) (*Manager, *Session, *exitFixtureProcess, chan OutputEvent, chan struct{}) {
	t.Helper()
	events := make(chan OutputEvent, 128)
	ctx, cancel := context.WithCancel(context.Background())
	manager := NewManager(ctx, func(_ string, payload any) { events <- payload.(OutputEvent) })
	process := newExitFixtureProcess()
	session := &Session{ID: "exit-fixture", Kind: "local", State: "running", pty: process, outputDone: make(chan struct{}), closed: make(chan struct{}), Remote: &openssh.RemoteContext{}}
	manager.sessions[session.ID] = session
	finished := make(chan struct{})
	go manager.forwardOutput(session)
	go func() { defer close(finished); manager.waitForExit(session) }()
	t.Cleanup(func() {
		manager.CloseAll()
		process.Kill()
		cancel()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("exit waiter did not stop")
		}
	})
	return manager, session, process, events, finished
}
func TestTerminalExitFollowsCompleteTailAndClearsRemoteContext(t *testing.T) {
	manager, session, process, events, _ := exitFixture(t)
	process.Kill()
	<-process.waited
	select {
	case event := <-events:
		t.Fatalf("exit published before output EOF: %+v", event)
	case <-time.After(30 * time.Millisecond):
	}
	payload := []byte("final 中文\r\n")
	if _, err := process.output.Write(payload); err != nil {
		t.Fatal(err)
	}
	process.output.Close()
	select {
	case event := <-events:
		if event.State != "exited" || event.ExitCode == nil || *event.ExitCode != 7 {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("exit not published after output EOF")
	}
	snapshot, err := manager.OutputSnapshot(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(snapshot.Data)
	if err != nil || !bytes.Equal(data, payload) || snapshot.Offset != uint64(len(payload)) {
		t.Fatal("tail incomplete at exit", snapshot, err)
	}
	manager.mu.RLock()
	remote := session.Remote
	manager.mu.RUnlock()
	if remote != nil {
		t.Fatal("finished terminal retained active SSH context")
	}
	if err := manager.Resize(session.ID, 100, 40); err != nil {
		t.Fatal("exited terminal tried resizing a closed transport", err)
	}
}
func TestTerminalCloseSuppressesPendingExitAndReleasesReader(t *testing.T) {
	manager, session, process, events, finished := exitFixture(t)
	process.Kill()
	<-process.waited
	if err := manager.Close(session.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("closed terminal exit remained blocked")
	}
	select {
	case event := <-events:
		t.Fatalf("closed session emitted an exit: %+v", event)
	default:
	}
	select {
	case <-session.outputDone:
	case <-time.After(time.Second):
		t.Fatal("closed terminal reader remained blocked")
	}
}
func TestTerminalExitInterruptsInheritedOpenTransport(t *testing.T) {
	_, _, process, events, _ := exitFixture(t)
	process.Kill()
	select {
	case event := <-events:
		if event.State != "exited" {
			t.Fatal(event)
		}
	case <-time.After(terminalExitDrainGrace + time.Second):
		t.Fatal("inherited PTY prevented terminal completion")
	}
	select {
	case <-process.closed:
	default:
		t.Fatal("inherited transport left open")
	}
}
func TestTerminalExitDrainHonorsRendererBackpressure(t *testing.T) {
	manager, session, process, events, _ := exitFixture(t)
	if _, err := manager.AttachOutput(session.ID); err != nil {
		t.Fatal(err)
	}
	payload := append(bytes.Repeat([]byte("x"), 512*1024), []byte("尾部")...)
	writeDone := make(chan error, 1)
	go func() { _, err := process.output.Write(payload); process.output.Close(); writeDone <- err }()
	process.Kill()
	deadline := time.Now().Add(time.Second)
	for {
		manager.mu.RLock()
		paused := session.outputPaused
		manager.mu.RUnlock()
		if paused {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("renderer backpressure did not pause output")
		}
		time.Sleep(time.Millisecond)
	}
	// Cross the inherited-transport grace interval while paused. This is a slow
	// renderer, not an inherited idle pipe: its accepted tail must not be dropped.
	select {
	case <-process.closed:
		t.Fatal("paused output transport closed early")
	case <-time.After(terminalExitDrainGrace + 30*time.Millisecond):
	}
	var received []byte
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.State == "exited" {
				if !bytes.Equal(received, payload) {
					t.Fatalf("exit preceded complete output: %d/%d", len(received), len(payload))
				}
				if err := <-writeDone; err != nil {
					t.Fatal(err)
				}
				return
			}
			data, err := base64.StdEncoding.DecodeString(event.Data)
			if err != nil {
				t.Fatal(err)
			}
			received = append(received, data...)
			if err := manager.AcknowledgeOutput(session.ID, event.Offset); err != nil {
				t.Fatal(err)
			}
		case <-timer.C:
			t.Fatal("terminal exit did not follow resumed output")
		}
	}
}
