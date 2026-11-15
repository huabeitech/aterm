package service

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"ssh-connect/internal/ssh/agentconn"
)

type nativeAuthStep struct {
	kind   string
	signer ssh.Signer
}

// One plan belongs to one handshake. x/crypto calls next serially after host
// verification and each server authentication response. Close can run from the
// handshake timeout goroutine and cancels any in-flight prompt/agent operation.
type nativeSSHAuthPlan struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	host                Host
	directory, password string
	options             NativeSSHAuthOptions
	steps               []nativeAuthStep
	mu                  sync.Mutex
	connection          net.Conn
	closed              bool
}

func newNativeSSHAuthPlan(ctx context.Context, host Host, directory, password string, options NativeSSHAuthOptions) (*nativeSSHAuthPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if host.PasswordAuth && password == "" && options.Prompt == nil {
		return nil, errors.New("no saved SSH password was found")
	}
	owned, cancel := context.WithCancel(ctx)
	p := &nativeSSHAuthPlan{ctx: owned, cancel: cancel, host: host, directory: directory, password: password, options: options}
	if !host.PasswordAuth {
		if strings.TrimSpace(host.IdentityFile) != "" {
			p.steps = append(p.steps, nativeAuthStep{kind: "files"})
		}
		if agentconn.DefaultEndpoint() != "" {
			p.steps = append(p.steps, nativeAuthStep{kind: "agent"})
		}
		if strings.TrimSpace(host.IdentityFile) == "" {
			p.steps = append(p.steps, nativeAuthStep{kind: "files"})
		}
	}
	p.steps = append(p.steps, nativeAuthStep{kind: "password"}, nativeAuthStep{kind: "keyboard-interactive"})
	return p, nil
}
func (p *nativeSSHAuthPlan) Close() error {
	p.cancel()
	p.mu.Lock()
	p.closed = true
	connection := p.connection
	p.connection = nil
	p.steps = nil // Release any untried file signers after authentication/abort.
	p.mu.Unlock()
	if connection != nil {
		return connection.Close()
	}
	return nil
}
func (p *nativeSSHAuthPlan) next(state *ssh.ClientAuthContext) (ssh.AuthMethod, error) {
	for {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		step, ok := p.popStep()
		if !ok {
			break
		}
		method := step.kind
		if method == "files" || method == "agent" || method == "key" {
			method = "publickey"
		}
		if !slices.Contains(state.AllowedMethods, method) {
			continue
		}
		switch step.kind {
		case "files":
			keys, err := p.fileKeys()
			if err != nil {
				return nil, err
			}
			// File keys are separate attempts so server capability changes and partial
			// successes are observed between identities, just as in Electron's handler.
			if err := p.prependKeys(keys); err != nil {
				return nil, err
			}
		case "key":
			return ssh.PublicKeys(step.signer), nil
		case "agent":
			signers, err := p.agentKeys()
			if err != nil {
				return nil, err
			}
			if len(signers) > 0 {
				return ssh.PublicKeys(signers...), nil
			}
		case "password":
			password := p.password
			if password == "" {
				if p.options.Prompt == nil {
					continue
				}
				var err error
				password, err = askSSHPassword(p.ctx, p.options.Prompt, p.host)
				if err != nil {
					return nil, err
				}
			}
			return ssh.Password(password), nil
		case "keyboard-interactive":
			if p.options.Prompt != nil {
				return interactiveSSHAuth(p.ctx, p.host, p.options.Prompt, p.password), nil
			}
		}
	}
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}
func (p *nativeSSHAuthPlan) popStep() (nativeAuthStep, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.steps) == 0 {
		return nativeAuthStep{}, false
	}
	step := p.steps[0]
	p.steps[0] = nativeAuthStep{}
	p.steps = p.steps[1:]
	return step, true
}
func (p *nativeSSHAuthPlan) prependKeys(keys []ssh.Signer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return context.Canceled
	}
	pending := make([]nativeAuthStep, 0, len(keys)+len(p.steps))
	for _, key := range keys {
		pending = append(pending, nativeAuthStep{kind: "key", signer: key})
	}
	p.steps = append(pending, p.steps...)
	return nil
}

func (p *nativeSSHAuthPlan) fileKeys() ([]ssh.Signer, error) {
	explicit := strings.TrimSpace(p.host.IdentityFile) != ""
	paths := []string{}
	if explicit {
		identity := strings.TrimSpace(p.host.IdentityFile)
		if !strings.HasPrefix(identity, "aterm-key://") {
			identity = expandSSHPath(identity, p.directory)
		}
		paths = append(paths, identity)
	} else {
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			paths = append(paths, filepath.Join(p.directory, name))
		}
	}
	var keys []ssh.Signer
	for _, path := range paths {
		result, err := loadNativeSSHIdentity(p.ctx, path, explicit, p.options)
		if err != nil {
			return nil, err
		}
		if result != nil {
			keys = append(keys, result)
		}
	}
	return keys, nil
}
func (p *nativeSSHAuthPlan) agentKeys() ([]ssh.Signer, error) {
	discovery, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()
	connection, err := agentconn.Dial(discovery, "")
	if err != nil {
		if p.ctx.Err() != nil {
			return nil, p.ctx.Err()
		}
		return nil, nil // Missing/busy agents may fall back to the next configured source.
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		connection.Close()
		return nil, context.Canceled
	}
	p.connection = connection
	p.mu.Unlock()
	stop := context.AfterFunc(discovery, func() { connection.Close() })
	defer stop()
	deadline, _ := discovery.Deadline()
	_ = connection.SetDeadline(deadline)
	signers, err := agent.NewClient(connection).Signers()
	_ = connection.SetDeadline(time.Time{})
	if p.ctx.Err() != nil || discovery.Err() != nil || err != nil || len(signers) == 0 {
		p.mu.Lock()
		if p.connection == connection {
			p.connection = nil
		}
		p.mu.Unlock()
		connection.Close()
		if p.ctx.Err() != nil {
			return nil, p.ctx.Err()
		}
		return nil, nil
	}
	return signers, nil
}
