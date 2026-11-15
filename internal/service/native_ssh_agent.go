package service

import (
	"context"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"ssh-connect/internal/ssh/agentconn"
)

// Forwarding owns a separate agent connection: authentication's temporary
// connection may be closed as soon as the SSH handshake has finished.
func nativeSSHAgentForwarding(ctx context.Context, client *ssh.Client, host Host) (net.Conn, error) {
	if !host.ForwardAgent {
		return nil, nil
	}
	connection, err := agentconn.Dial(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("connect SSH forwarding agent: %w", err)
	}
	if err := agent.ForwardToAgent(client, agent.NewClient(connection)); err != nil {
		connection.Close()
		return nil, fmt.Errorf("configure SSH agent forwarding: %w", err)
	}
	return connection, nil
}
