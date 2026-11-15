// Package hostkeys owns application-specific SSH trust, independently of the
// user's OpenSSH known_hosts file. Its disk format matches Electron aTerm.
package hostkeys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ssh-connect/internal/fsutil"
)

const maxBytes = 2 * 1024 * 1024

var ErrChanged = errors.New("trusted SSH host key changed while confirmation was open; reconnect to verify the current fingerprint")

type Store struct {
	path string
}
type document struct {
	Version int               `json:"version"`
	Keys    map[string]string `json:"keys"`
}

func New(path string) *Store {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return &Store{path: path}
}
func identity(host, port string) string { return strings.ToLower(host) + ":" + port }
func validFingerprint(value string) bool {
	if !strings.HasPrefix(value, "SHA256:") {
		return false
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "SHA256:"))
	return err == nil && len(decoded) == 32
}
func (s *Store) read() (document, error) {
	empty := document{Version: 1, Keys: map[string]string{}}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return document{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return document{}, errors.New("invalid SSH host key store; the file was preserved")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return document{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return document{}, err
	}
	if len(data) > maxBytes {
		return document{}, errors.New("SSH host key store exceeds its size limit")
	}
	value, err := decodeDocument(data)
	if err != nil {
		return document{}, fmt.Errorf("unable to read trusted SSH host keys; the file was preserved: %w", err)
	}
	if value.Version != 1 || value.Keys == nil {
		return document{}, errors.New("invalid SSH host key store; the file was preserved")
	}
	for id, fingerprint := range value.Keys {
		if id == "" || !validFingerprint(fingerprint) {
			return document{}, errors.New("invalid trusted SSH host key; the file was preserved")
		}
	}
	return value, nil
}
func (s *Store) Lookup(host, port string) (string, error) {
	value, err := s.read()
	if err != nil {
		return "", err
	}
	return value.Keys[identity(host, port)], nil
}

// Commit is a compare-and-swap. User confirmation never holds the store lock;
// another completed verification cannot be overwritten by an older prompt.
func (s *Store) Commit(ctx context.Context, host, port, expected, received string) error {
	if host == "" || port == "" || !validFingerprint(received) {
		return errors.New("invalid SSH host key identity")
	}
	unlock, err := lockStore(ctx, s.path)
	if err != nil {
		return err
	}
	defer unlock()
	value, err := s.read()
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	id := identity(host, port)
	if value.Keys[id] == received {
		return nil
	}
	if value.Keys[id] != expected {
		return ErrChanged
	}
	value.Keys[id] = received
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > maxBytes {
		return errors.New("SSH host key store exceeds its size limit")
	}
	return fsutil.AtomicWriteFile(s.path, data, 0600)
}
