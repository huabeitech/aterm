package app

import (
	"context"
	"encoding/json"
	"fmt"
)

// DesktopRequest is the runtime boundary shared by the migrated React renderer.
// Dispatch is explicit: the webview cannot reflectively invoke arbitrary Go methods.
func (a *App) DesktopRequest(ctx context.Context, channel string, args []json.RawMessage) (any, error) {
	switch channel {
	case "app:bootstrap":
		return a.desktopBootstrap()
	case "app:info":
		return a.desktopInfo(), nil
	case "preferences:save":
		return a.desktopSavePreferences(args)
	case "library:request":
		return a.desktopLibrary(args)
	case "integration:request":
		return a.desktopIntegration(args)
	case "terminal:request", "terminal:input", "terminal:binary-input", "terminal:resize", "terminal:ack":
		return a.desktopTerminal(channel, args)
	case "file-editor:request":
		return a.desktopFileEditor(ctx, args)
	case "files:request":
		return a.desktopFiles(args)
	case "themes:request":
		return a.desktopThemes(args)
	case "plugins:runtime:rpc", "plugins:runtime:reset", "plugins:runtime:document":
		return a.desktopPluginRuntimeRequest(ctx, channel, args)
	case "plugins:manage":
		return a.desktopPlugins(ctx, args)
	case "window:action":
		return a.desktopWindow(ctx, args)
	case "workspace:input-mode":
		return a.desktopInputMode(ctx, args)
	case "app:external":
		return a.desktopExternal(args)
	default:
		return nil, fmt.Errorf("unknown desktop channel %q", channel)
	}
}

func argument[T any](args []json.RawMessage, index int) (T, error) {
	var value T
	if index >= len(args) {
		return value, fmt.Errorf("missing argument %d", index)
	}
	if err := json.Unmarshal(args[index], &value); err != nil {
		return value, fmt.Errorf("invalid argument %d: %w", index, err)
	}
	return value, nil
}
func operation(args []json.RawMessage) (string, []json.RawMessage, error) {
	name, err := argument[string](args, 0)
	if err != nil {
		return "", nil, err
	}
	return name, args[1:], nil
}
