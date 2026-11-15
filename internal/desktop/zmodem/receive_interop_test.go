package zmodem

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReceiveFromElectronProtocolPeerToRealStagedFile(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for live Electron protocol peer")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.bin")
	destination := filepath.Join(directory, "received.bin")
	payload := make([]byte, 256*1024+17)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../../scripts/zmodem-peer.cjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", script, "--library", filepath.Join(reference, "node_modules/zmodem.js-ex"), "--file", source, "--name", "peer 中文.bin")
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
	offers := 0
	router := NewRouter(ctx, RouterOptions{Write: func(data []byte) error { _, err := input.Write(data); return err }, Output: func(data []byte) { text.Write(data) }, Offer: func(ctx context.Context, offer FileOffer) (Download, error) {
		offers++
		if offer.Name != "peer 中文.bin" || offer.Size != int64(len(payload)) {
			t.Error("peer metadata lost", offer)
		}
		return NewDownload(ctx, destination)
	}, Progress: func(value Progress) { final = value }})
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
		t.Fatalf("Electron peer failed: %v: %s", err, diagnostic.String())
	}
	received, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(received, payload) {
		t.Fatal("received file differs from Electron source", err)
	}
	if offers != 1 || final.State != "completed" || final.Transferred != int64(len(payload)) || router.Busy() {
		t.Fatal("peer did not complete", offers, final)
	}
	if text.String() != "ZMODEM fixture completed\r\n" {
		t.Fatalf("protocol leaked into terminal output %q", text.String())
	}
}
