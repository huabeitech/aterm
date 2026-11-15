package app

import (
	"strings"

	"ssh-connect/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) ListRemoteFiles(alias string, remotePath string) ([]service.RemoteFile, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.ListRemoteFiles(a.ctx, alias, remotePath)
}

func (a *App) RemoteHomeDirectory(alias string) (string, error) {
	svc, err := a.service()
	if err != nil {
		return "", err
	}
	return svc.RemoteHomeDirectory(a.ctx, alias)
}

func (a *App) ReadRemoteTextFile(alias string, remotePath string) (service.RemoteTextFile, error) {
	svc, err := a.service()
	if err != nil {
		return service.RemoteTextFile{}, err
	}
	return svc.ReadRemoteTextFile(a.ctx, alias, remotePath)
}

func (a *App) WriteRemoteTextFile(alias string, remotePath string, content string, expectedRevision string, overwrite bool) (service.RemoteTextFile, error) {
	svc, err := a.service()
	if err != nil {
		return service.RemoteTextFile{}, err
	}
	return svc.WriteRemoteTextFile(a.ctx, alias, remotePath, content, expectedRevision, overwrite)
}

func (a *App) ReadRemoteImagePreview(alias string, remotePath string) (service.RemoteImagePreview, error) {
	svc, err := a.service()
	if err != nil {
		return service.RemoteImagePreview{}, err
	}
	return svc.ReadRemoteImagePreview(a.ctx, alias, remotePath)
}

func (a *App) CreateRemoteDirectory(alias string, parentPath string, name string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.CreateRemoteDirectory(a.ctx, alias, parentPath, name)
}

func (a *App) RemotePathExists(alias string, remotePath string) (bool, error) {
	svc, err := a.service()
	if err != nil {
		return false, err
	}
	return svc.RemotePathExists(a.ctx, alias, remotePath)
}

func (a *App) RenameRemoteFile(alias string, remotePath string, newName string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.RenameRemoteFile(a.ctx, alias, remotePath, newName)
}

func (a *App) DeleteRemoteFile(alias string, remotePath string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.DeleteRemoteFile(a.ctx, alias, remotePath)
}

func (a *App) DeleteRemoteFiles(alias string, remotePaths []string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.DeleteRemoteFiles(a.ctx, alias, remotePaths)
}

func (a *App) MoveRemoteFiles(alias string, remotePaths []string, destination string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.MoveRemoteFiles(a.ctx, alias, remotePaths, destination)
}

func (a *App) ChooseRemoteUploadFiles() ([]string, error) {
	return a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Choose Files to Upload"})
}

func (a *App) ChooseRemoteUploadFolder() (string, error) {
	return a.openDirectoryDialog(application.OpenFileDialogOptions{Title: "Choose Folder to Upload"})
}

func (a *App) InspectLocalUploadPath(localPath string) (LocalUploadManifest, error) {
	return a.transferService().inspectLocalUploadPath(localPath)
}

func (a *App) EnsureRemoteUploadDirectory(alias string, remotePath string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.EnsureRemoteDirectory(a.ctx, alias, remotePath)
}

func (a *App) UploadRemotePath(alias string, remotePath string, localPath string) error {
	if _, err := a.service(); err != nil {
		return err
	}
	return a.transferService().upload(alias, remotePath, localPath)
}

func (a *App) DownloadRemoteFile(alias string, remotePath string, suggestedName string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	if _, err := svc.PrepareFileConnection(a.ctx, alias); err != nil {
		return err
	}
	localPath, err := a.saveFileDialog(application.SaveFileDialogOptions{Title: "Save Remote File", Filename: suggestedName, CanCreateDirectories: true})
	if err != nil || strings.TrimSpace(localPath) == "" {
		return err
	}
	return svc.DownloadRemotePath(a.ctx, alias, remotePath, localPath)
}

func (a *App) ChooseRemoteDownloadDirectory() (string, error) {
	return a.openDirectoryDialog(application.OpenFileDialogOptions{Title: "Choose Download Folder", CanCreateDirectories: true})
}

// DownloadRemotePaths transfers selected paths concurrently into one local
// folder. Each completed item emits progress to the desktop UI.
func (a *App) DownloadRemotePaths(alias string, remotePaths []string, localDirectory string) ([]remoteTransferResult, error) {
	if _, err := a.service(); err != nil {
		return nil, err
	}
	return a.transferService().downloadMany(alias, remotePaths, localDirectory)
}

func (a *App) transferService() *remoteTransferService {
	if a.remoteTransfer == nil {
		a.remoteTransfer = newRemoteTransferService(a.ctx, a.svc, nil)
	}
	return a.remoteTransfer
}
