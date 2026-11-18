package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	desktopplugins "ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/service"
	"strings"
)

func (a *App) resolvePluginSession(owner uint, id string) (desktopplugins.Session, error) {
	window := a.mainWindow()
	if window == nil || window.ID() != owner || a.terminal == nil {
		return desktopplugins.Session{}, errors.New("session not found")
	}
	entry, client, identity, err := a.terminal.ExecutionContext(id)
	if err != nil {
		return desktopplugins.Session{}, err
	}
	snapshot := desktopplugins.Session{ID: entry.ID, Kind: entry.Kind, Label: entry.Alias, Running: entry.State == "running"}
	if snapshot.Label == "" {
		snapshot.Label = entry.Title
	}
	connected := client != nil
	if snapshot.Kind == "local" {
		username := ""
		if current, err := user.Current(); err == nil {
			username = current.Username
		}
		snapshot.Target = desktopplugins.GrantTarget{ID: "local", HostName: "localhost", User: username}
		snapshot.ExecutionSupported = true
	} else if connected && identity != nil {
		snapshot.Target = pluginHostTarget(*identity)
		snapshot.ExecutionSupported = true
	}
	return snapshot, nil
}
func (a *App) runPluginProcess(ctx context.Context, session desktopplugins.Session, request desktopplugins.ProcessRequest, policy desktopplugins.ExecutionPolicy, onData desktopplugins.ProcessData) (desktopplugins.ProcessResult, error) {
	if a.terminal == nil {
		return desktopplugins.ProcessResult{}, errors.New("terminal disconnected")
	}
	entry, client, _, err := a.terminal.ExecutionContext(session.ID)
	if err != nil {
		return desktopplugins.ProcessResult{}, err
	}
	if entry.State != "running" {
		return desktopplugins.ProcessResult{}, errors.New("terminal disconnected")
	}
	if session.Kind == "local" {
		return desktopplugins.RunLocalProcess(ctx, request, policy, onData)
	}
	if client == nil {
		return desktopplugins.ProcessResult{}, errors.New("execution environment unavailable")
	}
	return desktopplugins.RunSSHProcess(ctx, client, request, policy, onData)
}
func (a *App) confirmPluginExecution(ctx context.Context, request desktopplugins.PermissionRequest) bool {
	programs, targets := "", ""
	if policy := request.Plugin.Manifest.Execution; policy != nil {
		programs = strings.Join(policy.Programs, ", ")
		targets = strings.Join(policy.Targets, ", ")
	}
	detail := fmt.Sprintf("%s\n\nDeclared programs: %s\nTargets: %s\n\nThis permission allows the declared programs to access this account’s files and network. Only allow plugins you trust.", request.Grant.Label, programs, targets)
	return a.confirmAction(ctx, windowConfirmation{
		Title:        "Allow " + request.Plugin.Manifest.Name + " to execute programs?",
		Message:      detail,
		ConfirmLabel: "Allow for This Host",
		CancelLabel:  "Not Now",
	})
}

func (a *App) initializeDesktopPlugins(ctx context.Context, root string) error {
	lock, err := desktopplugins.LockProfile(root)
	if err != nil {
		return err
	}
	a.desktopPluginLock = lock
	registry, err := desktopplugins.NewRegistry(desktopplugins.RegistryOptions{
		Root: root, SafeMode: slices.Contains(os.Args, "--disable-plugins"), MacAppStore: service.IsMacAppStoreBuild(),
	})
	if err != nil {
		lock.Close()
		return err
	}
	a.desktopPluginRegistry = registry
	a.desktopNativePlugins = desktopplugins.NewNativeManager(desktopplugins.NativeOptions{LogRoot: filepath.Join(root, "logs"), Registry: registry, Host: a.newNativePluginHost, Changed: func() { a.emitEvent("plugins:changed", nil) }})
	a.desktopPluginViews = desktopplugins.NewViewHost()
	a.desktopPluginRuntime = desktopplugins.NewNativeRuntime(a.requestPluginCapability)
	if preferences, err := a.desktopPreferences(); err == nil {
		locale, _ := preferences["language"].(string)
		a.desktopPluginRuntime.SetLocale(locale)
	}
	a.desktopPluginExecutor = desktopplugins.NewExecutor(desktopplugins.ExecutionOptions{
		Audit: desktopplugins.NewAudit(filepath.Join(root, "execution-audit.json")),
		Run:   a.runPluginProcess,
		StreamEvent: func(id string, event desktopplugins.StreamEvent) error {
			return a.desktopPluginRuntime.StreamEvent(id, event)
		},
		CloseTerminal: a.closePluginTerminal,
		OpenTerminal:  a.openPluginTerminal,
		TerminalOpened: func(owner uint, id string) {
			if window := a.mainWindow(); window != nil && window.ID() == owner {
				window.EmitEvent("plugins:terminal-opened", id)
			}
		},
	})
	a.desktopPluginManager = a.newDesktopPluginManager(ctx, root, a.confirmPluginExecution)
	return nil
}

func (a *App) newDesktopPluginManager(ctx context.Context, root string, authorize func(context.Context, desktopplugins.PermissionRequest) bool) *desktopplugins.Manager {
	return desktopplugins.NewManager(desktopplugins.ManagerOptions{
		Context: ctx, Registry: a.desktopPluginRegistry, Runtime: a.desktopPluginRuntime,
		Storage:        desktopplugins.NewStorage(filepath.Join(root, "data")),
		Views:          a.desktopPluginViews,
		ResolveSession: a.resolvePluginSession, Authorize: authorize,
		SendStatus:   a.sendPluginStatus,
		Changed:      func() { a.emitEvent("plugins:changed", nil) },
		Capabilities: a.desktopPluginExecutor,
	})
}
func (a *App) sendPluginStatus(owner uint, status desktopplugins.Status) {
	if window := a.mainWindow(); window != nil && window.ID() == owner {
		window.EmitEvent("plugins:status", status)
	}
}
func (a *App) requestPluginCapability(ctx context.Context, id, method string, params json.RawMessage) (any, error) {
	if a.desktopPluginManager == nil {
		return nil, errors.New("plugin manager is not initialized")
	}
	return a.desktopPluginManager.Request(ctx, id, method, params)
}

func pluginHostTarget(host service.Host) desktopplugins.GrantTarget {
	target := desktopplugins.GrantTarget{ID: host.ID, HostName: host.HostName, Port: host.Port, User: host.User, JumpHostID: host.JumpHostID, ProxyJump: host.ProxyJump}
	if target.User == "" {
		if current, err := user.Current(); err == nil {
			target.User = current.Username
		}
	}
	return target
}
