package app

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) openDirectoryDialog(options application.OpenFileDialogOptions) (string, error) {
	if a.wailsApp == nil {
		return "", errors.New("application is not initialized")
	}
	options.CanChooseFiles = false
	options.CanChooseDirectories = true
	options.AllowsOtherFileTypes = true
	if options.ButtonText == "" {
		options.ButtonText = "Select Folder"
	}
	if options.Message == "" {
		options.Message = options.Title
	}
	if options.Window == nil {
		options.Window = a.mainWindow()
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&options).PromptForSingleSelection()
}

func (a *App) openMultipleFilesDialog(options application.OpenFileDialogOptions) ([]string, error) {
	if a.wailsApp == nil {
		return nil, errors.New("application is not initialized")
	}
	options.CanChooseFiles = true
	options.CanChooseDirectories = false
	options.AllowsMultipleSelection = true
	if options.Window == nil {
		options.Window = a.mainWindow()
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&options).PromptForMultipleSelection()
}

func (a *App) saveFileDialog(options application.SaveFileDialogOptions) (string, error) {
	if a.wailsApp == nil {
		return "", errors.New("application is not initialized")
	}
	if options.Window == nil {
		options.Window = a.mainWindow()
	}
	return a.wailsApp.Dialog.SaveFileWithOptions(&options).PromptForSingleSelection()
}
