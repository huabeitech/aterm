package app

import (
	"encoding/json"
	"fmt"
	"ssh-connect/internal/service"
)

func (a *App) desktopLibrary(raw []json.RawMessage) (any, error) {
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	// Payload validation stays in the capability/domain service after decoding.
	switch op {
	case "listHosts":
		hosts, err := a.ListHosts()
		if hosts == nil {
			hosts = []service.Host{}
		}
		return hosts, err
	case "listDirectories":
		dirs, err := a.ListDirectories()
		if dirs == nil {
			dirs = []service.Directory{}
		}
		return dirs, err
	case "createHost":
		input, err := argument[service.HostInput](args, 0)
		if err != nil {
			return nil, err
		}
		return a.CreateHost(input)
	case "updateHost":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		input, err := argument[service.HostInput](args, 1)
		if err != nil {
			return nil, err
		}
		return a.UpdateHost(id, input)
	case "deleteHosts":
		ids, err := argument[[]string](args, 0)
		if err != nil {
			return nil, err
		}
		return nil, a.DeleteHosts(ids)
	case "moveHosts":
		ids, err := argument[[]string](args, 0)
		if err != nil {
			return nil, err
		}
		id, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return a.MoveHosts(ids, id)
	case "reorderHosts":
		scope, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		id, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		ids, err := argument[[]string](args, 2)
		if err != nil {
			return nil, err
		}
		return a.ReorderHosts(scope, id, ids)
	case "createDirectory":
		input, err := argument[service.DirectoryInput](args, 0)
		if err != nil {
			return nil, err
		}
		return a.CreateDirectory(input)
	case "updateDirectory":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		input, err := argument[service.DirectoryInput](args, 1)
		if err != nil {
			return nil, err
		}
		return a.UpdateDirectory(id, input)
	case "moveDirectory":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		input, err := argument[service.DirectoryMoveInput](args, 1)
		if err != nil {
			return nil, err
		}
		return a.MoveDirectory(id, input)
	case "deleteDirectory":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		return nil, a.DeleteDirectory(id)
	case "getHostPassword":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		return a.GetHostPassword(id)
	case "previewSnippet":
		content, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		return a.PreviewHostSnippetImport(content)
	case "importSnippet":
		input, err := argument[service.HostSnippetImportInput](args, 0)
		if err != nil {
			return nil, err
		}
		return a.ImportHostSnippet(input)
	case "vaultStatus":
		status, err := a.GetEncryptedSyncStatus()
		return map[string]bool{"localVaultAvailable": status.LocalVaultAvailable, "unlocked": status.Unlocked}, err
	case "unlockVault":
		password, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		setup, err := argument[bool](args, 1)
		if err != nil {
			return nil, err
		}
		if setup {
			_, err = a.SetUpSecureData(password)
		} else {
			_, err = a.UnlockSecureData(password)
		}
		return nil, err
	case "copyText":
		return a.desktopCopyText(args)
	case "listManagedKeys", "createManagedKey", "chooseManagedKeyImport", "importManagedKey", "exportManagedKey", "addManagedKeyToAgent", "deleteManagedKey":
		return a.desktopManagedKeys(op, args)
	default:
		return nil, fmt.Errorf("unknown library operation %q", op)
	}
}
