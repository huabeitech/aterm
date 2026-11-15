package hostkeys

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestTrustStoreProcessHelper(t *testing.T) {
	path := os.Getenv("ATERM_TRUST_TEST_PATH")
	if path == "" {
		return
	}
	switch os.Getenv("ATERM_TRUST_TEST_MODE") {
	case "hold":
		unlock, err := lockStore(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		fmt.Println("locked")
		var data [1]byte
		_, _ = os.Stdin.Read(data[:])
	case "replace":
		if err := New(path).Commit(context.Background(), "host", "22", fingerprint('a'), fingerprint('c')); err != nil {
			t.Fatal(err)
		}
	case "commit":
		prefix := os.Getenv("ATERM_TRUST_TEST_PREFIX")
		for i := 0; i < 10; i++ {
			if err := New(path).Commit(context.Background(), prefix+"-"+strconv.Itoa(i), "22", "", fingerprint('a')); err != nil {
				t.Fatal(err)
			}
		}
	default:
		t.Fatal("invalid subprocess fixture mode")
	}
}
func trustProcess(t *testing.T, ctx context.Context, path, mode string) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTrustStoreProcessHelper$", "-test.timeout=10s")
	command.Env = append(os.Environ(), "ATERM_TRUST_TEST_PATH="+path, "ATERM_TRUST_TEST_MODE="+mode)
	return command
}
func TestTrustCommitsAcrossProcessesPreserveAllHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	commands := make([]*exec.Cmd, 6)
	for i := range commands {
		commands[i] = trustProcess(t, ctx, path, "commit")
		commands[i].Env = append(commands[i].Env, fmt.Sprintf("ATERM_TRUST_TEST_PREFIX=process-%d", i))
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatal("trust subprocess failed", err)
		}
	}
	for i := range commands {
		for j := 0; j < 10; j++ {
			host := fmt.Sprintf("process-%d-%d", i, j)
			if got, err := New(path).Lookup(host, "22"); err != nil || got != fingerprint('a') {
				t.Fatal("lost cross-process trust update", host, got, err)
			}
		}
	}
}
func TestTrustLockCancellationAndProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := trustProcess(t, ctx, path, "hold")
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatal("subprocess did not acquire lock", line, err)
	}
	waiting, stop := context.WithTimeout(ctx, 35*time.Millisecond)
	err = New(path).Commit(waiting, "host", "22", "", fingerprint('a'))
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock wait ignored cancellation: %v", err)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled wait wrote trust data")
	}
	if err = command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	// No stale-lock deletion or PID recovery is necessary after an abrupt exit.
	if err = New(path).Commit(ctx, "host", "22", "", fingerprint('a')); err != nil {
		t.Fatal("process exit retained trust lock", err)
	}
}

func TestStaleApprovalAfterAnotherProcessReplacesKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
	store := New(path)
	if err := store.Commit(context.Background(), "host", "22", "", fingerprint('a')); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if output, err := trustProcess(t, ctx, path, "replace").CombinedOutput(); err != nil {
		t.Fatalf("replacement subprocess: %v: %s", err, output)
	}
	if err := store.Commit(ctx, "host", "22", fingerprint('a'), fingerprint('b')); !errors.Is(err, ErrChanged) {
		t.Fatal("stale approval overwrote another process", err)
	}
	if got, err := store.Lookup("host", "22"); err != nil || got != fingerprint('c') {
		t.Fatal("newer cross-process trust lost", got, err)
	}
}
