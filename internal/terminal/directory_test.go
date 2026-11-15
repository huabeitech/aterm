package terminal

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTerminalDirectoryFollowsRealShellCD(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix PTY metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	manager := NewManager(ctx, nil)
	defer manager.CloseAll()
	root := t.TempDir()
	directory := filepath.Join(root, "space 中文")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", "-i")
	command.Dir = root
	session, err := manager.openWithOptions("local", "Local", "", command, LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	input := "cd '" + strings.ReplaceAll(directory, "'", "'\\''") + "'\r"
	if err = manager.Write(session.ID, base64.StdEncoding.EncodeToString([]byte(input))); err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(directory)
	for {
		manager.mu.RLock()
		entry := manager.sessions[session.ID]
		cwd, title := entry.Cwd, entry.Title
		manager.mu.RUnlock()
		if cwd == directory || cwd == resolved {
			if title != "space 中文" {
				t.Fatal("directory title not updated", title)
			}
			view, err := manager.Snapshot(session.ID)
			if err != nil || view.PID != command.Process.Pid || view.Shell != "/bin/sh" || view.Cwd != cwd {
				t.Fatal("shared renderer metadata contract missing", view, err)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("shell directory change was not observed", cwd)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
func TestTerminalEnvironmentStripsBootstrapOptions(t *testing.T) {
	result := terminalEnvironment([]string{"PATH=/bin", "ZDOTDIR=/fixture", "NODE_OPTIONS=--inspect", "ATERM_DATA_DIR=/private-profile", "TERM=old", "TERM_PROGRAM=old"})
	joined := strings.Join(result, "\n")
	if strings.Contains(joined, "NODE_OPTIONS=") || strings.Contains(joined, "ATERM_DATA_DIR=") || strings.Contains(joined, "TERM=old") {
		t.Fatal("bootstrap environment leaked", joined)
	}
	for _, expected := range []string{"PATH=/bin", "ZDOTDIR=/fixture", "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=aTerm"} {
		if !strings.Contains(joined, expected) {
			t.Fatal("terminal environment missing", expected)
		}
	}
}
