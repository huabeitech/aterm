package service

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
)

func (s *AppService) trustedHostKeyCallback(ctx context.Context, confirmer HostKeyConfirmer) ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		host, port := splitKnownHostAddress(hostname)
		received := ssh.FingerprintSHA256(key)
		expected, err := s.hostKeyStore.Lookup(host, port)
		if err != nil {
			return err
		}
		if expected == received {
			return nil
		}
		if confirmer == nil {
			return fmt.Errorf("SSH host key confirmation is required for %s", hostname)
		}
		approved, err := confirmer(ctx, HostKeyInfo{Host: host, Port: port, KeyType: key.Type(), Fingerprint: received, PreviousFingerprint: expected})
		if err != nil {
			return fmt.Errorf("confirm SSH host key for %s: %w", hostname, err)
		}
		if !approved {
			return fmt.Errorf("SSH host key was not trusted for %s", hostname)
		}
		return s.hostKeyStore.Commit(ctx, host, port, expected, received)
	}
}
