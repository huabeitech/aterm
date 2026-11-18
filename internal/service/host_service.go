package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type HostService struct {
	store         HostStore
	passwordStore PasswordStore
}

func NewHostService(store HostStore, passwordStore PasswordStore) *HostService {
	return &HostService{store: store, passwordStore: passwordStore}
}

func (s *HostService) ListHosts(ctx context.Context) ([]Host, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.store.ListHosts()
}

func (s *HostService) CreateHost(ctx context.Context, input HostInput) (Host, error) {
	if err := ctx.Err(); err != nil {
		return Host{}, err
	}
	input, err := s.normalizeJumpHost(input, "")
	if err != nil {
		return Host{}, err
	}
	if input.PasswordAuth && input.Password == "" {
		return Host{}, fmt.Errorf("enter a password before enabling password authentication")
	}
	if input.PasswordAuth {
		if err := s.ensurePasswordStoreUnlocked(); err != nil {
			return Host{}, err
		}
	}
	host, err := s.store.CreateHost(input)
	if err != nil {
		return Host{}, err
	}
	if input.PasswordAuth {
		if err := s.savePassword(host, input, false); err != nil {
			_ = s.store.DeleteHost(host.ID)
			return Host{}, err
		}
	}
	return host, nil
}

func (s *HostService) UpdateHost(ctx context.Context, id string, input HostInput) (Host, error) {
	if err := ctx.Err(); err != nil {
		return Host{}, err
	}
	existing, err := s.hostByID(id)
	if err != nil {
		return Host{}, err
	}
	input, err = s.normalizeJumpHost(input, id)
	if err != nil {
		return Host{}, err
	}
	if input.PasswordAuth && input.Password == "" {
		if !existing.PasswordAuth {
			return Host{}, fmt.Errorf("enter a password before enabling password authentication")
		}
	}
	if passwordChangeRequiresUnlockedVault(existing, input) {
		if err := s.ensurePasswordStoreUnlocked(); err != nil {
			return Host{}, err
		}
	}
	host, err := s.store.UpdateHost(id, input)
	if err != nil {
		return Host{}, err
	}
	if err := s.savePassword(host, input, existing.PasswordAuth); err != nil {
		return Host{}, err
	}
	return host, nil
}

func (s *HostService) MoveHosts(ctx context.Context, ids []string, directoryID string) ([]Host, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.store.MoveHosts(ids, strings.TrimSpace(directoryID))
}

func (s *HostService) ReorderHosts(ctx context.Context, scopeType, scopeID string, ids []string) ([]Host, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.store.ReorderHosts(strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), ids)
}

func (s *HostService) DeleteHost(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureJumpHostNotInUse(id); err != nil {
		return err
	}
	host, err := s.hostByID(id)
	if err != nil {
		return err
	}
	if host.PasswordAuth {
		if err := s.ensurePasswordStoreUnlocked(); err != nil {
			return err
		}
	}
	if err := s.store.DeleteHost(id); err != nil {
		return err
	}
	if host.PasswordAuth {
		if err := s.passwordStore.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *HostService) GetHostPassword(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	host, err := s.hostByID(id)
	if err != nil {
		return "", err
	}
	if !host.PasswordAuth {
		return "", ErrPasswordNotFound
	}
	password, err := s.passwordStore.Get(host.ID)
	if errors.Is(err, ErrPasswordNotFound) {
		return "", errors.New("the saved SSH password is unavailable; enter it again in the host settings")
	}
	return password, err
}

func (s *HostService) normalizeJumpHost(input HostInput, hostID string) (HostInput, error) {
	input.JumpHostID = strings.TrimSpace(input.JumpHostID)
	if input.JumpHostID == "" {
		return input, nil
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return HostInput{}, err
	}
	byID := make(map[string]Host, len(hosts))
	for _, host := range hosts {
		byID[host.ID] = host
	}
	jump, ok := byID[input.JumpHostID]
	if !ok {
		return HostInput{}, fmt.Errorf("selected jump host no longer exists")
	}
	if input.JumpHostID == hostID {
		return HostInput{}, fmt.Errorf("a host cannot use itself as its jump host")
	}
	for seen := map[string]bool{}; jump.JumpHostID != ""; {
		if seen[jump.ID] || jump.JumpHostID == hostID {
			return HostInput{}, fmt.Errorf("jump host chain contains a cycle")
		}
		seen[jump.ID] = true
		next, ok := byID[jump.JumpHostID]
		if !ok {
			break
		}
		jump = next
	}
	input.ProxyJump = jumpAlias(byID[input.JumpHostID])
	return input, nil
}

func (s *HostService) ensureJumpHostNotInUse(id string) error {
	return s.ensureJumpHostsNotInUse([]string{id})
}

func (s *HostService) ensureJumpHostsNotInUse(ids []string) error {
	deleted := make(map[string]bool, len(ids))
	for _, id := range ids {
		deleted[id] = true
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return err
	}
	byID := make(map[string]Host, len(hosts))
	for _, host := range hosts {
		byID[host.ID] = host
	}
	for _, host := range hosts {
		if host.JumpHostID != "" && deleted[host.JumpHostID] && !deleted[host.ID] {
			return fmt.Errorf("%s is used as a jump host by %s", byID[host.JumpHostID].DisplayName(), host.DisplayName())
		}
	}
	return nil
}

func jumpAlias(host Host) string { return strings.TrimSpace(host.Alias) }

func (s *HostService) DeleteHosts(ctx context.Context, ids []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureJumpHostsNotInUse(ids); err != nil {
		return err
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return err
	}
	passwordHostIDs := map[string]bool{}
	deleting := map[string]bool{}
	for _, id := range ids {
		deleting[id] = true
	}
	for _, host := range hosts {
		if deleting[host.ID] && host.PasswordAuth {
			passwordHostIDs[host.ID] = true
		}
	}
	if len(passwordHostIDs) > 0 {
		if err := s.ensurePasswordStoreUnlocked(); err != nil {
			return err
		}
	}
	if err := s.store.DeleteHosts(ids); err != nil {
		return err
	}
	for _, id := range ids {
		if passwordHostIDs[id] {
			if err := s.passwordStore.Delete(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *HostService) savePassword(host Host, input HostInput, hadPasswordAuth bool) error {
	if !input.PasswordAuth {
		if !hadPasswordAuth {
			return nil
		}
		return s.passwordStore.Delete(host.ID)
	}
	if input.Password == "" {
		return nil
	}
	return s.passwordStore.Set(host.ID, input.Password)
}

func passwordChangeRequiresUnlockedVault(existing Host, input HostInput) bool {
	if input.PasswordAuth && input.Password != "" {
		return true
	}
	return existing.PasswordAuth && !input.PasswordAuth
}

func (s *HostService) hostByID(id string) (Host, error) {
	hosts, err := s.store.ListHosts()
	if err != nil {
		return Host{}, err
	}
	for _, host := range hosts {
		if host.ID == id {
			return host, nil
		}
	}
	return Host{}, ErrHostNotFound
}

func (s *HostService) ensurePasswordStoreUnlocked() error {
	if store, ok := s.passwordStore.(*switchablePasswordStore); ok {
		if store.Active() == nil {
			return ErrVaultLocked
		}
	}
	return nil
}
