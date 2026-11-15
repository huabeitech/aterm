package service

import (
	"context"
	"errors"
)

var (
	ErrPasswordNotFound              = errors.New("saved SSH password not found")
	ErrPasswordStoreUnavailable      = errors.New("secure system storage is unavailable")
	ErrKeychainUserCanceled          = errors.New("keychain access canceled")
	ErrKeychainInteractionNotAllowed = errors.New("keychain interaction not allowed")
	ErrKeychainAccessDenied          = errors.New("keychain access denied")
)

// PasswordStore keeps SSH passwords outside the application database. Host
// configuration can be synced safely because the secret remains device-local.
type PasswordStore interface {
	Get(hostID string) (string, error)
	Set(hostID, password string) error
	Delete(hostID string) error
}

// NewSystemSecretStore exposes the OS-protected credential adapter for other
// services; it never falls back to plaintext storage.
func NewSystemSecretStore(ctx context.Context) PasswordStore { return newPasswordStore(ctx) }
