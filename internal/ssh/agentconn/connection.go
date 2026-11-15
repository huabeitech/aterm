// Package agentconn owns SSH-agent endpoint selection and transport only.
// Authentication, forwarding and managed-key operations share this boundary.
package agentconn

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"time"
)

const WindowsOpenSSHPipe = `\\.\pipe\openssh-ssh-agent`

var ErrUnavailable = errors.New("no SSH agent is available; start an SSH agent and try again")

func defaultEndpoint(platform, socket string) string {
	if socket != "" {
		return socket
	}
	if platform == "windows" {
		return WindowsOpenSSHPipe
	}
	return ""
}
func DefaultEndpoint() string { return defaultEndpoint(runtime.GOOS, os.Getenv("SSH_AUTH_SOCK")) }

// Dial bounds endpoint discovery, not the lifetime of a successful connection.
// The caller owns Close and operation deadlines, including forwarding lifetime.
func Dial(ctx context.Context, endpoint string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	automatic := endpoint == "" && os.Getenv("SSH_AUTH_SOCK") == ""
	if endpoint == "" {
		endpoint = DefaultEndpoint()
	}
	if endpoint == "" {
		return nil, ErrUnavailable
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return dialEndpoint(dialCtx, endpoint, automatic)
}
