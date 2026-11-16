package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	desktopplugins "ssh-connect/internal/desktop/plugins"
)

func (a *App) requirePluginOwner(ctx context.Context) (application.Window, error) {
	window, _ := ctx.Value(application.WindowKey).(application.Window)
	current := a.mainWindow()
	if current == nil || window == nil || window.ID() != current.ID() {
		return nil, errors.New("plugin operations require the owning application window")
	}
	return window, nil
}
func (a *App) desktopPlugins(ctx context.Context, raw []json.RawMessage) (any, error) {
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	registry := a.desktopPluginRegistry
	if registry == nil {
		return nil, errors.New("plugin registry is not initialized")
	}
	if op == "list" {
		return registry.List()
	}
	window, err := a.requirePluginOwner(ctx)
	if err != nil {
		return nil, err
	}
	if op == "install" {
		if err = registry.InstallationAllowed(); err != nil {
			return nil, err
		}
		paths, err := a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Install from Package", AllowsOtherFileTypes: true, Message: "Choose an aTerm .aterm-plugin package"})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		if len(paths) != 1 {
			return nil, errors.New("choose one plugin package")
		}
		plugin, err := desktopplugins.InspectArchive(paths[0])
		if err != nil {
			return nil, err
		}
		if previous, _, _, err := registry.Package(plugin.Manifest.ID); err == nil && previous.Manifest.Version == plugin.Manifest.Version {
			return nil, errors.New("this plugin version is already installed")
		}
		epoch := registry.Revision()
		if !a.confirmPluginInstall(plugin) {
			return nil, nil
		}
		if err = registry.Install(plugin, epoch); err != nil {
			return nil, err
		}
		if a.desktopPluginManager != nil {
			a.desktopPluginManager.Changed(plugin.Manifest.ID)
		}
		a.emitEvent("plugins:changed", nil)
		return nil, nil
	}
	id, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}

	owner := window.ID()
	switch op {
	case "visibility":
		sessionID, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		consumer, e := argument[string](args, 2)
		if e != nil {
			return nil, e
		}
		visible, e := argument[bool](args, 3)
		if e != nil {
			return nil, e
		}
		passive := false
		if len(args) > 4 {
			passive, e = argument[bool](args, 4)
			if e != nil {
				return nil, e
			}
		}
		return nil, a.desktopPluginManager.Visibility(owner, id, sessionID, consumer, visible, passive)
	case "authorize", "refresh":
		sessionID, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		if op == "authorize" {
			return nil, a.desktopPluginManager.Authorize(owner, id, sessionID)
		}
		return nil, a.desktopPluginManager.Refresh(owner, id, sessionID)
	case "executeCommand":
		sessionID, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		command, e := argument[string](args, 2)
		if e != nil {
			return nil, e
		}
		var params json.RawMessage
		if len(args) > 3 {
			params = args[3]
		}
		return a.desktopPluginManager.ExecuteCommand(ctx, owner, id, sessionID, command, params)
	case "openView":
		if !a.securePluginFrames(window) {
			return nil, errors.New("native plugin frame isolation is unavailable")
		}
		sessionID, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		viewID, e := argument[string](args, 2)
		if e != nil {
			return nil, e
		}
		return a.desktopPluginManager.OpenView(owner, id, sessionID, viewID)
	case "closeView":
		return nil, a.desktopPluginManager.CloseView(owner, id)
	case "activity":
		if _, _, _, err = registry.Package(id); err != nil {
			return nil, err
		}
		if a.desktopPluginExecutor == nil {
			return nil, errors.New("plugin execution is not initialized")
		}
		return a.desktopPluginExecutor.Activity(id), nil
	case "configure":
		enabled, e := argument[bool](args, 1)
		if e != nil {
			return nil, e
		}
		interval, e := argument[int](args, 2)
		if e != nil {
			return nil, e
		}
		err = registry.Configure(id, enabled, interval)
	case "revoke":
		grantID, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		err = registry.Revoke(id, grantID)
		if err == nil && a.desktopPluginManager != nil {
			a.desktopPluginManager.Revoked(id, grantID)
		}
	case "rollback":
		err = registry.Rollback(id)
	case "uninstall":
		err = registry.Uninstall(id)
	default:
		return nil, fmt.Errorf("plugin operation %q is not implemented", op)
	}
	if err == nil {
		if a.desktopPluginManager != nil && op != "revoke" {
			a.desktopPluginManager.Changed(id)
		}
		a.emitEvent("plugins:changed", nil)
	}
	return nil, err
}
func (a *App) confirmPluginInstall(plugin desktopplugins.Package) bool {
	return a.confirmAction(a.ctx, windowConfirmation{
		Title:        plugin.Manifest.Name,
		Message:      "This local plugin has not been verified. Its code runs in an isolated sandbox, but execution and interactive terminal permissions can access the target account’s files and network. Only install plugins from sources you trust.",
		ConfirmLabel: "Install",
	})
}
