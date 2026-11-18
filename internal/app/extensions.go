package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"slices"
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
		if previous, _, _, err := registry.Describe(plugin.Manifest.ID); err == nil && previous.Manifest.Version == plugin.Manifest.Version {
			return nil, errors.New("this plugin version is already installed")
		}
		epoch := registry.Revision()
		if !a.confirmPluginInstall(plugin) {
			return nil, nil
		}
		if err = a.desktopNativePlugins.Change(plugin.Manifest.ID, func() error { return registry.Install(plugin, epoch) }); err != nil {
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
	current, _, currentGeneration, lookupErr := registry.Describe(id)
	native := lookupErr == nil && current.Manifest.Runtime == "native" && !desktopplugins.SessionScoped(current.Manifest)
	if native {
		switch op {
		case "visibility":
			consumer, e := argument[string](args, 2)
			if e != nil {
				return nil, e
			}
			visible, e := argument[bool](args, 3)
			if e != nil {
				return nil, e
			}
			return nil, a.desktopNativePlugins.Visibility(id, fmt.Sprintf("%d:%s", owner, consumer), visible)
		case "executeCommand":
			command, e := argument[string](args, 2)
			if e != nil {
				return nil, e
			}
			if !slices.ContainsFunc(current.Manifest.Contributes.Commands, func(c desktopplugins.Command) bool { return c.ID == command }) {
				return nil, errors.New("command not declared")
			}
			params := json.RawMessage("null")
			if len(args) > 3 {
				params = args[3]
			}
			value, e := a.desktopNativePlugins.Call(ctx, id, command, params)
			return map[string]any{"data": value}, e
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
			for _, v := range current.Manifest.Contributes.Views {
				if v.ID == viewID && registry.Available(id) {
					document, err := a.desktopPluginViews.Open(owner, id, sessionID, current.Views[v.Entry], current.Localizations)
					if err != nil {
						return nil, err
					}
					_, _, generation, e := registry.Describe(id)
					if e != nil || generation != currentGeneration || !registry.Available(id) {
						_ = a.desktopPluginViews.Close(owner, document.InstanceID)
						return nil, errors.New("plugin changed while opening view")
					}
					return document, nil
				}
			}
			return nil, errors.New("view unavailable")
		case "authorize", "refresh":
			return nil, nil
		}
	}

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
		if _, _, _, err = registry.Describe(id); err != nil {
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
		err = a.desktopNativePlugins.Change(id, func() error { return registry.Configure(id, enabled, interval) })
	case "revoke":
		grantID, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		err = a.desktopNativePlugins.Change(id, func() error { return registry.Revoke(id, grantID) })
		if err == nil && a.desktopPluginManager != nil {
			a.desktopPluginManager.Revoked(id, grantID)
		}
	case "rollback":
		err = a.desktopNativePlugins.Change(id, func() error { return registry.Rollback(id) })
	case "uninstall":
		err = a.desktopNativePlugins.Change(id, func() error { return registry.Uninstall(id) })
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
	if plugin.Manifest.Runtime == "native" {
		return a.confirmAction(a.ctx, windowConfirmation{Title: plugin.Manifest.Name, TitleKey: "nativePlugin.installTitle", MessageKey: "nativePlugin.installMessage", ConfirmKey: "nativePlugin.install", CancelKey: "nativePlugin.cancel", Parameters: map[string]string{"plugin": plugin.Manifest.Name}})
	}
	return a.confirmAction(a.ctx, windowConfirmation{
		Title:        plugin.Manifest.Name,
		Message:      "This local plugin has not been verified. Its code runs in an isolated sandbox, but execution and interactive terminal permissions can access the target account’s files and network. Only install plugins from sources you trust.",
		ConfirmLabel: "Install",
	})
}
