package app

import (
	"encoding/json"
	"errors"

	desktopfiles "ssh-connect/internal/desktop/files"
)

type terminalFileTarget struct {
	Kind       string `json:"kind"`
	TerminalID string `json:"terminalID"`
	ContextID  string `json:"contextID"`
}

func (a *App) openTerminalFiles(raw json.RawMessage) (any, error) {
	var target terminalFileTarget
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, err
	}
	if target.Kind != "terminal" || target.TerminalID == "" || target.ContextID == "" || len(target.TerminalID) > 200 || len(target.ContextID) > 200 {
		return nil, errors.New("invalid file connection target")
	}
	if a.terminal == nil {
		return nil, errors.New("terminal is not initialized")
	}
	client, owner, title, err := a.terminal.OpenFiles(a.ctx, target.TerminalID, target.ContextID)
	if err != nil {
		return nil, err
	}
	info, err := a.fileManager().Add(client, owner, desktopfiles.SessionInfo{Target: target, Title: title})
	if err == nil {
		a.emitEvent("files:event", map[string]any{"type": "session", "session": info})
	}
	return info, err
}
