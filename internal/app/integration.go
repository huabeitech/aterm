package app

import (
	"encoding/json"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"ssh-connect/internal/service"
)

func (a *App) desktopIntegrationStatus() (any, error) {
	app, err := a.Status()
	if err != nil {
		return nil, err
	}
	sync, err := a.GetEncryptedSyncStatus()
	if err != nil {
		return nil, err
	}
	openssh, err := a.OpenSSHExportStatus()
	if err != nil {
		return nil, err
	}
	return map[string]any{"app": app, "sync": sync, "openssh": openssh}, nil
}
func (a *App) desktopIntegration(raw []json.RawMessage) (any, error) {
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	switch op {
	case "status":
		return a.desktopIntegrationStatus()
	case "chooseDirectory":
		kind, e := argument[string](args, 0)
		if e != nil {
			return nil, e
		}
		if kind == "ssh" {
			_, err = a.AuthorizeSSHDirectory()
		} else if kind == "sync" || kind == "icloud" {
			_, err = a.AuthorizeSyncDirectory()
		} else {
			err = fmt.Errorf("invalid directory kind")
		}
	case "changePassword":
		current, e := argument[string](args, 0)
		if e != nil {
			return nil, e
		}
		next, e := argument[string](args, 1)
		if e != nil {
			return nil, e
		}
		_, err = a.ChangeEncryptionPassword(current, next)
	case "backup":
		file, e := argument[bool](args, 0)
		if e != nil {
			return nil, e
		}
		if file {
			path, e := a.saveFileDialog(application.SaveFileDialogOptions{Title: "Export Encrypted Backup", Filename: "vault.enc", CanCreateDirectories: true})
			if e != nil {
				return nil, e
			}
			if path == "" {
				return a.desktopIntegrationStatus()
			}
			_, err = a.svc.ExportEncryptedBackup(a.ctx, path)

		} else {
			_, err = a.UploadEncryptedSync()
		}
	case "restore":
		password, e := argument[string](args, 0)
		if e != nil {
			return nil, e
		}
		file, e := argument[bool](args, 1)
		if e != nil {
			return nil, e
		}
		if file {
			paths, e := a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Restore Encrypted Backup", Message: "Choose an encrypted backup file"})
			if e != nil {
				return nil, e
			}
			if len(paths) == 0 {
				return a.desktopIntegrationStatus()
			}
			if len(paths) != 1 {
				return nil, fmt.Errorf("choose one encrypted backup file")
			}
			_, err = a.svc.RestoreEncryptedBackup(a.ctx, paths[0], password, a.confirmBackupRestore)
		} else {
			_, err = a.svc.RestoreCloudEncryptedBackup(a.ctx, password, a.confirmBackupRestore)
		}
		if err == nil {
			if preferences, e := a.desktopPreferences(); e == nil {
				a.installDesktopMenu(preferences)
			}
		}
	case "deleteBackup":
		err = a.svc.DeleteICloudBackupWithConfirmation(a.ctx, func(string) bool {
			return a.confirmBackupAction("Delete iCloud Backup", "Local hosts and passwords will be kept.", "Delete")
		})
	case "exportSSH":
		err = a.ExportOpenSSHConfig()
	case "autoUnlock":
		enable, e := argument[bool](args, 0)
		if e != nil {
			return nil, e
		}
		err = a.svc.SetAutoUnlock(a.ctx, enable)
	default:
		return nil, fmt.Errorf("unknown integration operation %q", op)
	}
	if err != nil {
		return nil, err
	}
	return a.desktopIntegrationStatus()
}

func (a *App) confirmBackupRestore(metadata service.SyncMetadata) bool {
	source, date := metadata.SourceDeviceName, metadata.SnapshotCreatedAt
	if source == "" {
		source = "Unknown device"
	}
	if date == "" {
		date = "Unknown date"
	}
	return a.confirmBackupAction("Restore Encrypted Backup", fmt.Sprintf("Replace saved connections and protected data with this backup?\n\nSource: %s\nDate: %s\nHosts: %d\nFolders: %d", source, date, metadata.HostCount, metadata.DirectoryCount), "Restore")
}
func (a *App) confirmBackupAction(title, message, action string) bool {
	if a.wailsApp == nil {
		return false
	}
	answer := make(chan bool, 1)
	dialog := a.wailsApp.Dialog.Question().SetTitle(title).SetMessage(message)
	cancel := dialog.AddButton("Cancel")
	accept := dialog.AddButton(action)
	cancel.OnClick(func() {
		select {
		case answer <- false:
		default:
		}
	})
	accept.OnClick(func() {
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
	case result := <-answer:
		return result
	case <-a.ctx.Done():
		return false
	}
}
