package terminalmeta

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNullDelimitedDirectoriesPreserveUnicodeAndLineBreaks(t *testing.T) {
	paths := ParseWorkingDirectories("p12\x00\nn/space 中文\x00\np13\x00\nn/path\nwith newline\x00\npinvalid\x00n/ignored\x00")
	if paths[12] != "/space 中文" || paths[13] != "/path\nwith newline" || len(paths) != 2 {
		t.Fatal(paths)
	}
}
func TestReadRealProcessWorkingDirectory(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix process metadata")
	}
	directory := filepath.Join(t.TempDir(), "space 中文")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", "-c", "read fixture")
	command.Dir = directory
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { command.Process.Kill(); command.Wait() }()
	got := ReadWorkingDirectories(ctx, []int{command.Process.Pid})[command.Process.Pid]
	resolved, _ := filepath.EvalSymlinks(directory)
	if got != directory && got != resolved {
		t.Fatalf("process %s cwd=%q, expected %q", strconv.Itoa(command.Process.Pid), got, directory)
	}
}
func TestBoundedMetadataOutputCannotBypassWriterLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := &boundedBuffer{limit: 16, cancel: cancel}
	if _, err := io.Copy(output, strings.NewReader(strings.Repeat("x", 17))); err == nil || ctx.Err() == nil {
		t.Fatal("oversized metadata output was accepted")
	}
}
func TestRemoteDirectoryValidation(t *testing.T) {
	for _, text := range []string{"/space 中文\n", "/path\r\n", "/"} {
		if ParseRemoteDirectory(text) == "" {
			t.Fatal("valid remote path rejected", text)
		}
	}
	for _, text := range []string{"home\n", "/first\n/second\n", "/null\x00", "/carriage\r", "/" + strings.Repeat("x", 8192)} {
		if ParseRemoteDirectory(text) != "" {
			t.Fatal("invalid remote path accepted")
		}
	}
}
