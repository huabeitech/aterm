package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	desktopkeys "ssh-connect/internal/desktop/keys"
	"ssh-connect/internal/service"
)

func (a *App) keyStore() (*desktopkeys.Store, error) {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.desktopKeys == nil {
		return nil, errors.New("managed SSH key storage is not initialized")
	}
	return a.desktopKeys, nil
}
func (a *App) desktopManagedKeys(operation string, args []json.RawMessage) (any, error) {
	store, err := a.keyStore()
	if err != nil {
		return nil, err
	}
	switch operation {
	case "listManagedKeys":
		return store.List(), nil
	case "createManagedKey":
		input, err := argument[desktopkeys.CreateInput](args, 0)
		if err != nil {
			return nil, err
		}
		return store.Create(input)
	case "chooseManagedKeyImport":
		paths, err := a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Import SSH Private Key", Message: "Choose one SSH private key", ShowHiddenFiles: true})
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			return nil, nil
		}
		if len(paths) != 1 {
			return nil, errors.New("choose one private key file")
		}
		file, err := os.Open(paths[0])
		if err != nil {
			return nil, err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > desktopkeys.MaxPrivateBytes {
			return nil, errors.New("choose a private key file smaller than 1 MB")
		}
		bytes, err := io.ReadAll(io.LimitReader(file, desktopkeys.MaxPrivateBytes+1))
		if err != nil {
			return nil, err
		}
		defer clear(bytes)
		return store.Preview(filepath.Base(paths[0]), bytes)
	case "importManagedKey":
		token, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		name, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		passphrase, err := argument[string](args, 2)
		if err != nil {
			return nil, err
		}
		return store.Import(token, name, passphrase)
	case "exportManagedKey":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		key, err := store.Metadata(id)
		if err != nil {
			return nil, err
		}
		safeName := regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(key.Name, "_")
		destination, err := a.saveFileDialog(application.SaveFileDialogOptions{Title: "Export Private Key for OpenSSH", Filename: safeName, CanCreateDirectories: true})
		if err != nil {
			return nil, err
		}
		if destination == "" {
			return false, nil
		}
		return true, store.Export(id, destination)
	case "addManagedKeyToAgent":
		if service.IsMacAppStoreBuild() {
			return nil, errors.New("SSH agent integration is unavailable in the Mac App Store edition")
		}
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		passphrase, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		return nil, store.AddToAgent(ctx, id, passphrase, os.Getenv("SSH_AUTH_SOCK"))
	case "deleteManagedKey":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		hosts, err := a.ListHosts()
		if err != nil {
			return nil, err
		}
		for _, host := range hosts {
			if host.IdentityFile == desktopkeys.Prefix+id {
				return nil, errors.New("this SSH key is still used by saved hosts; choose another key in those connections before deleting it")
			}
		}
		return nil, store.Delete(id)
	default:
		return nil, errors.New("unknown managed SSH key operation")
	}
}
