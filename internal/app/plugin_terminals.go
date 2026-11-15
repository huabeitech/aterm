package app

import (
	"context"
	"errors"
	desktopplugins "ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/service"
	"ssh-connect/internal/terminal"
)

// A plugin terminal is an ordinary workbench terminal with a validated initial
// command. Its own transport survives page hiding and follows its source/grant
// lease; it never writes into the user's existing shell.
func (a *App) openPluginTerminal(ctx context.Context, owner uint, source desktopplugins.Session, request desktopplugins.ProcessRequest, policy desktopplugins.ExecutionPolicy) (string, error) {
	current, err := a.resolvePluginSession(owner, source.ID)
	if err != nil || !current.Running || !current.ExecutionSupported || current.Target != source.Target {
		return "", errors.New("terminal disconnected or connection settings changed")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	executable := request.Executable
	if source.Kind == "local" {
		if !service.LocalTerminalAvailable() {
			return "", errors.New("local terminals are unavailable in this edition")
		}
		executable, err = desktopplugins.ResolveExecutable(request.Executable)
		if err != nil {
			return "", err
		}
	}
	command, err := desktopplugins.TerminalCommand(request, policy, executable)
	if err != nil {
		return "", err
	}
	title := request.Title
	if title == "" {
		title = request.Executable
	}
	options := terminal.LaunchOptions{Title: title, InitialCommand: command}
	var session terminal.Session
	if source.Kind == "local" {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		session, err = a.terminal.OpenLocalShellWithOptions(options)
	} else {
		hosts, readErr := a.ListHosts()
		if readErr != nil {
			return "", readErr
		}
		var selected *service.Host
		for _, host := range hosts {
			if host.ID == source.Target.ID {
				target := pluginHostTarget(host)
				if target != source.Target {
					return "", errors.New("connection settings changed; reconnect before opening a plugin terminal")
				}
				copy := host
				selected = &copy
				break
			}
		}
		if selected == nil {
			return "", errors.New("connection settings changed; reconnect before opening a plugin terminal")
		}
		options.HostID = selected.ID
		if err = ctx.Err(); err != nil {
			return "", err
		}
		alias := selected.Alias
		session, err = a.terminal.OpenSSH(alias, options, func(connectCtx context.Context, id string) (*service.NativeSSHClient, service.Host, error) {
			return a.svc.OpenNativeSSHHostWithInteraction(connectCtx, *selected, a.hostKeyConfirmer(id), a.credentialPrompter(id))
		})
	}
	if err != nil {
		return "", err
	}
	a.terminal.Present(session.ID, options.HostID, source.Kind == "local")
	if err = ctx.Err(); err != nil {
		_ = a.CloseTerminal(session.ID)
		return "", err
	}
	return session.ID, nil
}
func (a *App) closePluginTerminal(owner uint, id string) {
	window := a.mainWindow()
	if window == nil || window.ID() != owner {
		return
	}
	snapshot, err := a.desktopTerminalSession(id)
	_ = a.CloseTerminal(id)
	if err == nil {
		snapshot["state"] = "exited"
		snapshot["exitCode"] = nil
		delete(snapshot, "error")
		window.EmitEvent("terminal:event", map[string]any{"type": "state", "session": snapshot})
	}
}
