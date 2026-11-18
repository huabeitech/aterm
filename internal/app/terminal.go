package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"ssh-connect/internal/desktop/platform"
	"ssh-connect/internal/service"
	"ssh-connect/internal/terminal"
)

func (a *App) desktopTerminalSession(id string) (map[string]any, error) {
	if a.terminal == nil {
		return nil, errors.New("terminal is not initialized")
	}
	s, err := a.terminal.Snapshot(id)
	if err != nil {
		return nil, err
	}
	state := s.State
	if state == "exited" && s.Kind == "ssh" && s.ExitCode != nil && *s.ExitCode < 0 {
		state = "error"
	}
	if state == "verifying-host-key" {
		state = "connecting"
	}
	title := s.Title

	result := map[string]any{"id": s.ID, "kind": s.Kind, "alias": s.Alias, "title": title, "state": state, "startedAt": s.StartedAt, "exitCode": s.ExitCode, "cols": s.Columns, "rows": s.Rows}
	if state == "error" {
		result["error"] = "SSH connection failed. See the terminal output for details."
	}
	if s.PID > 0 {
		result["pid"] = s.PID
	}
	if s.Cwd != "" {
		result["cwd"] = s.Cwd
	}
	if s.Shell != "" {
		result["shell"] = s.Shell
	}
	if s.Remote != nil {
		result["remote"] = s.Remote
	}
	if s.HostID != "" {
		result["hostID"] = s.HostID
	}
	if s.Kind == "local" && s.LocalNumber > 0 {
		result["localNumber"] = s.LocalNumber
	}
	return result, nil
}

func (a *App) desktopTerminal(channel string, raw []json.RawMessage) (any, error) {
	if a.terminal == nil {
		return nil, errors.New("terminal is not initialized")
	}
	if channel == "terminal:ack" {
		id, err := argument[string](raw, 0)
		if err != nil {
			return nil, err
		}
		seq, err := argument[uint64](raw, 1)
		if err != nil {
			return nil, err
		}
		return nil, a.terminal.AcknowledgeOutput(id, seq)
	}
	if channel == "terminal:input" || channel == "terminal:binary-input" {
		id, err := argument[string](raw, 0)
		if err != nil {
			return nil, err
		}
		if channel == "terminal:binary-input" {
			data, err := argument[[]int](raw, 1)
			if err != nil {
				return nil, err
			}
			if len(data) > 65536 {
				return nil, errors.New("terminal input is too large")
			}
			bytes := make([]byte, len(data))
			for i, v := range data {
				if v < 0 || v > 255 {
					return nil, errors.New("invalid terminal byte")
				}
				bytes[i] = byte(v)
			}
			return nil, a.terminal.WriteBinary(id, bytes)
		}
		text, err := argument[string](raw, 1)
		if err != nil {
			return nil, err
		}
		return nil, a.WriteTerminal(id, base64.StdEncoding.EncodeToString([]byte(text)))
	}
	if channel == "terminal:resize" {
		return a.desktopTerminalResize(raw)
	}
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	switch op {
	case "accessibilityEnabled":
		return platform.AccessibilityEnabled(), nil
	case "list":
		ids := a.terminal.IDs()
		result := []any{}
		for _, id := range ids {
			session, err := a.desktopTerminalSession(id)
			if err == nil {
				result = append(result, session)
			}
		}
		return result, nil
	case "requiresUnlock":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		svc, err := a.service()
		if err != nil {
			return nil, err
		}
		return svc.ConnectionRequiresUnlock(a.ctx, id)
	case "open":
		request, err := argument[struct {
			Kind   string `json:"kind"`
			HostID string `json:"hostID"`
		}](args, 0)
		if err != nil {
			return nil, err
		}
		var session terminal.Session
		if request.Kind == "local" {
			session, err = a.OpenLocalTerminal()
		} else if request.Kind == "ssh" {
			hosts, e := a.ListHosts()
			if e != nil {
				return nil, e
			}
			alias := ""
			for _, host := range hosts {
				if host.ID == request.HostID {
					alias = host.Alias
					break
				}
			}
			if alias == "" {
				return nil, errors.New("host not found")
			}
			svc, e := a.service()
			if e != nil {
				return nil, e
			}
			session, err = a.terminal.OpenSSH(alias, terminal.LaunchOptions{}, func(ctx context.Context, id string) (*service.NativeSSHClient, service.Host, error) {
				return svc.OpenNativeSSHWithInteraction(ctx, alias, a.hostKeyConfirmer(id), a.credentialPrompter(id))
			})
		} else {
			return nil, errors.New("invalid terminal kind")
		}
		if err != nil {
			return nil, err
		}
		a.terminal.Present(session.ID, request.HostID, request.Kind == "local")
		return a.desktopTerminalSession(session.ID)
	case "attach":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		if _, err = a.desktopTerminalResize(args); err != nil {
			return nil, err
		}
		output, err := a.terminal.AttachOutput(id)
		if err != nil {
			return nil, err
		}
		session, err := a.desktopTerminalSession(id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"session": session, "data": output.Data, "seq": output.Offset, "encoding": "base64"}, nil
	case "close":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		a.promptBroker().CancelSession(id)
		return nil, a.CloseTerminal(id)
	case "readClipboard":
		if a.wailsApp == nil {
			return nil, errors.New("clipboard is not initialized")
		}
		text, ok := a.wailsApp.Clipboard.Text()
		if !ok {
			return "", nil
		}
		return text, nil
	case "openLink":
		return a.desktopExternal(args)
	case "answer":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		answers, err := argument[[]string](args, 1)
		if err != nil {
			return nil, err
		}
		return nil, a.promptBroker().Answer(id, answers)
	case "cancelTransfer":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		return nil, a.terminal.CancelTransfer(id)
	default:
		return nil, fmt.Errorf("unknown terminal operation %q", op)
	}
}

func (a *App) desktopTerminalResize(args []json.RawMessage) (any, error) {
	id, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	cols, err := argument[int](args, 1)
	if err != nil {
		return nil, err
	}
	rows, err := argument[int](args, 2)
	if err != nil {
		return nil, err
	}
	if cols < 2 || cols > 500 || rows < 1 || rows > 300 {
		return nil, errors.New("invalid terminal size")
	}
	if err := a.ResizeTerminal(id, uint16(cols), uint16(rows)); err != nil {
		return nil, err
	}
	a.terminal.SetSize(id, cols, rows)
	return nil, nil
}
