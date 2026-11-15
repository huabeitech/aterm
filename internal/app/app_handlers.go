package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ssh-connect/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func (a *App) Status() (service.AppStatus, error) {
	svc, err := a.service()
	if err != nil {
		return service.AppStatus{}, err
	}
	return svc.Status(a.ctx)
}

func (a *App) ExportOpenSSHConfig() error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.ExportOpenSSHConfig(a.ctx)
}

func (a *App) OpenSSHExportStatus() (service.OpenSSHExportStatus, error) {
	svc, err := a.service()
	if err != nil {
		return service.OpenSSHExportStatus{}, err
	}
	return svc.OpenSSHExportStatus(a.ctx)
}

func (a *App) AuthorizeSSHDirectory() (service.AppStatus, error) {
	if a.ctx == nil {
		return service.AppStatus{}, errors.New("application context is not initialized")
	}
	svc, err := a.service()
	if err != nil {
		return service.AppStatus{}, err
	}
	home, _ := os.UserHomeDir()
	sshDir := defaultSSHDirectory(home)
	defaultDirectory := existingSSHDirectoryOrHome(home, sshDir)
	selectedPath, err := a.openDirectoryDialog(application.OpenFileDialogOptions{
		Title:                   "Select .ssh Directory",
		Directory:               defaultDirectory,
		ShowHiddenFiles:         true,
		CanCreateDirectories:    true,
		CanChooseDirectories:    true,
		CanChooseFiles:          false,
		AllowsMultipleSelection: false,
	})
	if err != nil {
		return service.AppStatus{}, err
	}
	if strings.TrimSpace(selectedPath) == "" {
		return svc.Status(a.ctx)
	}
	if _, err := svc.AuthorizeSSHDirectory(a.ctx, selectedPath); err != nil {
		return service.AppStatus{}, err
	}
	return svc.Status(a.ctx)
}

func existingSSHDirectoryOrHome(home, sshDir string) string {
	if info, err := os.Stat(sshDir); err == nil && info.IsDir() {
		return sshDir
	}
	return home
}

func (a *App) AuthorizeSyncDirectory() (service.SyncStatus, error) {
	if a.ctx == nil {
		return service.SyncStatus{}, errors.New("application context is not initialized")
	}
	svc, err := a.service()
	if err != nil {
		return service.SyncStatus{}, err
	}
	paths, err := defaultAppPaths()
	if err != nil {
		return service.SyncStatus{}, err
	}
	defaultDirectory := filepath.Dir(paths.syncRootPath)
	if info, err := os.Stat(paths.syncRootPath); err == nil && info.IsDir() {
		defaultDirectory = paths.syncRootPath
	}
	selectedPath, err := a.openDirectoryDialog(application.OpenFileDialogOptions{
		Title:                "Select iCloud Drive",
		Directory:            defaultDirectory,
		CanCreateDirectories: false,
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	})
	if err != nil {
		return service.SyncStatus{}, err
	}
	if strings.TrimSpace(selectedPath) == "" {
		return svc.GetSyncStatus(a.ctx)
	}
	if _, err := svc.AuthorizeSyncDirectory(a.ctx, selectedPath); err != nil {
		return service.SyncStatus{}, err
	}
	return svc.GetSyncStatus(a.ctx)
}

func (a *App) ListHosts() ([]service.Host, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.ListHosts(a.ctx)
}

func (a *App) CreateHost(input service.HostInput) (service.Host, error) {
	svc, err := a.service()
	if err != nil {
		return service.Host{}, err
	}
	return svc.CreateHost(a.ctx, input)
}

func (a *App) UpdateHost(id string, input service.HostInput) (service.Host, error) {
	svc, err := a.service()
	if err != nil {
		return service.Host{}, err
	}
	return svc.UpdateHost(a.ctx, id, input)
}

func (a *App) MoveHosts(ids []string, directoryID string) ([]service.Host, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.MoveHosts(a.ctx, ids, directoryID)
}

func (a *App) ReorderHosts(scopeType string, scopeID string, ids []string) ([]service.Host, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.ReorderHosts(a.ctx, scopeType, scopeID, ids)
}

func (a *App) DeleteHost(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.DeleteHost(a.ctx, id)
}

func (a *App) DeleteHosts(ids []string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.DeleteHosts(a.ctx, ids)
}

func (a *App) GetHostPassword(id string) (string, error) {
	svc, err := a.service()
	if err != nil {
		return "", err
	}
	return svc.GetHostPassword(a.ctx, id)
}

