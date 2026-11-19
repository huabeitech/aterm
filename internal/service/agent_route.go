package service

import (
	"context"
	"errors"
	"golang.org/x/crypto/ssh"
	"net"
	"strconv"
	"strings"

	desktopkeys "ssh-connect/internal/desktop/keys"
)

// AgentRoute holds the saved destination and jump hosts displayed for approval.
// Only the display endpoints are exposed to the confirmation UI.
type AgentRoute struct {
	hosts []Host
}

func (s *AppService) PrepareAgentRoute(ctx context.Context, hostID string) (AgentRoute, error) {
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return AgentRoute{}, err
	}
	target, err := s.storeHostByID(hostID)
	if err != nil {
		return AgentRoute{}, err
	}
	hosts, err := s.resolveSSHRoute(target, 0)
	return AgentRoute{hosts: hosts}, err
}

func (r AgentRoute) Endpoints() []string {
	endpoints := make([]string, len(r.hosts))
	for i, host := range r.hosts {
		address := net.JoinHostPort(host.HostName, strconv.Itoa(resolvedSSHPort(host.Port)))
		if host.User != "" {
			address = host.User + "@" + address
		}
		endpoints[i] = address
	}
	return endpoints
}

func (s *AppService) AgentRouteRequiresUnlock(ctx context.Context, r AgentRoute) (bool, error) {
	for _, host := range r.hosts {
		if !host.PasswordAuth && strings.HasPrefix(host.IdentityFile, desktopkeys.Prefix) {
			status, err := s.GetEncryptedSyncStatus(ctx)
			return !status.Unlocked, err
		}
	}
	return false, nil
}

func (r AgentRoute) Hosts() []Host { return append([]Host(nil), r.hosts...) }

func (s *AppService) ManagedAgentSigner(ctx context.Context, identity string, prompt CredentialPrompter) (ssh.Signer, error) {
	if !strings.HasPrefix(identity, desktopkeys.Prefix) {
		return nil, errors.New("managed identity required")
	}
	return loadNativeSSHIdentity(ctx, identity, true, NativeSSHAuthOptions{ReadIdentity: s.identityReader, IdentityName: s.identityNameReader, Prompt: prompt})
}
