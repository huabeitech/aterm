//go:build darwin && !mas

package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"ssh-connect/internal/agentbridge"
	"ssh-connect/internal/desktop/keys"
	"ssh-connect/internal/service"
	"strconv"
	"strings"
)

var sshAddress = regexp.MustCompile(`^[A-Za-z0-9_:][A-Za-z0-9_.:-]*$`)

func sshPort(port int) int {
	if port == 0 {
		return 22
	}
	return port
}
func quoteProxyArg(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func (a *App) agentCommand(ctx, lease context.Context, route service.AgentRoute) (*agentbridge.Connection, error) {
	result := &agentbridge.Connection{Program: "/usr/bin/ssh", Env: map[string]string{"SSH_ASKPASS_REQUIRE": "never"}}
	hosts := route.Hosts()
	var proxy []string
	for i := len(hosts) - 1; i >= 0; i-- {
		h := hosts[i]
		if !sshAddress.MatchString(h.HostName) || strings.ContainsAny(h.User, "\x00\r\n%") {
			return nil, errors.New("INVALID_SSH_ENDPOINT")
		}
		args := []string{"-F", "/dev/null", "-o", "ForwardAgent=no", "-o", "StrictHostKeyChecking=ask", "-p", strconv.Itoa(sshPort(h.Port))}
		if os.Getenv("ATERM_DATA_DIR") != "" {
			paths, err := defaultAppPaths()
			if err != nil {
				return nil, err
			}
			file := filepath.Join(paths.sshDir, "known_hosts")
			if strings.ContainsAny(file, "\r\n%\"\\") {
				return nil, errors.New("INVALID_TRUST_PATH")
			}
			args = append(args, "-o", "UserKnownHostsFile=\""+file+"\"")
		}
		if h.User != "" {
			args = append(args, "-l", h.User)
		}
		mode := "local_identity"
		switch {
		case h.PasswordAuth:
			mode = "user_password"
			result.RequiresUserTerminal = true
			args = append(args, "-o", "PubkeyAuthentication=no", "-o", "PreferredAuthentications=password,keyboard-interactive", "-o", "BatchMode=no")
		case strings.HasPrefix(h.IdentityFile, keys.Prefix):
			mode = "vault_signer"
			signer, err := a.svc.ManagedAgentSigner(ctx, h.IdentityFile, a.credentialPrompter("agent"))
			if err != nil {
				return nil, err
			}
			socket, err := serveLeaseSigner(lease, signer)
			if err != nil {
				return nil, err
			}
			args = append(args, "-o", "IdentityAgent="+socket, "-o", "IdentityFile=none", "-o", "PreferredAuthentications=publickey")
		case h.IdentityFile != "":
			if strings.ContainsAny(h.IdentityFile, "\r\n%") {
				return nil, errors.New("INVALID_IDENTITY_PATH")
			}
			args = append(args, "-i", h.IdentityFile)
		}
		if len(proxy) > 0 {
			escaped := make([]string, len(proxy))
			for j, arg := range proxy {
				escaped[j] = quoteProxyArg(strings.ReplaceAll(arg, "%", "%%"))
			}
			args = append(args, "-o", "ProxyCommand="+strings.Join(escaped, " "))
		}
		if i == 0 {
			result.Args = append(args, "--", h.HostName)
			result.AuthMode = mode
			break
		}
		target := hosts[i-1]
		proxy = append([]string{result.Program}, args...)
		proxy = append(proxy, "-W", net.JoinHostPort(target.HostName, strconv.Itoa(sshPort(target.Port))), "--", h.HostName)
	}
	return result, nil
}
