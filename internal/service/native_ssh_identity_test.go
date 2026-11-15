package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func encryptedIdentityFixture(t *testing.T) []byte {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "disposable fixture", []byte("correct"))
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}
func TestNativeSelectedIdentityFailuresDoNotFallBack(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	directory := t.TempDir()
	malformed := filepath.Join(directory, "bad-key")
	if err := os.WriteFile(malformed, []byte("not a private key"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, vector := range []struct{ path, want string }{
		{filepath.Join(directory, "missing"), "Identity file does not exist"},
		{malformed, errNativeIdentityFormat.Error()},
	} {
		err := dialNativeAuthTest(t, Host{IdentityFile: vector.path}, directory, "", NativeSSHAuthOptions{Prompt: func(context.Context, CredentialPrompt) ([]string, error) {
			t.Fatal("plain/missing identity prompted for unrelated credentials")
			return nil, nil
		}}, &ssh.ServerConfig{
			PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
				return nil, errors.New("fixture rejects keys")
			},
			PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
				t.Error("invalid selected key fell back to password")
				return nil, errors.New("unexpected password")
			},
		})
		if err == nil || !strings.Contains(err.Error(), vector.want) {
			t.Fatalf("%s: error=%v", vector.path, err)
		}
	}
	result, err := loadNativeSSHIdentity(context.Background(), malformed, false, NativeSSHAuthOptions{})
	if err != nil || result != nil {
		t.Fatal("invalid discovered default was not skipped", err)
	}
}
func TestNativeIdentityReadSizeBoundaryAndSymlink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "identity")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxNativeIdentityBytes), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readNativeIdentityFile(path)
	if err != nil || len(data) != maxNativeIdentityBytes {
		t.Fatal(len(data), err)
	}
	clear(data)
	if err := os.Truncate(path, maxNativeIdentityBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := readNativeIdentityFile(path); !errors.Is(err, errNativeIdentityTooLarge) {
		t.Fatal(err)
	}
	for _, explicit := range []bool{true, false} {
		if _, err := loadNativeSSHIdentity(context.Background(), path, explicit, NativeSSHAuthOptions{}); !errors.Is(err, errNativeIdentityTooLarge) {
			t.Fatal("oversized key ignored", explicit, err)
		}
	}
	if _, err := readNativeIdentityFile(directory); err == nil {
		t.Fatal("directory accepted as identity")
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(directory, "linked-key")
	if err := os.Symlink(path, linked); err != nil {
		t.Skip("symlink fixture unavailable", err)
	}
	data, err = readNativeIdentityFile(linked)
	if err != nil || string(data) != "fixture" {
		t.Fatal(string(data), err)
	}
}
func TestNativeEncryptedIdentityPromptsOnceAndClearsOwnedBytes(t *testing.T) {
	fixture := encryptedIdentityFixture(t)
	for _, managed := range []bool{true, false} {
		for _, correct := range []bool{true, false} {
			path := "aterm-key://fixture"
			owned := append([]byte(nil), fixture...)
			options := NativeSSHAuthOptions{ReadIdentity: func(string) ([]byte, error) { return owned, nil }}
			if !managed {
				path = filepath.Join(t.TempDir(), "encrypted-key")
				if err := os.WriteFile(path, owned, 0600); err != nil {
					t.Fatal(err)
				}
			}
			prompts := 0
			options.Prompt = func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
				prompts++
				if prompt.Title != "Unlock Private Key" || prompt.Message != "Enter the passphrase for "+path+"." || len(prompt.Fields) != 1 || !prompt.Fields[0].Secret {
					t.Fatal("wrong passphrase prompt", prompt)
				}
				if correct {
					return []string{"correct"}, nil
				}
				return []string{"incorrect"}, nil
			}
			result, err := loadNativeSSHIdentity(context.Background(), path, true, options)
			if prompts != 1 {
				t.Fatal("passphrase unexpectedly retried", prompts)
			}
			if correct && (err != nil || result == nil) {
				t.Fatal("correct passphrase rejected", err)
			}
			if !correct && (err == nil || result != nil) {
				t.Fatal("incorrect passphrase silently skipped", err)
			}
			if managed && !bytes.Equal(owned, make([]byte, len(owned))) {
				t.Fatal("owned managed key bytes retained")
			}
		}
	}
}
func TestNativeIdentityCancellationAndOversizedManagedBuffer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadNativeSSHIdentity(ctx, "aterm-key://fixture", true, NativeSSHAuthOptions{ReadIdentity: func(string) ([]byte, error) { t.Fatal("canceled lookup read private material"); return nil, nil }}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	owned := bytes.Repeat([]byte{'x'}, maxNativeIdentityBytes+1)
	if _, err := loadNativeSSHIdentity(context.Background(), "aterm-key://fixture", true, NativeSSHAuthOptions{ReadIdentity: func(string) ([]byte, error) { return owned, nil }}); !errors.Is(err, errNativeIdentityTooLarge) {
		t.Fatal(err)
	}
	if !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("oversized managed buffer not cleared")
	}
	owned = encryptedIdentityFixture(t)
	if _, err := loadNativeSSHIdentity(context.Background(), "aterm-key://fixture", true, NativeSSHAuthOptions{ReadIdentity: func(string) ([]byte, error) { return owned, nil }, Prompt: func(context.Context, CredentialPrompt) ([]string, error) { return nil, context.Canceled }}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("canceled managed key buffer retained")
	}
}
