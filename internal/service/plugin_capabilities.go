package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"ssh-connect/internal/domain"
)

// OpenPluginSSH opens an independent transport using the same authentication
// and jump-host path as native terminals, without opening a shell or tab.
func (s *AppService) OpenPluginSSH(ctx context.Context, id string, confirmer HostKeyConfirmer, prompt CredentialPrompter) (*NativeSSHClient, error) {
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return nil, err
	}
	host, err := s.storeHostByID(id)
	if err != nil {
		return nil, err
	}
	return s.OpenPluginSSHHost(ctx, host, confirmer, prompt)
}
func (s *AppService) OpenPluginSSHHost(ctx context.Context, host Host, confirmer HostKeyConfirmer, prompt CredentialPrompter) (*NativeSSHClient, error) {
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return nil, err
	}
	if host.ServerAliveInterval <= 0 {
		host.ServerAliveInterval = 15
		host.ServerAliveCountMax = 3
	}
	client, _, err := s.OpenNativeSSHHostWithInteraction(ctx, host, confirmer, prompt)
	return client, err
}
func (s *AppService) PluginData(ctx context.Context, id, key string) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, ok := s.store.(interface {
		GetPluginData(string, string) (json.RawMessage, error)
	})
	if !ok {
		return json.RawMessage("null"), nil
	}
	return store.GetPluginData(id, key)
}
func (s *AppService) SavePluginData(ctx context.Context, p domain.PluginData) error {
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.store.(interface{ SetPluginData(domain.PluginData) error }).SetPluginData(p)
}

// Plugin secrets share the encrypted workspace vault but cannot address host
// passwords or another plugin's namespace. They participate in encrypted backup.
func (s *AppService) PluginSecret(ctx context.Context, id, key, operation, value string) (string, error) {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9.-]{0,98}$`).MatchString(id) || !regexp.MustCompile(`^[a-zA-Z0-9.-]{1,100}$`).MatchString(key) || len(value) > 32768 {
		return "", errors.New("invalid plugin secret")
	}
	release, err := s.workspaceRestoreGate.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	name := "plugin:" + id + ":" + key
	switch operation {
	case "get":
		value, err := s.hostPasswordStore.Get(name)
		if errors.Is(err, ErrPasswordNotFound) {
			return "", nil
		}
		return value, err
	case "set":
		return "", s.hostPasswordStore.Set(name, value)
	case "delete":
		return "", s.hostPasswordStore.Delete(name)
	default:
		return "", errors.New("invalid secret operation")
	}
}