func (a *App) ListDirectories() ([]service.Directory, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.ListDirectories(a.ctx)
}

func (a *App) CreateDirectory(input service.DirectoryInput) (service.Directory, error) {
	svc, err := a.service()
	if err != nil {
		return service.Directory{}, err
	}
	return svc.CreateDirectory(a.ctx, input)
}

func (a *App) UpdateDirectory(id string, input service.DirectoryInput) (service.Directory, error) {
	svc, err := a.service()
	if err != nil {
		return service.Directory{}, err
	}
	return svc.UpdateDirectory(a.ctx, id, input)
}

func (a *App) MoveDirectory(id string, input service.DirectoryMoveInput) (service.Directory, error) {
	svc, err := a.service()
	if err != nil {
		return service.Directory{}, err
	}
	return svc.MoveDirectory(a.ctx, id, input)
}

func (a *App) DeleteDirectory(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.DeleteDirectory(a.ctx, id)
}

func (a *App) GetSettings() (service.AppSettings, error) {
	svc, err := a.service()
	if err != nil {
		return service.AppSettings{}, err
	}
	return svc.GetSettings(a.ctx)
}

func (a *App) UpdateSettings(input service.AppSettingsInput) (service.AppSettings, error) {
	svc, err := a.service()
	if err != nil {
		return service.AppSettings{}, err
	}
	return svc.UpdateSettings(a.ctx, input)
}

func (a *App) GetSyncStatus() (service.SyncStatus, error) {
	svc, err := a.service()
	if err != nil {
		return service.SyncStatus{}, err
	}
	return svc.GetSyncStatus(a.ctx)
}

func (a *App) UploadSyncSnapshot() (service.SyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.SyncResult{}, err
	}
	return svc.UploadSyncSnapshot(a.ctx)
}

func (a *App) DownloadSyncSnapshot() (service.SyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.SyncResult{}, err
	}
	return svc.DownloadSyncSnapshot(a.ctx)
}

func (a *App) GetEncryptedSyncStatus() (service.EncryptedSyncStatus, error) {
	svc, err := a.service()
	if err != nil {
		return service.EncryptedSyncStatus{}, err
	}
	return svc.GetEncryptedSyncStatus(a.ctx)
}

func (a *App) SetUpSecureData(password string) (service.EncryptedSyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.EncryptedSyncResult{}, err
	}
	return svc.SetUpSecureData(a.ctx, password)
}

func (a *App) UnlockSecureData(password string) (service.EncryptedSyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.EncryptedSyncResult{}, err
	}
	return svc.UnlockSecureData(a.ctx, password)
}

func (a *App) TryAutoUnlockSecureData() (service.AutoUnlockResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.AutoUnlockResult{}, err
	}
	return svc.TryAutoUnlockSecureData(a.ctx), nil
}

func (a *App) UploadEncryptedSync() (service.EncryptedSyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.EncryptedSyncResult{}, err
	}
	return svc.UploadEncryptedSync(a.ctx)
}

func (a *App) DownloadEncryptedSync(password string) (service.EncryptedSyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.EncryptedSyncResult{}, err
	}
	return svc.DownloadEncryptedSync(a.ctx, password)
}

func (a *App) PreviewICloudBackup(password string) (service.SyncMetadata, error) {
	svc, err := a.service()
	if err != nil {
		return service.SyncMetadata{}, err
	}
	return svc.PreviewICloudBackup(a.ctx, password)
}

func (a *App) ChangeEncryptionPassword(currentPassword, newPassword string) (service.EncryptedSyncResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.EncryptedSyncResult{}, err
	}
	return svc.ChangeEncryptionPassword(a.ctx, currentPassword, newPassword)
}

func (a *App) DeleteICloudBackup() error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.DeleteICloudBackup(a.ctx)
}

func (a *App) ShowErrorNotification(title string, message string) error {
	if a.ctx == nil {
		return errors.New("application context is not initialized")
	}
	if a.notificationInitErr != nil {
		return fmt.Errorf("notification initialization failed: %w", a.notificationInitErr)
	}
	if a.notifier == nil {
		return errors.New("notifications are not initialized")
	}
	authorized, err := a.notifier.CheckNotificationAuthorization()
	if err != nil {
		return err
	}
	if !authorized {
		authorized, err = a.notifier.RequestNotificationAuthorization()
		if err != nil {
			return err
		}
		if !authorized {
			return errors.New("notification authorization denied")
		}
	}
	return a.notifier.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("ssh-connect-error-%d", time.Now().UnixNano()),
		Title: title,
		Body:  message,
	})
}
