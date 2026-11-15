package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

const maxNativeIdentityBytes = 1024 * 1024

var errNativeIdentityTooLarge = errors.New("Identity file is too large")
var errNativeIdentityFormat = errors.New("Unable to read private key. Check its format and passphrase.")
var errNativeManagedIdentityFormat = errors.New("Unable to read the selected managed SSH key.")

// A configured identity is a user choice: failure is an error, not permission
// to silently skip it. Invalid default-discovery files can be skipped. Both
// modes use the renderer's one-prompt rule and a bounded, owned key buffer.
func loadNativeSSHIdentity(ctx context.Context, path string, explicit bool, options NativeSSHAuthOptions) (ssh.Signer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	managed := strings.HasPrefix(path, "aterm-key://")
	var data []byte
	var err error
	if managed {
		if options.ReadIdentity == nil {
			return nil, errors.New("managed SSH key storage is unavailable")
		}
		data, err = options.ReadIdentity(path)
	} else {
		data, err = readNativeIdentityFile(path)
	}
	if data != nil {
		defer clear(data)
	}
	if err != nil {
		if errors.Is(err, errNativeIdentityTooLarge) {
			return nil, err
		}
		if managed {
			return nil, err
		}
		if explicit {
			if errors.Is(err, os.ErrNotExist) {
				return nil, errors.New("Identity file does not exist")
			}
			return nil, fmt.Errorf("read identity file: %w", err)
		}
		return nil, nil
	}
	if len(data) > maxNativeIdentityBytes {
		return nil, errNativeIdentityTooLarge
	}
	signer, parseErr := ssh.ParsePrivateKey(data)
	var missing *ssh.PassphraseMissingError
	if errors.As(parseErr, &missing) {
		if options.Prompt != nil {
			name := path
			if managed && options.IdentityName != nil {
				if label := options.IdentityName(path); label != "" {
					name = label
				}
			}
			answers, err := options.Prompt(ctx, CredentialPrompt{Title: "Unlock Private Key", Message: "Enter the passphrase for " + name + ".", Fields: []CredentialField{{Label: "Passphrase", Secret: true}}})
			if err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if len(answers) != 1 {
				return nil, errors.New("invalid private key passphrase response")
			}
			passphrase := []byte(answers[0])
			signer, parseErr = ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
			clear(passphrase)
		}
	}
	if parseErr != nil {
		if managed {
			return nil, errNativeManagedIdentityFormat
		}
		if explicit {
			return nil, errNativeIdentityFormat
		}
		return nil, nil
	}
	signer, err = nativeCompatibleSigner(signer)
	if err != nil {
		if explicit || managed {
			return nil, err
		}
		return nil, nil
	}
	return signer, nil
}

func readNativeIdentityFile(path string) ([]byte, error) {
	// Reject ordinary directories/devices/FIFOs before opening them. Stat follows
	// symlinks, allowing the standard symlinked ~/.ssh identity layout.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("identity must be a regular file")
	}
	if info.Size() > maxNativeIdentityBytes {
		return nil, errNativeIdentityTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	current, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !current.Mode().IsRegular() {
		return nil, errors.New("identity must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxNativeIdentityBytes+1))
	if err != nil {
		clear(data)
		return nil, err
	}
	if len(data) > maxNativeIdentityBytes {
		clear(data)
		return nil, errNativeIdentityTooLarge
	}
	return data, nil
}
