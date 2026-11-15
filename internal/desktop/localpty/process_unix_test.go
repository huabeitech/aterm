//go:build !windows

package localpty

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPTYProcessPreservesOutputSizeAndExitCode(t *testing.T) {
	process, err := Start(exec.Command("/bin/sh", "-c", "stty size; printf local-pty-marker; exit 7"), 91, 37)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if process.PID() <= 0 {
		t.Fatal("missing child PID")
	}
	output, _ := io.ReadAll(process)
	if !strings.Contains(string(output), "37 91") || !strings.Contains(string(output), "local-pty-marker") {
		t.Fatalf("unexpected terminal output %q", output)
	}
	var exited interface{ ExitCode() int }
	if err := process.Wait(); !errors.As(err, &exited) || exited.ExitCode() != 7 {
		t.Fatalf("unexpected exit: %v", err)
	}
}
func TestPTYProcessResizeInputAndKill(t *testing.T) {
	process, err := Start(exec.Command("/bin/sh", "-c", "read answer; stty size; printf 'received:%s\n' \"$answer\"; while read answer; do :; done"), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if err := process.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	output := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(process)
		var text strings.Builder
		for {
			line, err := reader.ReadString('\n')
			text.WriteString(line)
			if strings.Contains(line, "received:hello") || err != nil {
				output <- text.String()
				return
			}
		}
	}()
	select {
	case text := <-output:
		if !strings.Contains(text, "40 100") || !strings.Contains(text, "received:hello") {
			t.Fatalf("resize/input not applied: %q", text)
		}
	case <-time.After(5 * time.Second):
		process.Kill()
		process.Wait()
		t.Fatal("timed out waiting for resized PTY input")
	}

	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err == nil {
		t.Fatal("killed shell reported success")
	}
}
