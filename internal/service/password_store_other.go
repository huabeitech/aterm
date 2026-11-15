//go:build !darwin

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const passwordStoreService = "pro.huabei.sshconfig.term"

type systemPasswordStore struct{}

func newPasswordStore(_ context.Context) PasswordStore { return systemPasswordStore{} }

func (systemPasswordStore) Get(account string) (string, error) {
	value, err := keyring.Get(passwordStoreService, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrPasswordNotFound
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPasswordStoreUnavailable, err)
	}
	return value, nil
}

func (systemPasswordStore) Set(account, password string) error {
	if err := keyring.Set(passwordStoreService, account, password); err != nil {
		return fmt.Errorf("%w: %v", ErrPasswordStoreUnavailable, err)
	}
	return nil
}

func (systemPasswordStore) Delete(account string) error {
	err := keyring.Delete(passwordStoreService, account)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrPasswordStoreUnavailable, err)
}
