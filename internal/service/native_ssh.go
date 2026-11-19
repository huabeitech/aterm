package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const maxProxyJumpDepth = 4

// OpenNativeSSH connects without launching /usr/bin/ssh, which is prohibited
// by the Mac App Store sandbox. Key material is loaded while the persisted
// security-scoped bookmark for ~/.ssh is active.
func (s *AppService) OpenNativeSSH(ctx context.Context, alias string) (*NativeSSHClient, Host, error) {
	return s.openNativeSSH(ctx, alias, s.hostKeyConfirmer)
}

// OpenNativeSSHWithHostKeyConfirmer connects using a confirmer scoped to this
// operation. Terminal sessions use it to keep host-key interaction inside the
// terminal, while non-terminal callers retain the app's native dialog.
func (s *AppService) OpenNativeSSHWithHostKeyConfirmer(ctx context.Context, alias string, confirmer HostKeyConfirmer) (*NativeSSHClient, Host, error) {
	return s.openNativeSSH(ctx, alias, confirmer)
}

func (s *AppService) openNativeSSH(ctx context.Context, alias string, confirmer HostKeyConfirmer, interactions ...NativeSSHAuthOptions) (*NativeSSHClient, Host, error) {
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return nil, Host{}, err
	}
	host, err := s.hostByAlias(strings.TrimSpace(alias))
	if err != nil {
		return nil, Host{}, err
	}
	return s.openNativeSSHHost(ctx, host, confirmer, interactions...)
}

// openNativeSSHHost receives a frozen destination so an approved plugin launch
// cannot be retargeted by a concurrent edit between validation and the handshake.
func (s *AppService) openNativeSSHHost(ctx context.Context, host Host, confirmer HostKeyConfirmer, interactions ...NativeSSHAuthOptions) (*NativeSSHClient, Host, error) {
	var client *NativeSSHClient
	err := s.withSSHDirectory(ctx, func(paths sshPaths) error {
		var dialErr error
		client, dialErr = s.dialNativeSSH(ctx, host, paths.sshDir, confirmer, 0, interactions...)
		return dialErr
	})
	if err != nil {
		return nil, Host{}, err
	}
	if _, err := s.store.MarkHostUsed(host.ID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		client.Close()
		return nil, Host{}, err
	}
	return client, host, nil
}

func (s *AppService) dialNativeSSH(ctx context.Context, host Host, sshDir string, confirmer HostKeyConfirmer, depth int, interactions ...NativeSSHAuthOptions) (*NativeSSHClient, error) {
	if depth > maxProxyJumpDepth {
		return nil, errors.New("SSH ProxyJump chain is too deep")
	}
	route := []Host{host}
	for {
		jumpID, alias := strings.TrimSpace(host.JumpHostID), firstProxyJump(host.ProxyJump)
		if jumpID == "" && alias == "" {
			break
		}
		if depth+len(route) > maxProxyJumpDepth {
			return nil, errors.New("SSH ProxyJump chain is too deep")
		}
		var err error
		if jumpID != "" {
			host, err = s.storeHostByID(jumpID)
		} else {
			host, err = s.hostByAlias(alias)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve ProxyJump %q: %w", alias, err)
		}
		route = append(route, host)
	}
	return s.dialNativeSSHRoute(ctx, route, sshDir, confirmer, interactions...)
}

