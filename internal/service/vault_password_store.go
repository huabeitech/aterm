package service

import (
	"errors"
	"sync"
)

type switchablePasswordStore struct {
	mu       sync.RWMutex
	delegate PasswordStore
	active   PasswordStore
}

func newSwitchablePasswordStore(delegate PasswordStore) *switchablePasswordStore {
	return &switchablePasswordStore{delegate: delegate}
}

func (s *switchablePasswordStore) Activate(active PasswordStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = active
}

func (s *switchablePasswordStore) Deactivate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = nil
}

func (s *switchablePasswordStore) Active() PasswordStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

func (s *switchablePasswordStore) current() PasswordStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active != nil {
		return s.active
	}
	return s.delegate
}

func (s *switchablePasswordStore) Get(hostID string) (string, error) {
	return s.current().Get(hostID)
}

func (s *switchablePasswordStore) Set(hostID, password string) error {
	return s.current().Set(hostID, password)
}

func (s *switchablePasswordStore) Delete(hostID string) error {
	return s.current().Delete(hostID)
}

type lockedPasswordStore struct{}

func (lockedPasswordStore) Get(hostID string) (string, error) {
	return "", ErrVaultLocked
}

func (lockedPasswordStore) Set(hostID, password string) error {
	return ErrVaultLocked
}

func (lockedPasswordStore) Delete(hostID string) error {
	return ErrVaultLocked
}

type vaultPasswordStore struct {
	mu       sync.RWMutex
	values   map[string]string
	onChange func() error
}

func newVaultPasswordStore(values map[string]string, onChange func() error) *vaultPasswordStore {
	next := make(map[string]string, len(values))
	for key, value := range values {
		next[key] = value
	}
	return &vaultPasswordStore{values: next, onChange: onChange}
}

func (s *vaultPasswordStore) Get(hostID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[hostID]
	if !ok {
		return "", ErrPasswordNotFound
	}
	return value, nil
}

func (s *vaultPasswordStore) Set(hostID, password string) error {
	s.mu.Lock()
	s.values[hostID] = password
	s.mu.Unlock()
	return s.changed()
}

func (s *vaultPasswordStore) Delete(hostID string) error {
	s.mu.Lock()
	delete(s.values, hostID)
	s.mu.Unlock()
	return s.changed()
}

func (s *vaultPasswordStore) Snapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	next := make(map[string]string, len(s.values))
	for key, value := range s.values {
		next[key] = value
	}
	return next
}

func (s *vaultPasswordStore) changed() error {
	if s.onChange == nil {
		return nil
	}
	return s.onChange()
}

func isPasswordMissing(err error) bool {
	return errors.Is(err, ErrPasswordNotFound)
}
