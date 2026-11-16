package service

import (
	"context"
	"errors"
	"ssh-connect/internal/domain"
	"ssh-connect/internal/tunnel"
)

type tunnelStore interface {
	ListTunnels() ([]domain.Tunnel, error)
	GetTunnel(string) (domain.Tunnel, error)
	CreateTunnel(domain.TunnelInput) (domain.Tunnel, error)
	UpdateTunnel(string, domain.TunnelInput) (domain.Tunnel, error)
	DeleteTunnel(string) error
}

type TunnelService struct {
	*AppService
	manager *tunnel.Manager
	changed func()
}

func (s *TunnelService) tunnelStore() (tunnelStore, error) {
	store, ok := s.store.(tunnelStore)
	if !ok {
		return nil, errors.New("tunnel storage is unavailable")
	}
	return store, nil
}

func (s *TunnelService) ListTunnels(ctx context.Context) ([]tunnel.Status, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := s.tunnelStore()
	if err != nil {
		return nil, err
	}
	configs, err := store.ListTunnels()
	if err != nil {
		return nil, err
	}
	result := make([]tunnel.Status, 0, len(configs))
	for _, config := range configs {
		result = append(result, s.manager.Status(config))
	}
	return result, nil
}

func (s *TunnelService) SaveTunnel(ctx context.Context, id string, input domain.TunnelInput) (domain.Tunnel, error) {
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return domain.Tunnel{}, err
	}
	defer release()
	store, err := s.tunnelStore()
	if err != nil {
		return domain.Tunnel{}, err
	}
	var config domain.Tunnel
	if id == "" {
		config, err = store.CreateTunnel(input)
	} else {
		config, err = store.UpdateTunnel(id, input)
	}
	if err == nil {
		s.changed()
	}
	return config, err
}

func (s *TunnelService) StartTunnel(ctx context.Context, id string, confirmer HostKeyConfirmer, prompt CredentialPrompter) error {
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return err
	}
	store, err := s.tunnelStore()
	if err != nil {
		return err
	}
	config, err := store.GetTunnel(id)
	if err != nil {
		return err
	}
	host, err := s.storeHostByID(config.HostID)
	if err != nil {
		return err
	}
	// Tunnels need liveness detection even when a terminal host disables keepalive.
	if host.ServerAliveInterval <= 0 {
		host.ServerAliveInterval = 15
		host.ServerAliveCountMax = 3
	}
	return s.manager.Start(config, func(connectionCtx context.Context) (tunnel.Transport, error) {
		client, _, err := s.OpenNativeSSHHostWithInteraction(connectionCtx, host, confirmer, prompt)
		if err != nil {
			return nil, err
		}
		return client, nil
	})
}

func (s *TunnelService) StopTunnel(ctx context.Context, id string) error {
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	s.manager.Stop(id)
	return nil
}

func (s *TunnelService) DeleteTunnel(ctx context.Context, id string) error {
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	store, err := s.tunnelStore()
	if err != nil {
		return err
	}
	s.manager.Forget(id)
	if err = store.DeleteTunnel(id); err != nil {
		return err
	}
	s.changed()
	return nil
}

func (s *TunnelService) CloseTunnels() { s.manager.Close() }
