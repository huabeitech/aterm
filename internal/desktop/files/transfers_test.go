package files

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func transferFixture(t *testing.T) (*Manager, string, *Transfers, <-chan Transfer) {
	t.Helper()
	files, id, _ := fixture(t)
	events := make(chan Transfer, 1024)
	transfers := NewTransfers(files, func(event any) {
		payload := event.(map[string]any)
		if payload["type"] == "transfer" {
			events <- payload["transfer"].(Transfer)
		}
	})
	t.Cleanup(transfers.Close)
	return files, id, transfers, events
}
func awaitTransfer(t *testing.T, events <-chan Transfer, id string, done func(Transfer) bool) Transfer {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.ID == id && done(event) {
				return event
			}
		case <-timer.C:
			t.Fatal("transfer did not reach expected state")
			return Transfer{}
		}
	}
}
func terminalTransfer(value Transfer) bool {
	return value.State == "completed" || value.State == "failed" || value.State == "cancelled"
}
func TestTransferStagesTreesAndNeverOverwritesExistingDownloads(t *testing.T) {
	files, id, transfers, events := transferFixture(t)
	session, _ := files.Session(id)
	directory := t.TempDir()
	source := filepath.Join(directory, "folder")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "hello.txt"), []byte("你好 transfer"), 0o600); err != nil {
		t.Fatal(err)
	}
	upload, err := transfers.Queue(id, "upload", source, "/uploaded")
	if err != nil {
		t.Fatal(err)
	}
	state := awaitTransfer(t, events, upload.ID, terminalTransfer)
	if state.State != "completed" || !state.Directory || state.Transferred != int64(len("你好 transfer")) {
		t.Fatalf("upload: %#v", state)
	}
	if _, err := session.Client.Stat("/uploaded/nested/hello.txt"); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "download")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "keep"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	download, err := transfers.Queue(id, "download", destination, "/uploaded")
	if err != nil {
		t.Fatal(err)
	}
	state = awaitTransfer(t, events, download.ID, terminalTransfer)
	if state.State != "completed" || state.LocalPath == destination {
		t.Fatalf("download: %#v", state)
	}
	data, err := os.ReadFile(filepath.Join(state.LocalPath, "nested", "hello.txt"))
	if err != nil || string(data) != "你好 transfer" {
		t.Fatalf("download content: %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(destination, "keep"))
	if err != nil || string(data) != "original" {
		t.Fatal("existing download folder was modified")
	}
}
func TestTransferRetryAndCancellationLeaveNoPartialDestination(t *testing.T) {
	files, id, transfers, events := transferFixture(t)
	session, _ := files.Session(id)
	source := filepath.Join(t.TempDir(), "source")
	upload, err := transfers.Queue(id, "upload", source, "/retry")
	if err != nil {
		t.Fatal(err)
	}
	if state := awaitTransfer(t, events, upload.ID, terminalTransfer); state.State != "failed" {
		t.Fatalf("missing source did not fail: %#v", state)
	}
	if err := os.WriteFile(source, []byte("retry works"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := transfers.Retry(id, upload.ID); err != nil {
		t.Fatal(err)
	}
	if state := awaitTransfer(t, events, upload.ID, terminalTransfer); state.State != "completed" {
		t.Fatalf("retry: %#v", state)
	}
	if err := transfers.Remove("unrelated", upload.ID); err == nil {
		t.Fatal("cross-session removal accepted")
	}
	if err := os.WriteFile(source, []byte(strings.Repeat("a", 8*1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	cancelled, err := transfers.Queue(id, "upload", source, "/cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if err := transfers.Cancel(id, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	state := awaitTransfer(t, events, cancelled.ID, terminalTransfer)
	if state.State != "cancelled" {
		t.Fatalf("cancel: %#v", state)
	}
	if _, err := session.Client.Stat("/cancelled"); err == nil {
		t.Fatal("cancelled transfer published a partial destination")
	}
	if err := transfers.Remove(id, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	transfers.ClearCompleted(id)
	if _, err := transfers.CompletedDownload(id, upload.ID); err == nil {
		t.Fatal("cleared task remains accessible")
	}
}
func TestTransferRejectsSymlinksAndPreservesRemoteCollisions(t *testing.T) {
	files, id, transfers, events := transferFixture(t)
	session, _ := files.Session(id)
	directory := t.TempDir()
	source := filepath.Join(directory, "file")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, session.Client, "/existing", "original")
	task, err := transfers.Queue(id, "upload", source, "/existing")
	if err != nil {
		t.Fatal(err)
	}
	if state := awaitTransfer(t, events, task.ID, terminalTransfer); state.State != "failed" {
		t.Fatal("remote collision was overwritten")
	}
	text, err := files.ReadText(id, "/existing")
	if err != nil || text.Text != "original" {
		t.Fatal("existing remote file was modified")
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	task, err = transfers.Queue(id, "upload", link, "/link")
	if err != nil {
		t.Fatal(err)
	}
	if state := awaitTransfer(t, events, task.ID, terminalTransfer); state.State != "failed" {
		t.Fatal("symlink was followed during transfer")
	}
}

type pausingWriter struct {
	bytes.Buffer
	afterFirstWrite func()
}

func (w *pausingWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if w.afterFirstWrite != nil {
		w.afterFirstWrite()
		w.afterFirstWrite = nil
	}
	return n, err
}
func (*pausingWriter) Close() error { return nil }

func TestPausedCopyCanResumeOrCancel(t *testing.T) {
	for _, action := range []string{"resume", "cancel"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			task := &transferTask{ctx: ctx, cancel: cancel, info: Transfer{SessionID: "session", ID: "file", State: "running"}, started: time.Now()}
			m := &Transfers{tasks: map[string]*transferTask{"file": task}}
			paused := make(chan struct{})
			dest := &pausingWriter{afterFirstWrite: func() { m.SetPaused("session", "file", true); close(paused) }}
			data := bytes.Repeat([]byte("sample"), 50000)
			done := make(chan error, 1)
			go func() { done <- m.copy(ctx, task, dest, io.NopCloser(bytes.NewReader(data))) }()
			<-paused
			select {
			case <-done:
				t.Fatal("copy continued while paused")
			case <-time.After(20 * time.Millisecond):
			}
			if action == "cancel" {
				m.Cancel("session", "file")
			} else {
				m.SetPaused("session", "file", false)
			}
			select {
			case err := <-done:
				if action == "cancel" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel: %v", err)
					}
				} else if err != nil || !bytes.Equal(dest.Bytes(), data) {
					t.Fatal("resumed copy differs", err)
				}
			case <-time.After(time.Second):
				t.Fatal("paused copy stuck")
			}
		})
	}
}
