package app

import (
	"io/fs"
	"log/slog"
	"ssh-connect/internal/desktop/product"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func Run(assets fs.FS) error {
	// Create an instance of the app structure
	appService := NewApp()
	productInfo := product.Info()
	// Create application with options
	wailsApp := application.New(application.Options{
		Name:        productInfo.Name,
		Description: productInfo.Description,
		Services: []application.Service{
			application.NewServiceWithOptions(appService, application.ServiceOptions{Name: "App"}),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: appService.desktopAssetMiddleware,
		},
		LogLevel:   slog.LevelInfo,
		ShouldQuit: appService.shouldQuitEditors,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	appService.setWailsApp(wailsApp)
	wailsApp.Event.OnApplicationEvent(events.Mac.ApplicationDidFinishLaunching, func(_ *application.ApplicationEvent) {
		appService.startDesktopAccessibility()
		if preferences, err := appService.desktopPreferences(); err == nil {
			appService.installDesktopMenu(preferences)
		}
	})
	wailsApp.Menu.SetApplicationMenu(applicationMenu())
	if _, err := appService.createMainWindow(); err != nil {
		return err
	}

	return wailsApp.Run()
}

func applicationMenu() *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	return menu
}
