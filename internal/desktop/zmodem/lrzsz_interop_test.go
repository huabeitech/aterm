package zmodem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// These peers are optional development tools, never bundled with aTerm.
func TestLrzszInteroperability(t *testing.T) {
	tools := os.Getenv("ATERM_LRZSZ_DIR")
	if tools == "" {
		t.Skip("set ATERM_LRZSZ_DIR to independently installed lrzsz binaries")
	}
	for _, test := range []struct {
		name   string
		upload bool
		args   []string
	}{
		{name: "download-crc32"},
		{name: "download-crc16", args: []string{"--16-bit-crc"}},
		{name: "download-sync-acks", args: []string{"--sync-transfer"}},
		{name: "upload-crc32", upload: true},
		{name: "upload-control-escaping", upload: true, args: []string{"--escape"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			incoming := filepath.Join(directory, "received")
			if err := os.Mkdir(incoming, 0700); err != nil {
				t.Fatal(err)
			}
			payload := make([]byte, 2*uploadWindow+37)
			for i := range payload {
				payload[i] = byte(i)
			}
			name := "fixture 中文.bin"
			source := filepath.Join(directory, name)
			empty := filepath.Join(directory, "empty.bin")
			if err := os.WriteFile(source, payload, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(empty, nil, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			tool := "lsz"
			if test.upload {
				tool = "lrz"
			}
			args := append([]string{"--binary", "--quiet", "--timeout", "20"}, test.args...)
			if !test.upload {
				args = append(args, source, empty)
			}
			command := exec.CommandContext(ctx, filepath.Join(tools, tool), args...)
			command.Dir = incoming
			input, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var diagnostic, text, trace bytes.Buffer
			command.Stderr = &diagnostic
			var final Progress
			offers := 0
			router := NewRouter(ctx, RouterOptions{
				Write: func(data []byte) error {
					if trace.Len() < 2048 {
						fmt.Fprintf(&trace, "write %x\n", data[:min(len(data), 128)])
					}
					_, err := input.Write(data)
					return err
				},
				Output: func(data []byte) { text.Write(data) },
				Select: func(ctx context.Context) ([]Upload, error) { return OpenUploads(ctx, []string{source, empty}) },
				Offer: func(ctx context.Context, offer FileOffer) (Download, error) {
					offers++
					return NewDownload(ctx, filepath.Join(incoming, offer.Name))
				},
				Progress: func(value Progress) { final = value },
			})
			defer router.Close()
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 32768)
			for {
				count, err := output.Read(data)
				if count > 0 {
					if trace.Len() < 2048 {
						fmt.Fprintf(&trace, "read %x\n", data[:min(count, 128)])
					}
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
				t.Fatalf("%s failed: %v, progress=%+v stderr=%s text=%q\n%s", tool, err, final, diagnostic.String(), text.String(), trace.String())
			}
			received, err := os.ReadFile(filepath.Join(incoming, name))
			if err != nil || !bytes.Equal(received, payload) {
				t.Fatalf("%s bytes differ: %v; progress=%+v stderr=%s\n%s", tool, err, final, diagnostic.String(), trace.String())
			}
			info, err := os.Stat(filepath.Join(incoming, "empty.bin"))
			if err != nil || info.Size() != 0 {
				t.Fatal("empty file missing", err)
			}
			if final.State != "completed" || router.Busy() {
				t.Fatalf("%s transfer did not complete: %+v stderr=%s text=%q\n%s", tool, final, diagnostic.String(), text.String(), trace.String())
			}
			if !test.upload && offers != 2 {
				t.Fatal("batch offers missing", offers)
			}
			if bytes.Contains(text.Bytes(), []byte{ZDLE}) || bytes.Contains(text.Bytes(), []byte{0, 1, 2, 3}) {
				t.Fatalf("%s binary frames leaked to canonical terminal: %x", tool, text.Bytes())
			}
		})
	}
}

type interruptedDownload struct {
	Download
	ctx     context.Context
	started chan struct{}
	written int
}

func (d *interruptedDownload) Write(data []byte) (int, error) {
	count, err := d.Download.Write(data)
	d.written += count
	if err == nil && d.written >= 128*1024 {
		close(d.started)
		<-d.ctx.Done()
		return count, context.Cause(d.ctx)
	}
	return count, err
}
func TestLrzszInterruptedDownloadRemovesPartialFile(t *testing.T) {
	tools := os.Getenv("ATERM_LRZSZ_DIR")
	if tools == "" {
		t.Skip("set ATERM_LRZSZ_DIR for interrupted lrzsz acceptance")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.bin")
	destination := filepath.Join(directory, "download.bin")
	payload := make([]byte, 4*1024*1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(tools, "lsz"), "--binary", "--quiet", "--timeout", "20", source)
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
	started := make(chan struct{})
	var final Progress
	router := NewRouter(ctx, RouterOptions{Write: func(data []byte) error { _, err := input.Write(data); return err }, Output: func(data []byte) { text.Write(data) }, Offer: func(ctx context.Context, _ FileOffer) (Download, error) {
		file, err := NewDownload(ctx, destination)
		if err != nil {
			return nil, err
		}
		return &interruptedDownload{Download: file, ctx: ctx, started: started}, nil
	}, Progress: func(value Progress) { final = value }})
	defer router.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		data := make([]byte, 32768)
		for {
			count, err := output.Read(data)
			if count > 0 {
				router.Consume(data[:count])
			}
			if err != nil {
				readDone <- err
				return
			}
		}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("did not begin writing a real download", ctx.Err())
	}
	_ = router.Cancel()
	if router.Busy() {
		t.Fatal("cancel retained input ownership")
	}
	select {
	case err := <-readDone:
		if err != io.EOF {
			t.Error(err)
		}
	case <-ctx.Done():
		t.Fatal("remote lsz did not stop after cancel", ctx.Err())
	}
	// Cancellation intentionally gives the independent sender a failing exit.
	if err = command.Wait(); err == nil {
		t.Fatal("lsz reported success after an interrupted transfer")
	}
	if final.State != "cancelled" || final.Transferred == 0 || final.Transferred >= int64(len(payload)) {
		t.Fatal("incorrect partial cancellation progress", final)
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("partial destination published", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(directory, ".aterm-*.zmodem"))
	if err != nil || len(leftovers) != 0 {
		t.Fatal("staging file survived cancellation", leftovers, err)
	}
	if bytes.Contains(text.Bytes(), []byte{0, 1, 2, 3}) || bytes.Contains(text.Bytes(), []byte{ZDLE}) {
		t.Logf("cancel tail: %x", text.Bytes()[max(0, text.Len()-256):])
		t.Fatalf("canceled transfer leaked wire bytes into canonical output (%d bytes): %x", text.Len(), text.Bytes()[:min(text.Len(), 256)])
	}
}

type interruptedUpload struct {
	ReadAtCloser
	ctx     context.Context
	started chan struct{}
	read    int
}

func (u *interruptedUpload) ReadAt(data []byte, offset int64) (int, error) {
	count, err := u.ReadAtCloser.ReadAt(data, offset)
	u.read += count
	if err == nil && u.read >= 128*1024 {
		close(u.started)
		<-u.ctx.Done()
		return count, context.Cause(u.ctx)
	}
	return count, err
}
func TestLrzszInterruptedUploadClosesSourceAndRemovesRemotePartial(t *testing.T) {
	tools := os.Getenv("ATERM_LRZSZ_DIR")
	if tools == "" {
		t.Skip("set ATERM_LRZSZ_DIR for interrupted lrzsz acceptance")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.bin")
	incoming := filepath.Join(directory, "received")
	if err := os.Mkdir(incoming, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, bytes.Repeat([]byte{0, 1, 2, 3}, 1024*1024), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(tools, "lrz"), "--binary", "--quiet", "--timeout", "20")
	command.Dir = incoming
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
	started := make(chan struct{})
	var final Progress
	var retained ReadAtCloser
	router := NewRouter(ctx, RouterOptions{Write: func(data []byte) error { _, err := input.Write(data); return err }, Output: func(data []byte) { text.Write(data) }, Select: func(ctx context.Context) ([]Upload, error) {
		files, err := OpenUploads(ctx, []string{source})
		if err != nil {
			return nil, err
		}
		retained = files[0].Reader
		files[0].Reader = &interruptedUpload{ReadAtCloser: retained, ctx: ctx, started: started}
		return files, nil
	}, Progress: func(value Progress) { final = value }})
	defer router.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		data := make([]byte, 32768)
		for {
			count, err := output.Read(data)
			if count > 0 {
				router.Consume(data[:count])
			}
			if err != nil {
				readDone <- err
				return
			}
		}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("did not begin a real upload", ctx.Err())
	}
	_ = router.Cancel()
	if router.Busy() {
		t.Fatal("cancel retained input ownership")
	}
	select {
	case err := <-readDone:
		if err != io.EOF {
			t.Error(err)
		}
	case <-ctx.Done():
		t.Fatal("remote lrz did not stop", ctx.Err())
	}
	if err = command.Wait(); err == nil {
		t.Fatal("lrz reported success after interrupted upload")
	}
	if final.State != "cancelled" || final.Transferred == 0 || final.Transferred >= 4*1024*1024 {
		t.Fatal("incorrect partial cancellation progress", final)
	}
	if _, err = os.Stat(filepath.Join(incoming, "source.bin")); !os.IsNotExist(err) {
		t.Fatal("remote partial survived", err, diagnostic.String())
	}
	if _, err = retained.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatal("upload source descriptor remained open", err)
	}
	if bytes.Contains(text.Bytes(), []byte{ZDLE}) || bytes.Contains(text.Bytes(), []byte{0, 1, 2, 3}) {
		t.Fatalf("canceled upload wire leaked: %x", text.Bytes())
	}
}