func (s *AppService) dialNativeSSHRoute(ctx context.Context, route []Host, sshDir string, confirmer HostKeyConfirmer, interactions ...NativeSSHAuthOptions) (*NativeSSHClient, error) {
	host := route[0]
	// Callbacks for host trust and interactive credentials must observe the
	// same deadline as the transport, or NewClientConn can remain blocked in
	// a prompt after the socket has already timed out.
	ctx, cancel := context.WithTimeout(ctx, nativeSSHTimeout(host.ConnectTimeout))
	defer cancel()
	options := NativeSSHAuthOptions{ReadIdentity: s.identityReader, IdentityName: s.identityNameReader, Prompt: s.credentialPrompter}
	if len(interactions) > 0 {
		options = interactions[0]
	}
	if s.hostKeyStore != nil {
		options.HostKeyCallback = s.trustedHostKeyCallback(ctx, confirmer)
	}
	password, err := s.passwordForHost(host)
	if err != nil && !(options.Prompt != nil && errors.Is(err, ErrPasswordNotFound)) {
		return nil, err
	}
	config, authentication, err := nativeSSHConfig(ctx, host, sshDir, password, confirmer, options)
	if err != nil {
		return nil, err
	}
	if authentication != nil {
		defer authentication.Close()
	}
	address := net.JoinHostPort(host.HostName, strconv.Itoa(resolvedSSHPort(host.Port)))
	var connection net.Conn
	var proxyClient *NativeSSHClient
	if len(route) > 1 {
		jumpAlias := route[1].DisplayName()
		proxy, err := s.dialNativeSSHRoute(ctx, route[1:], sshDir, confirmer, options)
		if err != nil {
			return nil, fmt.Errorf("connect ProxyJump %q: %w", jumpAlias, err)
		}
		proxyClient = proxy
		dialCtx, cancel := context.WithTimeout(ctx, nativeSSHTimeout(host.ConnectTimeout))
		connection, err = proxy.RawClient().DialContext(dialCtx, "tcp", address)
		cancel()
		if err != nil {
			proxy.Close()
			return nil, fmt.Errorf("dial %s through %s: %w", address, jumpAlias, err)
		}
	} else {
		dialer := net.Dialer{Timeout: nativeSSHTimeout(host.ConnectTimeout)}
		connection, err = dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("dial %s: %w", address, err)
		}
	}
	client, err := nativeSSHHandshake(ctx, connection, address, config, nativeSSHTimeout(host.ConnectTimeout), authentication)
	if err != nil {
		connection.Close()
		if proxyClient != nil {
			proxyClient.Close()
		}
		return nil, fmt.Errorf("SSH handshake with %s: %w", host.DisplayName(), err)
	}
	forwarding, err := nativeSSHAgentForwarding(ctx, client, host)
	if err != nil {
		client.Close()
		if proxyClient != nil {
			proxyClient.Close()
		}
		return nil, err
	}
	return newNativeSSHClient(client, host, proxyClient, forwarding), nil
}

func (s *AppService) storeHostByID(id string) (Host, error) {
	hosts, err := s.store.ListHosts()
	if err != nil {
		return Host{}, err
	}
	for _, host := range hosts {
		if host.ID == id {
			return host, nil
		}
	}
	return Host{}, ErrHostNotFound
}

func (s *AppService) runNativeSSH(ctx context.Context, alias, command string, input []byte) (output []byte, err error) {
	client, _, err := s.OpenNativeSSH(ctx, alias)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close()
		err = preferNativeSSHCause(client, err)
	}()
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("create SSH command session: %w", err)
	}
	defer session.Close()
	session.Stdin = strings.NewReader(string(input))
	output, err = session.CombinedOutput(command)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

func (s *AppService) hostByAlias(alias string) (Host, error) {
	if alias == "" {
		return Host{}, ErrAliasRequired
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return Host{}, err
	}
	for _, host := range hosts {
		if host.Alias == alias {
			if strings.TrimSpace(host.HostName) == "" {
				return Host{}, fmt.Errorf("host %q has no hostname", alias)
			}
			return host, nil
		}
	}
	return Host{}, ErrHostNotFound
}

func (s *AppService) passwordForHost(host Host) (string, error) {
	if !host.PasswordAuth {
		return "", nil
	}
	var store PasswordStore = s.hostPasswordStore
	if store == nil {
		store = s.passwordStore
	}
	password, err := store.Get(host.ID)
	if errors.Is(err, ErrPasswordNotFound) {
		return "", fmt.Errorf("the saved SSH password is unavailable; enter it again in the host settings: %w", ErrPasswordNotFound)
	}
	return password, err
}

