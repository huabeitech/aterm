package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"ssh-connect/internal/domain"
	"ssh-connect/internal/tunnel"
)

type TunnelTestResult struct {
	LocalListener string `json:"localListener"`
}

// TestTunnelConnection validates an unsaved draft using a temporary SSH transport.
// It never changes a saved configuration or stops an existing tunnel.
func (s *TunnelService) TestTunnelConnection(ctx context.Context, id string, input domain.TunnelInput, confirmer HostKeyConfirmer, prompt CredentialPrompter) (TunnelTestResult, error) {
	input = domain.NormalizeTunnelInput(input)
	if err := domain.ValidateTunnelInput(input); err != nil {
		return TunnelTestResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	host, err := s.storeHostByID(input.HostID)
	if err != nil {
		return TunnelTestResult{}, fmt.Errorf("SSH host: %w", err)
	}
	result := TunnelTestResult{LocalListener: "checked"}
	reuse := false
	if id != "" {
		store, err := s.tunnelStore()
		if err != nil {
			return TunnelTestResult{}, err
		}
		saved, err := store.GetTunnel(id)
		if err != nil {
			return TunnelTestResult{}, err
		}
		status := s.manager.Status(saved)
		active := status.ActiveConfig
		reuse = status.State == "running" && active != nil && active.LocalAddress == input.LocalAddress && active.LocalPort == input.LocalPort
	}
	if reuse {
		result.LocalListener = "existing"
	} else {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", tunnel.Address(input.LocalAddress, input.LocalPort))
		if err != nil {
			return TunnelTestResult{}, fmt.Errorf("local listener: %w", err)
		}
		defer listener.Close()
	}
	client, _, err := s.OpenNativeSSHHostWithInteraction(ctx, host, confirmer, prompt)
	if err != nil {
		return TunnelTestResult{}, fmt.Errorf("SSH connection: %w", err)
	}
	defer client.Close()
	target, err := client.DialContext(ctx, "tcp", tunnel.Address(input.TargetAddress, input.TargetPort))
	if err != nil {
		return TunnelTestResult{}, fmt.Errorf("target service: %w", err)
	}
	target.Close()
	return result, nil
}
