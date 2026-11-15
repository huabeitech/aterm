package zmodem

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSendToElectronProtocolPeerStreamsBatchAndSkippedFiles(t *testing.T) {
	for _, offset := range []int{0, 73, uploadWindow + 17} {
		t.Run(strconv.Itoa(offset), func(t *testing.T) { sendToElectronPeer(t, offset) })
	}
}

func sendToElectronPeer(t *testing.T, resumeOffset int) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for live Electron protocol peer")
	}
	directory := t.TempDir()
	destination := filepath.Join(directory, "received")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, uploadWindow+17)
	for i := range payload {
		payload[i] = byte(i)
	}
	paths := []string{}
	for _, name := range []string{"peer 中文.bin", "skip.bin", "empty.bin"} {
		data := payload
		if name == "empty.bin" {
			data = nil
		}
		file := filepath.Join(directory, name)
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, file)
	}
	script, err := filepath.Abs("../../../scripts/zmodem-receive-peer.cjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", script, "--library", filepath.Join(reference, "node_modules/zmodem.js-ex"), "--directory", destination, "--skip", "skip.bin", "--resume-offset", strconv.Itoa(resumeOffset))
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic, text bytes.Buffer
	command.Stderr = &diagnostic
	var final Progress
	selections := 0
	router := NewRouter(ctx, RouterOptions{Write: func(data []byte) error { _, err := input.Write(data); return err }, Output: func(data []byte) { text.Write(data) }, Select: func(ctx context.Context) ([]Upload, error) { selections++; return OpenUploads(ctx, paths) }, Progress: func(value Progress) { final = value }})
	defer router.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 32768)
	for {
		count, err := output.Read(data)
		if count > 0 {
			router.Consume(data[:count])
		}
		if err != nil {
			if err != io.EOF {
				t.Error(err)
			}
			break
		}
	}
	if err = command.Wait(); err != nil {
		t.Fatalf("Electron receiver failed: %v: %s; progress: %+v", err, diagnostic.String(), final)
	}
	received, err := os.ReadFile(filepath.Join(destination, "peer 中文.bin"))
	if err != nil || !bytes.Equal(received, payload[resumeOffset:]) {
		t.Fatal("uploaded bytes differ", err)
	}
	info, err := os.Stat(filepath.Join(destination, "empty.bin"))
	if err != nil || info.Size() != 0 {
		t.Fatal("empty upload missing", err)
	}
	if _, err = os.Stat(filepath.Join(destination, "skip.bin")); !os.IsNotExist(err) {
		t.Fatal("skipped file was written", err)
	}
	if selections != 1 || final.State != "completed" || final.Direction != "upload" || router.Busy() {
		t.Fatal("upload session did not complete", selections, final)
	}
	if text.String() != "ZMODEM upload fixture completed\r\n" {
		t.Fatalf("upload protocol leaked to terminal %q", text.String())
	}
}
