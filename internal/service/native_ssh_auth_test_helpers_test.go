package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// Use the production ClientConfig/AuthCallback in handshake tests; inspecting a
// static AuthMethod list cannot prove deferred credential selection.
func nativeAuthTestConfig(t *testing.T, ctx context.Context, host Host, directory, password string, options NativeSSHAuthOptions) (*ssh.ClientConfig, io.Closer, error) {
	t.Helper()
	// Empty SSH_AUTH_SOCK on Windows means the real system agent. Fixtures
	// requesting no agent must use a missing explicit endpoint instead.
	if runtime.GOOS == "windows" && os.Getenv("SSH_AUTH_SOCK") == "" {
		t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "absent-agent.sock"))
	}
	if host.User == "" {
		host.User = "fixture"
	}
	if options.HostKeyCallback == nil {
		options.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
	config, owner, err := nativeSSHConfig(ctx, host, directory, password, nil, options)
	if owner != nil {
		t.Cleanup(func() { owner.Close() })
	}
	return config, owner, err
}
func dialNativeAuthTest(t *testing.T, host Host, directory, password string, options NativeSSHAuthOptions, server *ssh.ServerConfig) error {
	t.Helper()
	address, key := authenticationServer(t, server)
	config, _, err := nativeAuthTestConfig(t, context.Background(), host, directory, password, options)
	if err != nil {
		return err
	}
	if options.HostKeyCallback == nil {
		config.HostKeyCallback = ssh.FixedHostKey(key)
	}
	config.Timeout = 3 * time.Second
	client, err := ssh.Dial("tcp", address, config)
	if client != nil {
		client.Close()
	}
	return err
}
