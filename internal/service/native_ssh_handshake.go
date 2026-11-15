package service

import (
	"context"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// NewClientConn does not observe ClientConfig.Timeout. Closing the transport
// on cancellation bounds both direct TCP handshakes and ProxyJump channels,
// whose net.Conn implementation does not support SetDeadline.
func nativeSSHHandshake(ctx context.Context, connection net.Conn, address string, config *ssh.ClientConfig, timeout time.Duration, authentication ...io.Closer) (*ssh.Client, error) {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(bounded, func() {
		connection.Close()
		// Authentication can be waiting on a local agent or a credential prompt.
		// Closing the transport alone cannot interrupt those waits; their owner
		// also cancels prompts and closes any lazily acquired agent connection.
		for _, socket := range authentication {
			if socket != nil {
				socket.Close()
			}
		}
	})
	clientConn, channels, requests, err := ssh.NewClientConn(connection, address, config)
	stop()
	if bounded.Err() != nil {
		if clientConn != nil {
			clientConn.Close()
		}
		connection.Close()
		return nil, bounded.Err()
	}
	if err != nil {
		connection.Close()
		return nil, err
	}
	return ssh.NewClient(clientConn, channels, requests), nil
}
