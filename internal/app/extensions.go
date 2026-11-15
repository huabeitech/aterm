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
func (a *App) desktopPluginRuntimeRequest(ctx context.Context, channel string, args []json.RawMessage) (any, error) {
	window, err := a.requirePluginOwner(ctx)
	if err != nil {
		return nil, err
	}
	if a.desktopPluginRuntime == nil {
		return nil, errors.New("plugin runtime is not initialized")
	}
	if channel == "plugins:runtime:reset" {
		if a.desktopPluginManager != nil {
			a.desktopPluginManager.Reset()
		}
		a.desktopPluginRuntime.Reset()
		return nil, nil
	}
	if channel == "plugins:runtime:document" {
		if !a.securePluginFrames(window) {
			return nil, errors.New("native plugin frame isolation is unavailable")
		}
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		html, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return a.desktopPluginRuntime.SetDocument(id, html)
	}
	instanceID, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	method, err := argument[string](args, 1)
	if err != nil {
		return nil, err
	}
	params, err := argument[json.RawMessage](args, 2)
	if err != nil {
		return nil, err
	}
	return a.desktopPluginRuntime.Handle(ctx, instanceID, method, params)
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
		if err = a.desktopPluginRuntime.Validate(ctx, plugin); err != nil {
			return nil, err
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
	if a.wailsApp == nil {
		return false
	}
	answer := make(chan bool, 1)
	dialog := a.wailsApp.Dialog.Question().SetTitle(plugin.Manifest.Name).SetMessage("This local plugin has not been verified. Its code runs in an isolated sandbox, but execution and interactive terminal permissions can access the target account’s files and network. Only install plugins from sources you trust.")
	cancel := dialog.AddButton("Cancel")
	install := dialog.AddButton("Install")
	cancel.OnClick(func() {
		select {
		case answer <- false:
		default:
		}
	})
	install.OnClick(func() {
		select {
		case answer <- true:
		default:
		}
	})
	dialog.SetDefaultButton(cancel)
	dialog.SetCancelButton(cancel)
	if window := a.mainWindow(); window != nil {
		dialog.AttachToWindow(window)
	}
	dialog.Show()
	select {
	case approved := <-answer:
		return approved
	case <-a.ctx.Done():
		return false
	}
}