func nativeSSHConfig(ctx context.Context, host Host, sshDir, password string, confirmer HostKeyConfirmer, interactions ...NativeSSHAuthOptions) (*ssh.ClientConfig, io.Closer, error) {
	options := NativeSSHAuthOptions{}
	if len(interactions) > 0 {
		options = interactions[0]
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	username := strings.TrimSpace(host.User)
	if username == "" {
		if current, err := user.Current(); err == nil {
			username = current.Username
		}
	}
	if username == "" {
		return nil, nil, errors.New("SSH username is required")
	}
	host.User = username // Prompts use the same resolved username as the wire config.
	plan, err := newNativeSSHAuthPlan(ctx, host, sshDir, password, options)
	if err != nil {
		if plan != nil {
			plan.Close()
		}
		return nil, nil, err
	}
	callback := options.HostKeyCallback
	if callback == nil {
		callback, err = nativeKnownHostsCallback(ctx, sshDir, confirmer)
	}
	if err != nil {
		if plan != nil {
			plan.Close()
		}
		return nil, nil, err
	}

	return &ssh.ClientConfig{
		User:            username,
		AuthCallback:    plan.next,
		HostKeyCallback: callback,
		Timeout:         nativeSSHTimeout(host.ConnectTimeout),
	}, plan, nil
}

func nativeCompatibleSigner(signer ssh.Signer) (ssh.Signer, error) {
	if signer.PublicKey().Type() != ssh.KeyAlgoRSA {
		return signer, nil
	}
	algorithmSigner, ok := signer.(ssh.AlgorithmSigner)
	if !ok {
		return nil, errors.New("RSA SSH key does not support SHA-2 signatures")
	}
	return ssh.NewSignerWithAlgorithms(algorithmSigner, []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256})
}

func nativeKnownHostsCallback(ctx context.Context, sshDir string, confirmer HostKeyConfirmer) (ssh.HostKeyCallback, error) {
	// 普通构建不需要预先授权目录，因此只在用户主动连接时创建 SSH 目录。
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return nil, fmt.Errorf("create SSH directory: %w", err)
	}
	if err := os.Chmod(sshDir, 0o700); err != nil {
		return nil, fmt.Errorf("set SSH directory permissions: %w", err)
	}
	path := filepath.Join(sshDir, "known_hosts")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return nil, fmt.Errorf("create known_hosts: %w", err)
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("set known_hosts permissions: %w", err)
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := callback(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) != 0 {
			return fmt.Errorf("SSH host key changed for %s; remove the outdated key from known_hosts only after verifying the server identity: %w", hostname, err)
		}
		host, port := splitKnownHostAddress(hostname)
		if confirmer == nil {
			return fmt.Errorf("SSH host key confirmation is required for %s", hostname)
		}
		approved, confirmErr := confirmer(ctx, HostKeyInfo{
			Host:        host,
			Port:        port,
			KeyType:     key.Type(),
			Fingerprint: ssh.FingerprintSHA256(key),
		})
		if confirmErr != nil {
			return fmt.Errorf("confirm SSH host key for %s: %w", hostname, confirmErr)
		}
		if !approved {
			return fmt.Errorf("SSH host key was not trusted for %s", hostname)
		}
		file, openErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		writer := bufio.NewWriter(file)
		if _, writeErr := writer.WriteString(knownhosts.Line([]string{hostname}, key) + "\n"); writeErr != nil {
			return writeErr
		}
		return writer.Flush()
	}, nil
}

func splitKnownHostAddress(address string) (string, string) {
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		return strings.Trim(host, "[]"), port
	}
	return strings.Trim(address, "[]"), "22"
}

