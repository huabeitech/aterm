package keys

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"ssh-connect/internal/ssh/agentconn"
)

func (s *Store) Metadata(id string) (Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.unload()
	return s.key(id)
}
func (s *Store) Export(id, destination string) error {
	bytes, err := s.ReadIdentity(Prefix + id)
	if err != nil {
		return err
	}
	defer clear(bytes)
	return WritePrivateExport(destination, bytes)
}

// WritePrivateExport writes through the file granted by the native Save panel.
// It does not create a sibling temporary file outside that sandbox grant.
func WritePrivateExport(destination string, bytes []byte) error {
	info, err := os.Lstat(destination)
	var file *os.File
	if os.IsNotExist(err) {
		file, err = os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	} else if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("select a regular file for private key export")
		}
		file, err = os.OpenFile(destination, os.O_WRONLY, 0)
		if err == nil {
			actual, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, actual) {
				file.Close()
				return errors.New("the selected export file changed; select it again")
			}
		}
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Write(bytes); err != nil {
		return err
	}
	return file.Sync()
}
func (s *Store) AddToAgent(ctx context.Context, id, passphrase, socket string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(passphrase) > 4096 {
		return errors.New("private key passphrase is too long")
	}
	connection, err := agentconn.Dial(ctx, socket)
	if err != nil {
		return fmt.Errorf("connect to SSH agent: %w", err)
	}
	defer connection.Close()
	if agentconn.IsPageant(connection) {
		return errors.New("adding managed keys to Pageant is unavailable; use the Windows OpenSSH agent")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	key, err := s.Metadata(id)
	if err != nil {
		return err
	}
	bytes, err := s.ReadIdentity(Prefix + id)
	if err != nil {
		return err
	}
	defer clear(bytes)
	raw, err := ssh.ParseRawPrivateKey(bytes)
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			raw, err = ssh.ParseRawPrivateKeyWithPassphrase(bytes, []byte(passphrase))
		}
	}
	if err != nil {
		return errors.New("enter the correct private key passphrase")
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return err
	}
	if ssh.FingerprintSHA256(signer.PublicKey()) != key.Fingerprint {
		return errors.New("protected private key does not match its catalog fingerprint")
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return agent.NewClient(connection).Add(agent.AddedKey{PrivateKey: raw, Comment: key.Name})
}
