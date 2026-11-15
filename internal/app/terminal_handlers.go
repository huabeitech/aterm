package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"ssh-connect/internal/terminal"
	"strings"

	"ssh-connect/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) OpenTerminal(alias string) (terminal.Session, error) {
	svc, err := a.service()
	if err != nil {
		return terminal.Session{}, err
	}
	if a.terminal == nil {
		return terminal.Session{}, errors.New("terminal is not initialized")
	}
	return a.terminal.OpenSSHAsync(alias, func(ctx context.Context, confirmer service.HostKeyConfirmer) (*service.NativeSSHClient, string, error) {
		client, host, err := svc.OpenNativeSSHWithHostKeyConfirmer(ctx, alias, confirmer)
		if err != nil {
			return nil, "", err
		}
		return client, host.Alias, nil
	})
}

func (a *App) OpenLocalTerminal() (terminal.Session, error) {
	if !service.LocalTerminalAvailable() {
		return terminal.Session{}, errors.New("local shell is unavailable in the Mac App Store build")
	}
	if a.terminal == nil {
		return terminal.Session{}, errors.New("terminal is not initialized")
	}
	return a.terminal.OpenLocalShell()
}

// ChooseTerminalUploadFiles opens Finder after a remote rz command starts a
// ZMODEM receive session. The frontend passes the selected bytes to ZMODEM.
func (a *App) ChooseTerminalUploadFiles() ([]terminalUploadFile, error) {
	paths, err := a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Choose Files to Upload"})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	files := make([]terminalUploadFile, 0, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read selected file %q: %w", path, err)
		}
		files = append(files, terminalUploadFile{Name: filepath.Base(path), Contents: base64.StdEncoding.EncodeToString(contents)})
	}
	return files, nil
}

// SaveTerminalDownload saves a file received from a remote sz command using a
// native Save dialog rather than the webview download location.
func (a *App) SaveTerminalDownload(name string, contents string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(contents)
	if err != nil {
		return "", fmt.Errorf("decode terminal download: %w", err)
	}
	localPath, err := a.saveFileDialog(application.SaveFileDialogOptions{
		Title:                "Save Downloaded File",
		Filename:             filepath.Base(name),
		CanCreateDirectories: true,
	})
	if err != nil || strings.TrimSpace(localPath) == "" {
		if err != nil {
			return "", err
		}
		return "", errors.New("download was cancelled")
	}
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return "", fmt.Errorf("save terminal download: %w", err)
	}
	return localPath, nil
}

func (a *App) WriteTerminal(id string, data string) error {
	if a.terminal == nil {
		return errors.New("terminal is not initialized")
	}
	return a.terminal.Write(id, data)
}

func (a *App) ResizeTerminal(id string, columns uint16, rows uint16) error {
	if a.terminal == nil {
		return errors.New("terminal is not initialized")
	}
	return a.terminal.Resize(id, columns, rows)
}

func (a *App) CloseTerminal(id string) error {
	if a.terminal == nil {
		return errors.New("terminal is not initialized")
	}
	window := a.mainWindow()
	err := a.terminal.Close(id)
	if a.desktopPluginExecutor != nil && window != nil {
		a.desktopPluginExecutor.ForgetTerminal(window.ID(), id)
	}
	if a.desktopPluginManager != nil && window != nil {
		a.desktopPluginManager.SessionChanged(window.ID(), id, true)
	}
	return err
}

func (a *App) TerminalScrollback(id string) (string, error) {
	if a.terminal == nil {
		return "", errors.New("terminal is not initialized")
	}
	return a.terminal.Scrollback(id)
}

// TerminalOutputSnapshot captures scrollback and its byte watermark under one lock.
func (a *App) TerminalOutputSnapshot(id string) (terminal.OutputSnapshot, error) {
	if a.terminal == nil {
		return terminal.OutputSnapshot{}, errors.New("terminal is not initialized")
	}
	return a.terminal.OutputSnapshot(id)
}
