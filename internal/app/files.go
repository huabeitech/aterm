package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pkg/sftp"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	desktopfiles "ssh-connect/internal/desktop/files"
	"ssh-connect/internal/desktop/platform"
	"ssh-connect/internal/desktop/terminalmeta"
	"ssh-connect/internal/service"
)

func (a *App) fileManager() *desktopfiles.Manager {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.desktopFileManager == nil {
		a.desktopFileManager = desktopfiles.NewManager()
	}
	return a.desktopFileManager
}
func (a *App) transfers() *desktopfiles.Transfers {
	manager := a.fileManager()
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.desktopTransfers == nil {
		a.desktopTransfers = desktopfiles.NewTransfers(manager, func(event any) { a.emitEvent("files:event", event) })
	}
	return a.desktopTransfers
}
func (a *App) openDesktopFiles(args []json.RawMessage) (any, error) {
	var target any
	if len(args) == 0 {
		return nil, errors.New("missing file target")
	}
	if err := json.Unmarshal(args[0], &target); err != nil {
		return nil, err
	}
	hostID, ok := target.(string)
	if !ok {
		return a.openTerminalFiles(args[0])
	}
	hosts, err := a.ListHosts()
	if err != nil {
		return nil, err
	}
	var host service.Host
	found := false
	for _, candidate := range hosts {
		if candidate.ID == hostID {
			host = candidate
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("host not found")
	}
	var client *service.NativeSSHClient
	var owner io.Closer
	probeDirectory := false
	if len(args) > 1 {
		terminalID, err := argument[string](args, 1)
		if err == nil && a.terminal != nil {
			client = a.terminal.ClientForHost(terminalID, hostID)
			probeDirectory = client != nil
		}
	}
	if client == nil {
		svc, err := a.service()
		if err != nil {
			return nil, err
		}
		client, _, err = svc.OpenNativeSSHHostWithInteraction(a.ctx, host, a.hostKeyConfirmer("files:"+hostID), a.credentialPrompter("files:"+hostID))
		if err != nil {
			return nil, err
		}
		owner = client
	}
	initialPath := ""
	if probeDirectory {
		initialPath = terminalmeta.ReadRemoteDirectory(a.ctx, client)
	}
	files, err := sftp.NewClient(client.RawClient())
	if err != nil {
		if owner != nil {
			owner.Close()
		}
		return nil, err
	}
	info, err := a.fileManager().Add(files, owner, desktopfiles.SessionInfo{HostID: hostID, Target: target, Title: host.DisplayName(), InitialPath: initialPath})
	if err == nil {
		a.emitEvent("files:event", map[string]any{"type": "session", "session": info})
	}
	return info, err
}
func (a *App) desktopFiles(raw []json.RawMessage) (any, error) {
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	switch op {
	case "open":
		return a.openDesktopFiles(args)
	case "clipboardFilePaths":
		return platform.ClipboardFilePaths(), nil
	}
	id, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	manager := a.fileManager()
	switch op {
	case "close":
		a.disconnectEditors(id)
		a.transfers().CloseSession(id)
		return nil, manager.Close(id)
	case "list":
		name, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return manager.List(id, name)
	case "readText":
		name, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return manager.ReadText(id, name)
	case "saveText":
		input, err := argument[desktopfiles.Text](args, 1)
		if err != nil {
			return nil, err
		}
		document, err := manager.SaveText(id, input)
		if err == nil {
			a.emitEvent("files:event", map[string]any{"type": "saved", "sessionID": id, "path": document.Path})
		}
		return document, err
	case "mutate":
		input, err := argument[desktopfiles.Mutation](args, 1)
		if err != nil {
			return nil, err
		}
		return nil, manager.Mutate(id, input)
	case "image":
		name, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return manager.Image(id, name)
	case "upload":
		directory, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		folder := false
		if len(args) > 2 {
			folder, err = argument[bool](args, 2)
			if err != nil {
				return nil, err
			}
		}
		var paths []string
		if folder {
			selected, err := a.ChooseRemoteUploadFolder()
			if err != nil {
				return nil, err
			}
			if selected != "" {
				paths = []string{selected}
			}
		} else {
			paths, err = a.ChooseRemoteUploadFiles()
			if err != nil {
				return nil, err
			}
		}
		return nil, a.queueUploads(id, directory, paths)
	case "drop":
		directory, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		paths, err := argument[[]string](args, 2)
		if err != nil {
			return nil, err
		}
		return nil, a.queueUploads(id, directory, paths)
	case "resolveConflict":
		transferID, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		choice, err := argument[desktopfiles.UploadChoice](args, 2)
		if err != nil {
			return nil, err
		}
		return nil, a.transfers().ResolveConflict(id, transferID, choice)
	case "download":
		paths, err := argument[[]string](args, 1)
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			return nil, nil
		}
		directory, err := a.ChooseRemoteDownloadDirectory()
		if err != nil || directory == "" {
			return nil, err
		}
		for _, remote := range paths {
			name := path.Base(remote)
			if name == "." || name == "/" || name == ".." {
				return nil, errors.New("select a file or folder to download")
			}
			if _, err := a.transfers().Queue(id, "download", filepath.Join(directory, name), remote); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case "pause", "resume", "cancel", "retry", "removeTransfer", "reveal":
		transferID, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		switch op {
		case "pause", "resume":
			return nil, a.transfers().SetPaused(id, transferID, op == "pause")
		case "cancel":
			return nil, a.transfers().Cancel(id, transferID)
		case "retry":
			return nil, a.transfers().Retry(id, transferID)
		case "removeTransfer":
			return nil, a.transfers().Remove(id, transferID)
		case "reveal":
			name, err := a.transfers().CompletedDownload(id, transferID)
			if err != nil {
				return nil, err
			}
			return nil, exec.Command("/usr/bin/open", "-R", name).Run()
		}
	case "clearCompleted":
		if _, err := manager.Session(id); err != nil {
			return nil, err
		}
		a.transfers().ClearCompleted(id)
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown file operation %q", op)
	}
	return nil, errors.New("invalid file operation")
}

func (a *App) queueUploads(id, directory string, paths []string) error {
	batch := desktopfiles.NewUploadBatch(len(paths) > 1)
	for _, local := range paths {
		if _, err := a.transfers().QueueUpload(id, local, path.Join(directory, filepath.Base(local)), batch); err != nil {
			return err
		}
	}
	return nil
}
