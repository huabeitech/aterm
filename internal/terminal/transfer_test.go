package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"ssh-connect/internal/desktop/zmodem"
)

func TestTerminalReceivesZmodemWithoutProtocolInReplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY fixture")
	}
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for PTY ZMODEM acceptance")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.bin")
	destination := filepath.Join(directory, "download.bin")
	payload := make([]byte, 128*1024+23)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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
	manager.download = func(ctx context.Context, _ string, offer zmodem.FileOffer) (zmodem.Download, error) {
		if offer.Name != "fixture.bin" || offer.Size != int64(len(payload)) {
			t.Error("incorrect terminal offer", offer)
		}
		return zmodem.NewDownload(ctx, destination)
	}
	command := exec.Command("node", filepath.Join("..", "..", "scripts", "zmodem-peer.cjs"), "--library", filepath.Join(reference, "node_modules/zmodem.js-ex"), "--file", source, "--name", "fixture.bin")
	session, err := manager.openWithOptions("local", "fixture", "", command, LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		manager.mu.RLock()
		entry := manager.sessions[session.ID]
		state := entry.State
		replay := bytes.Clone(entry.output)
		manager.mu.RUnlock()
		progressMu.Lock()
		progress := last
		progressMu.Unlock()
		if state == "exited" && progress.State == "completed" && bytes.Contains(replay, []byte("ZMODEM fixture completed")) {
			if bytes.Contains(replay, []byte{0x18}) || bytes.Contains(replay, []byte{0, 1, 2, 3}) {
				t.Fatalf("wire bytes leaked into canonical terminal replay: %x", replay)
			}
			received, err := os.ReadFile(destination)
			if err != nil || !bytes.Equal(received, payload) {
				t.Fatal("PTY download differs from peer", err)
			}
			if progress.Transferred != int64(len(payload)) {
				t.Fatal("incorrect final progress", progress)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("PTY receive did not complete: state=%s progress=%+v replay=%q", state, progress, replay)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type recordedTerminalInput struct {
	mu   sync.Mutex
	data []byte
}

func (r *recordedTerminalInput) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, data...)
	return len(data), nil
}
func (r *recordedTerminalInput) Close() error { return nil }
func (r *recordedTerminalInput) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bytes.Clone(r.data)
}
func TestTerminalTransferGuardsInputAndCtrlCCancelsSelection(t *testing.T) {
	manager := NewManager(context.Background(), nil)
	defer manager.CloseAll()
	selected := make(chan struct{})
	manager.download = func(ctx context.Context, _ string, _ zmodem.FileOffer) (zmodem.Download, error) {
		close(selected)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	input := &recordedTerminalInput{}
	session := &Session{ID: "transfer", Kind: "ssh", State: "running", stdin: input}
	manager.setupTransfer(session)
	manager.sessions[session.ID] = session
	initial, _ := zmodem.EncodeHeader(zmodem.Header{Type: zmodem.ZRQINIT}, zmodem.Hex)
	header, _ := zmodem.EncodeHeader(zmodem.OffsetHeader(zmodem.ZFILE, 1<<24), zmodem.Binary32)
	metadata, _ := zmodem.EncodeSubpacket([]byte("file.bin\x004\x00"), zmodem.ZCRCW, zmodem.Binary32)
	wire := append(append(initial, header...), metadata...)
	done := make(chan struct{})
	go func() { manager.appendOutput(session, wire); close(done) }()
	select {
	case <-selected:
	case <-time.After(time.Second):
		t.Fatal("transfer did not reach selection")
	}
	before := input.Bytes()
	if err := manager.Write(session.ID, base64.StdEncoding.EncodeToString([]byte("do not send"))); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, input.Bytes()) {
		t.Fatal("ordinary typing corrupted transfer")
	}
	if err := manager.WriteBinary(session.ID, []byte{3}); err != nil {
		t.Fatal(err)
	}
	if !session.transfer.Busy() || !bytes.Equal(before, input.Bytes()) {
		t.Fatal("binary input bypassed transfer guard")
	}
	if err := manager.Write(session.ID, base64.StdEncoding.EncodeToString([]byte{3})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Ctrl+C did not stop selection")
	}
	if session.transfer.Busy() || !bytes.HasSuffix(input.Bytes(), []byte{24, 24, 24, 24, 24, 8, 8, 8, 8, 8}) {
		t.Fatal("cancel did not abort protocol or release input")
	}
	if err := manager.CancelTransfer("transfer"); err != nil {
		t.Fatal("cancel of an inactive transfer must be a no-op", err)
	}
}

func TestTerminalUploadsZmodemWithoutProtocolInReplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY fixture")
	}
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for PTY ZMODEM acceptance")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.bin")
	destination := filepath.Join(directory, "received")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 128*1024+23)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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
	command := exec.Command("node", filepath.Join("..", "..", "scripts", "zmodem-receive-peer.cjs"), "--library", filepath.Join(reference, "node_modules/zmodem.js-ex"), "--directory", destination)

	session, err := manager.openWithOptions("local", "fixture", "", command, LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		manager.mu.RLock()
		entry := manager.sessions[session.ID]
		state := entry.State
		replay := bytes.Clone(entry.output)
		manager.mu.RUnlock()
		progressMu.Lock()
		progress := last
		progressMu.Unlock()
		if state == "exited" && progress.State == "completed" && bytes.Contains(replay, []byte("ZMODEM upload fixture completed")) {
			if bytes.Contains(replay, []byte{0x18}) || bytes.Contains(replay, []byte{0, 1, 2, 3}) {
				t.Fatalf("wire bytes leaked into canonical terminal replay: %x", replay)
			}
			received, err := os.ReadFile(filepath.Join(destination, "source.bin"))
			if err != nil || !bytes.Equal(received, payload) {
				t.Fatal("PTY upload differs from source", err)
			}
			if progress.Transferred != int64(len(payload)) {
				t.Fatal("incorrect final progress", progress)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("PTY upload did not complete: state=%s progress=%+v replay=%q", state, progress, replay)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type partialTerminalWriter struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *partialTerminalWriter) Write(data []byte) (int, error) {
	count := min(len(data), w.limit)
	w.Buffer.Write(data[:count])
	return count, w.err
}
func TestTerminalProtocolWritesKeepEveryByteOrReportFailure(t *testing.T) {
	payload := []byte{42, 24, 65, 0, 1, 2, 3, 255}
	writer := &partialTerminalWriter{limit: 3}
	if err := writeTerminalInput(writer, payload); err != nil || !bytes.Equal(writer.Bytes(), payload) {
		t.Fatal("partial transport lost protocol bytes", err, writer.Bytes())
	}
	writer = &partialTerminalWriter{limit: 0}
	if err := writeTerminalInput(writer, payload); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("zero write was accepted", err)
	}
	writer = &partialTerminalWriter{limit: 3, err: io.ErrClosedPipe}
	if err := writeTerminalInput(writer, payload); !errors.Is(err, io.ErrClosedPipe) || writer.Len() != 3 {
		t.Fatal("transport failure was hidden or retried", err)
	}
}
