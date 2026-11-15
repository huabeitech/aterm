//go:build windows

package localpty

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestWindowsEnvironmentBlock(t *testing.T) {
	got, err := environmentBlock([]string{"Z=last", "a=中文", "Path=C:\\bin"})
	if err != nil {
		t.Fatal(err)
	}
	if value := string(utf16.Decode(got)); value != "a=中文\x00Path=C:\\bin\x00Z=last\x00\x00" {
		t.Fatalf("unexpected block %q", value)
	}
	if _, err := environmentBlock([]string{"A=bad\x00value"}); err == nil {
		t.Fatal("NUL accepted")
	}
}
func TestWindowsShellDiscovery(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramFiles", filepath.Join(root, "programs"))
	t.Setenv("SystemRoot", filepath.Join(root, "windows"))
	t.Setenv("COMSPEC", filepath.Join(root, "custom-cmd.exe"))
	if got, err := DefaultShell(); err != nil || got != os.Getenv("COMSPEC") {
		t.Fatal(got)
	}
	windowsPS := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	powerShell7 := filepath.Join(os.Getenv("ProgramFiles"), "PowerShell", "7", "pwsh.exe")
	for _, path := range []string{windowsPS, powerShell7} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := DefaultShell(); err != nil || got != path {
			t.Fatal(got, path)
		}
	}
}
func TestConPTYCommandOutputAndExit(t *testing.T) {
	command := exec.Command("cmd.exe", "/d", "/q", "/c", "echo aterm-conpty-marker & exit /b 7")
	process, err := Start(command, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if process.PID() <= 0 {
		t.Fatal("missing PID")
	}
	if err := process.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
	output := make(chan string, 32)
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := process.Read(buffer)
			if count > 0 {
				output <- string(buffer[:count])
			}
			if err != nil {
				close(output)
				return
			}
		}
	}()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	var text strings.Builder
	found := false
	for !found {
		select {
		case chunk, ok := <-output:
			if !ok {
				t.Fatalf("output ended before marker: %q", text.String())
			}
			text.WriteString(chunk)
			found = strings.Contains(text.String(), "aterm-conpty-marker")
		case <-deadline.C:
			t.Fatalf("timed out waiting for ConPTY: %q", text.String())
		}
	}
	exit := make(chan error, 1)
	go func() { exit <- process.Wait() }()
	select {
	case err := <-exit:
		var exited interface{ ExitCode() int }
		if !errors.As(err, &exited) || exited.ExitCode() != 7 {
			t.Fatalf("unexpected exit %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("process did not exit")
	}
}

func TestPowerShellConPTYReportsChangedUnicodeDirectory(t *testing.T) {
	shell, err := DefaultShell()
	if err != nil {
		t.Fatal(err)
	}
	nonce := "aterm-test-directory"
	args := PowerShellIntegration(shell, nonce)
	if len(args) == 0 {
		t.Skip("PowerShell not installed")
	}
	directory := filepath.Join(t.TempDir(), "space 中文 ' folder")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(shell, args...)
	command.Dir = t.TempDir()
	process, err := Start(command, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { process.Close(); process.Wait() }()
	paths := make(chan string, 16)
	parser := NewDirectoryReports(nonce, func(path string) { paths <- path })
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := process.Read(buffer)
			if count > 0 {
				parser.Consume(buffer[:count])
			}
			if err != nil {
				return
			}
		}
	}()
	if _, err := process.Write([]byte("Set-Location -LiteralPath '" + strings.ReplaceAll(directory, "'", "''") + "'\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case path := <-paths:
			if path == directory {
				return
			}
		case <-deadline.C:
			t.Fatal("PowerShell did not report changed Unicode directory")
		}
	}
}
