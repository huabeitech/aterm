//go:build darwin && !mas

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"os"
	"path/filepath"
)

// The socket exposes one signing identity for the lifetime of the lease.
type leaseSigner struct{ signer ssh.Signer }

var errUnsupported = errors.New("unsupported SSH agent operation")

func (s leaseSigner) List() ([]*agent.Key, error) {
	k := s.signer.PublicKey()
	return []*agent.Key{{Format: k.Type(), Blob: k.Marshal()}}, nil
}
func (s leaseSigner) Sign(k ssh.PublicKey, b []byte) (*ssh.Signature, error) {
	return s.SignWithFlags(k, b, 0)
}
func (s leaseSigner) SignWithFlags(k ssh.PublicKey, b []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	if !bytes.Equal(k.Marshal(), s.signer.PublicKey().Marshal()) {
		return nil, errUnsupported
	}
	if flags == 0 {
		return s.signer.Sign(rand.Reader, b)
	}
	a, ok := s.signer.(ssh.AlgorithmSigner)
	if !ok {
		return nil, errUnsupported
	}
	switch flags {
	case agent.SignatureFlagRsaSha256:
		return a.SignWithAlgorithm(rand.Reader, b, ssh.KeyAlgoRSASHA256)
	case agent.SignatureFlagRsaSha512:
		return a.SignWithAlgorithm(rand.Reader, b, ssh.KeyAlgoRSASHA512)
	}
	return nil, errUnsupported
}
func (leaseSigner) Add(agent.AddedKey) error       { return errUnsupported }
func (leaseSigner) Remove(ssh.PublicKey) error     { return errUnsupported }
func (leaseSigner) RemoveAll() error               { return errUnsupported }
func (leaseSigner) Lock([]byte) error              { return errUnsupported }
func (leaseSigner) Unlock([]byte) error            { return errUnsupported }
func (leaseSigner) Signers() ([]ssh.Signer, error) { return nil, errUnsupported }
func (leaseSigner) Extension(string, []byte) ([]byte, error) {
	return nil, agent.ErrExtensionUnsupported
}

func serveLeaseSigner(ctx context.Context, signer ssh.Signer) (string, error) {
	dir, err := os.MkdirTemp("/tmp", "aterm-key-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "agent.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	stop := context.AfterFunc(ctx, func() { l.Close(); os.RemoveAll(dir) })
	go func() {
		defer stop()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				closeOnCancel := context.AfterFunc(ctx, func() { c.Close() })
				defer closeOnCancel()
				defer c.Close()
				_ = agent.ServeAgent(leaseSigner{signer}, c)
			}()
		}
	}()
	return path, nil
}
