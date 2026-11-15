package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"ssh-connect/internal/desktop"
	"ssh-connect/internal/service"
	"strings"
)

func menuShortcut(value string) string {
	parts := strings.Split(value, "+")
	for i, part := range parts {
		switch part {
		case "meta":
			parts[i] = "Cmd"
		case "ctrl":
			parts[i] = "Ctrl"
		case "alt":
			parts[i] = "Option"
		case "shift":
			parts[i] = "Shift"
		case "arrowleft":
			parts[i] = "Left"
		case "arrowright":
			parts[i] = "Right"
		}
	}
	return strings.Join(parts, "+")
}
func (a *App) installDesktopMenu(preferences desktop.Preferences) {
	if a.wailsApp == nil {
		return
	}
	application.InvokeAsync(func() { a.applyDesktopMenu(preferences) })
}
func (a *App) applyDesktopMenu(preferences desktop.Preferences) {
	if a.wailsApp == nil {
		return
	}
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	add := func(target *application.Menu, id, label, key string) {
		item := target.Add(label)
		if key != "" {
			item.SetAccelerator(menuShortcut(key))
		}
		item.OnClick(func(_ *application.Context) { a.dispatchWorkspaceCommand(id) })
	}
	shortcut := func(key string) string { value, _ := preferences[key].(string); return value }
	appMenu := menu.FindByRole(application.AppMenu)
	if appMenu != nil && appMenu.GetSubmenu() != nil {
		add(appMenu.GetSubmenu(), "settings", "Settings…", "meta+,")
	}
	file := menu.AddSubmenu("File")
	if service.LocalTerminalAvailable() {
		add(file, "new-terminal", "New Local Shell", shortcut("newTerminalShortcut"))
	}
	add(file, "close-tab", "Close Tab", shortcut("closeTerminalShortcut"))
	file.Add("Close Window").SetAccelerator("Cmd+Shift+w").OnClick(func(_ *application.Context) {
		if window := a.wailsApp.Window.Current(); window != nil {
			window.Close()
		}
	})
	menu.AddRole(application.EditMenu)
	view := menu.AddSubmenu("View")
	for _, command := range []struct{ id, label, field string }{
		{"search", "Search Hosts…", "commandPaletteShortcut"}, {"command-palette", "Command Palette…", "commandSearchShortcut"}, {"toggle-sidebar", "Toggle Sidebar", "toggleSidebarShortcut"}, {"toggle-terminal-files", "Switch Terminal and SFTP", "toggleTerminalFilesShortcut"},
	} {
		add(view, command.id, command.label, shortcut(command.field))
	}
	add(view, "find", "Find…", "meta+f")
	add(view, "hosts", "Connections", "meta+shift+h")
	view.AddSeparator()
	view.Add("Toggle Full Screen").SetAccelerator("Cmd+Ctrl+f").OnClick(func(_ *application.Context) {
		if window := a.mainWindow(); window != nil {
			window.ToggleFullscreen()
		}
	})
	session := menu.AddSubmenu("Session")
	add(session, "split-vertical", "Split Right", shortcut("splitVerticalShortcut"))
	add(session, "split-horizontal", "Split Down", shortcut("splitHorizontalShortcut"))
	for _, command := range []struct{ id, label, field string }{
		{"previous-tab", "Previous Tab", "previousTerminalShortcut"}, {"next-tab", "Next Tab", "nextTerminalShortcut"}, {"move-left", "Move Tab Left", "moveTabLeftShortcut"}, {"move-right", "Move Tab Right", "moveTabRightShortcut"},
	} {
		add(session, command.id, command.label, shortcut(command.field))
	}
	menu.AddRole(application.WindowMenu)
	help := menu.AddSubmenu("Help")
	add(help, "settings", "Settings…", "")
	a.wailsApp.Menu.SetApplicationMenu(menu)
}
