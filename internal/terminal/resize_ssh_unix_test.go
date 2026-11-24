//go:build !windows

package terminal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
	"ssh-connect/internal/service"
)

func TestSSHInitialPTYSizeFillsVi(t *testing.T) {
	if _, err := exec.LookPath("vim"); err != nil {
		t.Skip("Vim is required for the SSH editor integration test")
	}
	t.Setenv("SSH_AUTH_SOCK", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.MarshalPrivateKey(private, "SSH PTY fixture")
	if err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(dir, "client.key")
	if err := os.WriteFile(identity, pem.EncodeToMemory(key), 0600); err != nil {
		t.Fatal(err)
	}

	shell := exec.CommandContext(ctx, "sh")
	shell.Dir = dir
	shell.Env = append(os.Environ(), "TERM=xterm-256color", "LINES=", "COLUMNS=", "ENV=")
	ptyRequested := make(chan pty.Winsize, 1)
	acceptPTY := make(chan struct{})
	address := terminalSSHPeer(t, ctx, shell, signer.PublicKey(), func(size pty.Winsize) {
		ptyRequested <- size
		select {
		case <-acceptPTY:
		case <-ctx.Done():
		}
	})
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	svc := sshServiceFixture(t)
	const alias = "pty-fixture"
	if _, err := svc.CreateHost(ctx, service.HostInput{Name: alias, Alias: alias, HostName: host, Port: port, User: "fixture", IdentityFile: identity}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, func(string, any) {})
	defer manager.CloseAll()
	startConnection := make(chan struct{})
	entry, err := manager.OpenSSHAsync(alias, func(ctx context.Context, _ service.HostKeyConfirmer) (*service.NativeSSHClient, string, error) {
		select {
		case <-startConnection:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		client, _, err := svc.OpenNativeSSHWithHostKeyConfirmer(ctx, alias, func(context.Context, service.HostKeyInfo) (bool, error) { return true, nil })
		return client, alias, err
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.Resize(entry.ID, 140, 55); err != nil {
		t.Fatal(err)
	}
	close(startConnection)
	select {
	case size := <-ptyRequested:
		if size.Cols != 140 || size.Rows != 55 {
			t.Fatalf("initial PTY size = %dx%d, want 140x55", size.Cols, size.Rows)
		}
	case <-ctx.Done():
		t.Fatal("SSH client did not request a PTY")
	}
	// Simulate a final layout measurement arriving while the SSH server opens the PTY.
	if err := manager.Resize(entry.ID, 150, 64); err != nil {
		t.Fatal(err)
	}
	close(acceptPTY)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	wait := func() {
		t.Helper()
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("timed out waiting for the SSH editor")
		}
	}
	for {
		snapshot, err := manager.Snapshot(entry.ID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.State == "running" {
			break
		}
		if snapshot.State == "exited" {
			t.Fatal("SSH session exited during setup")
		}
		wait()
	}

	// Run Vim in its normal screen mode with a single editing window.
	command := "stty size > pty-size; vim -u NONE -i NONE -n -c 'call writefile([string(&lines), string(winheight(0))], \"vi-size\")' -c 'qa!' sample.txt\r"
	if err := manager.Write(entry.ID, base64.StdEncoding.EncodeToString([]byte(command))); err != nil {
		t.Fatal(err)
	}
	for {
		screen, err := os.ReadFile(filepath.Join(dir, "vi-size"))
		if err == nil && strings.Count(string(screen), "\n") == 2 {
			actualPTY, err := os.ReadFile(filepath.Join(dir, "pty-size"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(actualPTY)) != "64 150" || string(screen) != "64\n63\n" {
				t.Fatalf("stty size = %q, Vim lines and editing rows = %q", actualPTY, screen)
			}
			break
		}
		wait()
	}
}
