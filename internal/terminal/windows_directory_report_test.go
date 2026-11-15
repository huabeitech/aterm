package terminal

import (
	"context"
	"encoding/base64"
	"os/exec"
	"runtime"
	"ssh-connect/internal/desktop/localpty"
	"strings"
	"testing"
	"time"
)

func TestReportedDirectoryUpdatesOnlyOwnedRunningSession(t *testing.T) {
	events := 0
	manager := NewManager(context.Background(), func(_ string, _ any) { events++ })
	session := &Session{ID: "test", State: "running", Cwd: `C:\old`, pluginTitle: "Plugin title"}
	manager.sessions[session.ID] = session
	parser := localpty.NewDirectoryReports("private", func(path string) { manager.updateReportedDirectory(session, path) })
	frame := "\x1b]777;aterm-cwd;private;" + base64.StdEncoding.EncodeToString([]byte(`C:\space 中文`)) + "\x07"
	for _, value := range []byte(frame) {
		parser.Consume([]byte{value})
	}
	if session.Cwd != `C:\space 中文` || session.Title != "Plugin title" || events != 1 {
		t.Fatal(session, events)
	}
	parser.Consume([]byte(frame))
	if events != 1 {
		t.Fatal("duplicate cwd emitted another event")
	}
	session.State = "exited"
	manager.updateReportedDirectory(session, `C:\exited`)
	if session.Cwd != `C:\space 中文` || events != 1 {
		t.Fatal("exited session updated")
	}
	replacement := &Session{ID: session.ID, State: "running", Cwd: `C:\replacement`}
	manager.sessions[session.ID] = replacement
	session.State = "running"
	manager.updateReportedDirectory(session, `C:\late`)
	if replacement.Cwd != `C:\replacement` || events != 1 {
		t.Fatal("stale session updated")
	}
}
func TestPTYReaderObservesDirectoryReportsWithoutChangingReplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY harness; native PowerShell test is in localpty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager := NewManager(ctx, nil)
	// This harness isolates the report path from the separate Unix PID poller.
	manager.directoryPolling = true
	defer manager.CloseAll()
	path := `C:\space 中文`
	frame := "\x1b]777;aterm-cwd;private;" + base64.StdEncoding.EncodeToString([]byte(path)) + "\x07"
	command := exec.Command("/bin/sh", "-c", `printf '%s' "$1"; read answer`, "sh", frame)
	session, err := manager.openWithOptions("local", "Local", "", command, LaunchOptions{windowsDirectoryNonce: "private", Title: "Report terminal"})
	if err != nil {
		t.Fatal(err)
	}
	for {
		manager.mu.RLock()
		current := manager.sessions[session.ID]
		cwd, title, output := current.Cwd, current.Title, string(current.output)
		manager.mu.RUnlock()
		if cwd == path && strings.Contains(output, frame) {
			if title != "Report terminal" {
				t.Fatal(title)
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("directory/report replay missing: cwd=%q output=%q", cwd, output)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