func expandSSHPath(value, sshDir string) string {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	if value == "~/.ssh" {
		return sshDir
	}
	if strings.HasPrefix(value, "~/.ssh/") {
		return filepath.Join(sshDir, strings.TrimPrefix(value, "~/.ssh/"))
	}
	return value
}

func firstProxyJump(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, ","); index >= 0 {
		value = value[:index]
	}
	if at := strings.LastIndex(value, "@"); at >= 0 {
		value = value[at+1:]
	}
	if colon := strings.LastIndex(value, ":"); colon >= 0 {
		value = value[:colon]
	}
	return strings.TrimSpace(value)
}

func nativeSSHTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func resolvedSSHPort(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}

func (s *AppService) OpenNativeSSHWithInteraction(ctx context.Context, alias string, confirmer HostKeyConfirmer, prompter CredentialPrompter) (*NativeSSHClient, Host, error) {
	return s.openNativeSSH(ctx, alias, confirmer, NativeSSHAuthOptions{ReadIdentity: s.identityReader, IdentityName: s.identityNameReader, Prompt: prompter})
}
func askSSHPassword(ctx context.Context, prompt CredentialPrompter, host Host) (string, error) {
	if prompt == nil {
		return "", errors.New("no SSH password is available")
	}
	answers, err := prompt(ctx, CredentialPrompt{Title: nativeAuthenticationTitle(host), Message: "Enter the SSH password for " + host.User + "@" + host.HostName + ". It will be used only for this connection.", Fields: []CredentialField{{Label: "SSH Password", Secret: true}}})
	if err != nil {
		return "", err
	}
	if len(answers) != 1 {
		return "", errors.New("invalid SSH password response")
	}
	return answers[0], nil
}

var nativePasswordChallenge = regexp.MustCompile(`^(?:[a-z0-9_.@-]+(?:’s|'s)? )?password:$`)

// The reference uses JavaScript's non-Unicode /i and \s semantics. RE2's
// Unicode case folding and ASCII-only whitespace would change autofill for
// Kelvin/long-s lookalikes, NBSP and BOM. Fold ASCII and trim the JS set first.
func matchesNativePasswordChallenge(question string) bool {
	const whitespace = "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"
	question = strings.TrimRight(question, whitespace)
	question = strings.Map(func(value rune) rune {
		if value >= 'A' && value <= 'Z' {
			return value + ('a' - 'A')
		}
		return value
	}, question)
	return nativePasswordChallenge.MatchString(question)
}

func nativeAuthenticationTitle(host Host) string {
	return "Authenticate " + host.DisplayName()
}
func interactiveSSHAuth(ctx context.Context, host Host, prompt CredentialPrompter, password string) ssh.AuthMethod {
	return ssh.KeyboardInteractive(func(_ string, instruction string, questions []string, echo []bool) ([]string, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(questions) == 0 {
			return []string{}, nil
		}
		if password != "" && len(questions) == 1 && matchesNativePasswordChallenge(questions[0]) {
			return []string{password}, nil
		}
		fields := make([]CredentialField, len(questions))
		for i, question := range questions {
			fields[i] = CredentialField{Label: question, Secret: i >= len(echo) || !echo[i]}
		}
		if instruction == "" {
			instruction = host.User + "@" + host.HostName
		}
		return prompt(ctx, CredentialPrompt{Title: nativeAuthenticationTitle(host), Message: instruction, Fields: fields})
	})
}

// OpenNativeSSHHostWithInteraction uses a host snapshot already read from the
// saved library by an authorized caller. It does not resolve the alias again.
func (s *AppService) OpenNativeSSHHostWithInteraction(ctx context.Context, host Host, confirmer HostKeyConfirmer, prompter CredentialPrompter) (*NativeSSHClient, Host, error) {
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return nil, Host{}, err
	}
	return s.openNativeSSHHost(ctx, host, confirmer, NativeSSHAuthOptions{ReadIdentity: s.identityReader, IdentityName: s.identityNameReader, Prompt: prompter})
}
