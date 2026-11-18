package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	plugins "ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/domain"
	"ssh-connect/internal/service"
	sdk "ssh-connect/packages/plugin-go"
)

type nativePluginHost struct {
	terminals   []string
	authorizeMu sync.Mutex
	app         *App
	id          string
	manifest    plugins.Manifest
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	clients     map[string]*service.NativeSSHClient
	grants      map[string]bool
}

func (a *App) newNativePluginHost(ctx context.Context, id string, manifest plugins.Manifest) (plugins.NativeHost, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &nativePluginHost{app: a, id: id, manifest: manifest, ctx: ctx, cancel: cancel, clients: map[string]*service.NativeSSHClient{}, grants: map[string]bool{}}, nil
}
func (h *nativePluginHost) Close() error {
	h.cancel()
	h.mu.Lock()
	terminals := h.terminals
	h.terminals = nil
	clients := h.clients
	h.clients = map[string]*service.NativeSSHClient{}
	h.mu.Unlock()
	for _, id := range terminals {
		_ = h.app.CloseTerminal(id)
	}
	for _, c := range clients {
		c.Close()
	}
	return nil
}
func (h *nativePluginHost) Call(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(h.ctx, cancel)
	defer stop()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var p struct {
		Kind    string          `json:"kind"`
		Secret  string          `json:"secret"`
		Key     string          `json:"key"`
		Value   json.RawMessage `json:"value"`
		HostIDs []string        `json:"hostIDs"`
		HostID  string          `json:"hostID"`
		ID      string          `json:"id"`
		Text    string          `json:"text"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	svc, err := h.app.service()
	if err != nil {
		return nil, err
	}
	switch method {
	case "secrets.get", "secrets.set", "secrets.delete":
		value, err := svc.PluginSecret(ctx, h.id, p.Key, strings.TrimPrefix(method, "secrets."), p.Secret)
		return sdk.JSON(value), err
	case "storage.get":
		return svc.PluginData(ctx, h.id, p.Key)
	case "storage.set":
		return sdk.JSON(nil), svc.SavePluginData(ctx, domain.PluginData{PluginID: h.id, Key: p.Key, Value: p.Value, HostIDs: p.HostIDs})
	case "terminal.open":
		if !slices.Contains(h.manifest.Permissions, "terminal.open") {
			return nil, errors.New("permission denied")
		}
		result, err := h.app.desktopTerminal("terminal:request", []json.RawMessage{sdk.JSON("open"), sdk.JSON(map[string]string{"kind": p.Kind, "hostID": p.HostID})})
		if err != nil {
			return nil, err
		}
		data := sdk.JSON(result)
		var session struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(data, &session)
		h.mu.Lock()
		if h.ctx.Err() != nil {
			h.mu.Unlock()
			_ = h.app.CloseTerminal(session.ID)
			return nil, h.ctx.Err()
		}
		h.terminals = append(h.terminals, session.ID)
		h.mu.Unlock()
		h.app.emitEvent("plugins:terminal-opened", session.ID)
		return data, nil
	case "clipboard.write":
		if len(p.Text) > 8192 {
			return nil, errors.New("clipboard size limit")
		}
		_, err := h.app.desktopCopyText([]json.RawMessage{sdk.JSON(p.Text)})
		return sdk.JSON(nil), err
	case "hosts.list":
		if !slices.Contains(h.manifest.Permissions, "hosts.read") {
			return nil, errors.New("permission denied")
		}
		hosts, err := svc.ListHosts(ctx)
		if err != nil {
			return nil, err
		}
		out := []map[string]string{}
		for _, host := range hosts {
			out = append(out, map[string]string{"id": host.ID, "alias": host.Alias, "hostName": host.HostName, "user": host.User})
		}
		return sdk.JSON(out), nil
	case "ssh.open":
		if !slices.Contains(h.manifest.Permissions, "ssh.connect") {
			return nil, errors.New("permission denied")
		}
		h.authorizeMu.Lock()
		defer h.authorizeMu.Unlock()
		// Pin grants to the current connection identity, not only to a mutable host ID.
		hosts, err := svc.ListHosts(ctx)
		if err != nil {
			return nil, err
		}
		var target *service.Host
		for _, host := range hosts {
			if host.ID == p.HostID {
				copy := host
				target = &copy
				break
			}
		}
		if target == nil {
			return nil, service.ErrHostNotFound
		}
		grant := plugins.GrantID(pluginHostTarget(*target), h.manifest)
		h.mu.Lock()
		approved := h.grants[grant]
		h.mu.Unlock()
		generation := uint64(0)
		if registry := h.app.desktopPluginRegistry; registry != nil {
			_, config, current, err := registry.Describe(h.id)
			if err != nil {
				return nil, err
			}
			generation = current
			for _, entry := range config.Grants {
				if entry.ID == grant {
					approved = true
					break
				}
			}
		}
		if !approved {
			if !h.app.confirmAction(ctx, windowConfirmation{Title: h.manifest.Name, Message: target.Alias, TitleKey: "nativePlugin.hostTitle", MessageKey: "nativePlugin.hostMessage", ConfirmKey: "nativePlugin.allow", CancelKey: "nativePlugin.cancel", Parameters: map[string]string{"plugin": h.manifest.Name, "host": target.Alias}}) {
				return nil, errors.New("permission denied")
			}
			if registry := h.app.desktopPluginRegistry; registry != nil {
				if err := registry.Grant(h.id, plugins.Grant{ID: grant, Label: target.Alias + " (SSH)"}, generation); err != nil {
					return nil, err
				}
				h.app.emitEvent("plugins:changed", nil)
			}
			h.mu.Lock()
			h.grants[grant] = true
			h.mu.Unlock()
		}
		connectionCtx, connectionCancel := context.WithCancel(h.ctx)
		stopRequest := context.AfterFunc(ctx, connectionCancel)
		client, err := svc.OpenPluginSSHHost(connectionCtx, *target, h.app.hostKeyConfirmer("plugin:"+h.id), h.app.credentialPrompter("plugin:"+h.id))
		stopRequest()
		if err != nil {
			connectionCancel()
			return nil, err
		}
		id := uuid.NewString()
		h.mu.Lock()
		if h.ctx.Err() != nil {
			h.mu.Unlock()
			client.Close()
			connectionCancel()
			return nil, h.ctx.Err()
		}
		if len(h.clients) >= 64 {
			h.mu.Unlock()
			client.Close()
			connectionCancel()
			return nil, errors.New("SSH connection limit")
		}
		h.clients[id] = client
		h.mu.Unlock()
		go func() { _ = client.Wait(); connectionCancel() }()
		return sdk.JSON(map[string]string{"id": id}), nil
	case "ssh.close", "ssh.wait":
		h.mu.Lock()
		client := h.clients[p.ID]
		if method == "ssh.close" {
			delete(h.clients, p.ID)
		}
		h.mu.Unlock()
		if client == nil {
			return nil, errors.New("connection closed")
		}
		if method == "ssh.close" {
			return sdk.JSON(nil), client.Close()
		}
		done := make(chan error, 1)
		go func() { done <- client.Wait() }()
		select {
		case err := <-done:
			return sdk.JSON(nil), err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	default:
		return nil, errors.New("unsupported host capability")
	}
}
func (h *nativePluginHost) Open(ctx context.Context, method string, raw json.RawMessage) (io.ReadWriteCloser, error) {
	if method != "ssh.tcp" && method != "ssh.exec" {
		return nil, errors.New("unsupported stream")
	}
	var p struct{ ID, Network, Address, Command string }
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if method == "ssh.tcp" && p.Network != "tcp" {
		return nil, errors.New("unsupported network")
	}
	if _, _, err := net.SplitHostPort(p.Address); method == "ssh.tcp" && err != nil {
		return nil, err
	}
	h.mu.Lock()
	client := h.clients[p.ID]
	h.mu.Unlock()
	if client == nil {
		return nil, errors.New("connection closed")
	}
	if method == "ssh.exec" {
		if p.Command == "" || len(p.Command) > 8192 {
			return nil, errors.New("invalid remote command")
		}
		session, err := client.NewSession()
		if err != nil {
			return nil, err
		}
		input, err := session.StdinPipe()
		if err != nil {
			session.Close()
			return nil, err
		}
		reader, writer := io.Pipe()
		session.Stdout = writer
		session.Stderr = writer
		if err = session.Start(p.Command); err != nil {
			session.Close()
			reader.Close()
			writer.Close()
			return nil, err
		}
		go func() { writer.CloseWithError(session.Wait()) }()
		return &pluginSSHCommand{Reader: reader, input: input, output: reader, session: session}, nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return client.DialContext(dialCtx, "tcp", p.Address)
}

type pluginSSHCommand struct {
	io.Reader
	input   io.WriteCloser
	output  *io.PipeReader
	session io.Closer
	once    sync.Once
}

func (c *pluginSSHCommand) Write(p []byte) (int, error) { return c.input.Write(p) }
func (c *pluginSSHCommand) CloseWrite() error           { return c.input.Close() }
func (c *pluginSSHCommand) Close() error {
	c.once.Do(func() { c.input.Close(); c.session.Close(); c.output.Close() })
	return nil
}
