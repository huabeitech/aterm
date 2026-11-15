package keys

import (
	"errors"
	"strings"

	"golang.org/x/crypto/ssh"
)

// BackupKey is allowed only inside an authenticated encrypted workspace payload.
// It is never returned by list/preview/create IPC operations.
type BackupKey struct {
	Key
	PrivateKey string `json:"privateKey"`
}

func (s *Store) Backup() ([]BackupKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("managed key storage is closed")
	}
	values, _, err := s.storage.LoadKeys()
	return values, err
}
func ValidateBackup(values []BackupKey) error {
	if len(values) > 1000 {
		return errors.New("managed SSH key backup exceeds 1000 keys")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, value := range values {
		if err := validateMetadata(value.Key); err != nil {
			return err
		}
		if ids[value.ID] || names[strings.ToLower(value.Name)] {
			return errors.New("duplicate keys in managed SSH backup")
		}
		ids[value.ID] = true
		names[strings.ToLower(value.Name)] = true
		if len(value.PrivateKey) == 0 || len(value.PrivateKey) > MaxPrivateBytes {
			return errors.New("invalid private key in managed SSH backup")
		}
		signer, err := ssh.ParsePrivateKey([]byte(value.PrivateKey))
		if err == nil {
			if ssh.FingerprintSHA256(signer.PublicKey()) != value.Fingerprint || value.PassphraseProtected {
				return errors.New("private key does not match its backup metadata")
			}
		} else {
			var missing *ssh.PassphraseMissingError
			if !errors.As(err, &missing) || !value.PassphraseProtected {
				return errors.New("invalid encrypted key in managed SSH backup")
			}
			if missing.PublicKey != nil && ssh.FingerprintSHA256(missing.PublicKey) != value.Fingerprint {
				return errors.New("encrypted key fingerprint does not match its backup")
			}
		}
	}
	return nil
}
func (s *Store) RestoreBackup(values []BackupKey) error {
	if err := ValidateBackup(values); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("managed key storage is closed")
	}
	_, revision, err := s.storage.LoadKeys()
	if err != nil {
		return err
	}
	return s.storage.SaveKeys(revision, values)
}
