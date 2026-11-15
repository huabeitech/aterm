package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTerminalOutputBackpressure(t *testing.T) {
	m := NewManager(context.Background(), func(string, any) {})
	s := &Session{ID: "flow"}
	m.sessions[s.ID] = s
	if _, err := m.AttachOutput(s.ID); err != nil {
		t.Fatal(err)
	}
	m.appendOutput(s, make([]byte, 256*1024+1))
	done := make(chan struct{})
	go func() { m.appendOutput(s, []byte("next")); close(done) }()
	select {
	case <-done:
		t.Fatal("output did not pause")
	case <-time.After(20 * time.Millisecond):
	}
	if err := m.AcknowledgeOutput(s.ID, s.outputOffset+1); err == nil {
		t.Fatal("future acknowledgement accepted")
	}
	if err := m.AcknowledgeOutput(s.ID, 200*1024); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("acknowledgement did not resume output")
	}
	snapshot, err := m.AttachOutput(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Offset != 256*1024+5 {
		t.Fatalf("unexpected snapshot watermark %d", snapshot.Offset)
	}
	if err := m.AcknowledgeOutput(s.ID, 1); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	ack := s.outputAcknowledged
	m.mu.RUnlock()
	if ack != snapshot.Offset {
		t.Fatal("stale acknowledgement moved watermark backwards")
	}
}

func TestTerminalCloseWakesBlockedOutput(t *testing.T) {
	m := NewManager(context.Background(), func(string, any) {})
	s := &Session{ID: "close", State: "exited"}
	m.sessions[s.ID] = s
	_, _ = m.AttachOutput(s.ID)
	m.appendOutput(s, make([]byte, 256*1024+1))
	done := make(chan struct{})
	go func() { m.appendOutput(s, []byte("discard")); close(done) }()
	if err := m.Close(s.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closed session retained blocked output")
	}
}

func TestUnattachedTerminalRetainsBoundedReplayWithoutPublishing(t *testing.T) {
	events := 0
	m := NewManager(context.Background(), func(string, any) { events++ })
	s := &Session{ID: "unattached"}
	m.sessions[s.ID] = s
	// Trim through a multibyte code point to exercise the replay boundary.
	data := []byte("你好")
	m.appendOutput(s, bytes.Repeat(data, terminalReplayLimit/len(data)+2))
	if events != 0 {
		t.Fatal("unattached output was published")
	}
	snapshot, err := m.AttachOutput(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(snapshot.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) > terminalReplayLimit || !utf8.Valid(decoded) {
		t.Fatal("invalid replay boundary")
	}
	m.appendOutput(s, []byte("live"))
	if events != 1 {
		t.Fatal("attached output was not published")
	}
}

func TestPTYOutputResumesAfterRendererStall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events := make(chan OutputEvent, 4096)
	m := NewManager(ctx, func(_ string, payload any) {
		event := payload.(OutputEvent)
		if event.Data != "" {
			select {
			case events <- event:
			case <-ctx.Done():
			}
		}
	})
	defer m.CloseAll()
	session, err := m.openWithOptions("local", "Flow Test", "", exec.Command("/bin/sh", "-i"), LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.AttachOutput(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	command := `awk 'BEGIN {for(i=0;i<2097152;i++)printf "x"}' ; printf '\146\154\157\167\055\144\157\156\145\n'` + "\r"
	if err = m.Write(session.ID, base64.StdEncoding.EncodeToString([]byte(command))); err != nil {
		t.Fatal(err)
	}
	for {
		m.mu.RLock()
		s := m.sessions[session.ID]
		paused := s.outputPaused
		pending := s.outputOffset - s.outputAcknowledged
		m.mu.RUnlock()
		if paused {
			if pending > 256*1024+32*1024 {
				t.Fatalf("unbounded pending output %d", pending)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("PTY never reached backpressure threshold")
		case <-time.After(time.Millisecond):
		}
	}
	offset := snapshot.Offset
	count := 0
	tail := ""
	for {
		select {
		case event := <-events:
			data, err := base64.StdEncoding.DecodeString(event.Data)
			if err != nil {
				t.Fatal(err)
			}
			if event.Offset <= offset {
				continue
			}
			if event.Offset-uint64(len(data)) != offset {
				t.Fatal("output gap during resume")
			}
			offset = event.Offset
			count += bytes.Count(data, []byte("x"))
			tail += string(data)
			if len(tail) > 64 {
				tail = tail[len(tail)-64:]
			}
			if err = m.AcknowledgeOutput(session.ID, offset); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(tail, "flow-done") {
				if count < 2097152 {
					t.Fatalf("output lost: %d payload bytes", count)
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("PTY output did not finish after acknowledgements")
		}
	}
}
