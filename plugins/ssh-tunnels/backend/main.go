package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	sdk "ssh-connect/packages/plugin-go"
	tunnel "ssh-connect/plugins/ssh-tunnels/backend/forward"
	domain "ssh-connect/plugins/ssh-tunnels/backend/model"
)

type backend struct {
	operations sync.Mutex
	mu         sync.Mutex
	host       sdk.Host
	manager    *tunnel.Manager
	configs    []domain.Tunnel
	loaded     bool
}

func newBackend(h sdk.Host) sdk.Handler {
	b := &backend{host: h}
	b.manager = tunnel.NewManager(func() {})
	return b
}
func main() { sdk.Serve(newBackend) }
func (b *backend) Open(context.Context, string, json.RawMessage) (io.ReadWriteCloser, error) {
	return nil, errors.New("unsupported stream")
}
func (b *backend) load(ctx context.Context) error {
	if b.loaded {
		return nil
	}
	if err := b.host.Call(ctx, "storage.get", map[string]any{"key": "configs"}, &b.configs); err != nil {
		return err
	}
	for _, t := range b.configs {
		if t.ID == "" {
			return errors.New("invalid saved tunnel ID")
		}
		if err := domain.ValidateTunnelInput(t.TunnelInput); err != nil {
			return err
		}
	}
	b.loaded = true
	return nil
}
func (b *backend) save(ctx context.Context, next []domain.Tunnel) error {
	refs := []string{}
	seen := map[string]bool{}
	for _, t := range next {
		if !seen[t.HostID] {
			refs = append(refs, t.HostID)
			seen[t.HostID] = true
		}
	}
	if err := b.host.Call(ctx, "storage.set", map[string]any{"key": "configs", "value": next, "hostIDs": refs}, nil); err != nil {
		return err
	}
	b.configs = next
	return nil
}
func (b *backend) Call(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	if method == "runtime.shutdown" {
		b.manager.Close()
		return sdk.JSON(nil), nil
	}
	if method == "runtime.status" {
		b.mu.Lock()
		defer b.mu.Unlock()
		tasks := 0
		for _, t := range b.configs {
			s := b.manager.Status(t)
			if s.State == "running" || s.State == "starting" || s.State == "stopping" {
				tasks++
			}
		}
		return sdk.JSON(map[string]int{"tasks": tasks, "pid": os.Getpid()}), nil
	}
	var request struct {
		Op    string             `json:"op"`
		ID    string             `json:"id"`
		Input domain.TunnelInput `json:"input"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if method == "tunnels.copy" {
		var p struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		return sdk.JSON(nil), b.host.Call(ctx, "clipboard.write", p, nil)
	}
	if method != "tunnels.request" && method != "tunnels.connect" {
		return nil, errors.New("unknown command")
	}
	if request.Op == "hosts" {
		var hosts json.RawMessage
		err := b.host.Call(ctx, "hosts.list", nil, &hosts)
		return hosts, err
	}
	if request.Op == "copy" {
		return nil, errors.New("use clipboard.write command")
	}
	if request.Op != "list" && request.Op != "test" {
		b.operations.Lock()
		defer b.operations.Unlock()
	}
	b.mu.Lock()
	if err := b.load(ctx); err != nil {
		b.mu.Unlock()
		return nil, err
	}
	configs := append([]domain.Tunnel{}, b.configs...)
	b.mu.Unlock()
	if request.Op == "list" {
		out := []tunnel.Status{}
		for _, t := range configs {
			out = append(out, b.manager.Status(t))
		}
		return sdk.JSON(out), nil
	}
	var config domain.Tunnel
	found := false
	for _, t := range configs {
		if t.ID == request.ID {
			config = t
			found = true
			break
		}
	}
	switch request.Op {
	case "save":
		input := domain.NormalizeTunnelInput(request.Input)
		if err := domain.ValidateTunnelInput(input); err != nil {
			return nil, err
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		next := append([]domain.Tunnel{}, b.configs...)
		if request.ID == "" {
			config = domain.Tunnel{ID: uuid.NewString(), TunnelInput: input, CreatedAt: now, UpdatedAt: now}
			next = append(next, config)
		} else {
			found = false
			for i, t := range next {
				if t.ID == request.ID {
					config = t
					config.TunnelInput = input
					config.UpdatedAt = now
					next[i] = config
					found = true
					break
				}
			}
			if !found {
				return nil, domain.ErrTunnelNotFound
			}
		}
		sort.Slice(next, func(i, j int) bool { return next[i].CreatedAt < next[j].CreatedAt })
		if err := b.save(ctx, next); err != nil {
			return nil, err
		}
		return sdk.JSON(config), nil
	case "test":
		return b.test(ctx, config, request.Input)
	case "start":
		if !found {
			return nil, domain.ErrTunnelNotFound
		}
		return sdk.JSON(nil), b.manager.Start(config, func(ctx context.Context) (tunnel.Transport, error) { return openSSH(ctx, b.host, config.HostID) })
	case "stop":
		b.manager.Stop(request.ID)
		return sdk.JSON(nil), nil
	case "delete":
		b.manager.Forget(request.ID)
		b.mu.Lock()
		defer b.mu.Unlock()
		next := []domain.Tunnel{}
		for _, t := range b.configs {
			if t.ID != request.ID {
				next = append(next, t)
			}
		}
		return sdk.JSON(nil), b.save(ctx, next)
	default:
		return nil, errors.New("unknown tunnel operation")
	}
}
func (b *backend) test(ctx context.Context, saved domain.Tunnel, input domain.TunnelInput) (json.RawMessage, error) {
	input = domain.NormalizeTunnelInput(input)
	if err := domain.ValidateTunnelInput(input); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	status := b.manager.Status(saved)
	existing := status.State == "running" && status.ActiveConfig != nil && status.ActiveConfig.LocalAddress == input.LocalAddress && status.ActiveConfig.LocalPort == input.LocalPort
	result := "checked"
	if existing {
		result = "existing"
	} else {
		l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", tunnel.Address(input.LocalAddress, input.LocalPort))
		if err != nil {
			return nil, fmt.Errorf("local listener: %w", err)
		}
		defer l.Close()
	}
	ssh, err := openSSH(ctx, b.host, input.HostID)
	if err != nil {
		return nil, fmt.Errorf("SSH connection: %w", err)
	}
	defer ssh.Close()
	conn, err := ssh.DialContext(ctx, "tcp", tunnel.Address(input.TargetAddress, input.TargetPort))
	if err != nil {
		return nil, fmt.Errorf("target service: %w", err)
	}
	conn.Close()
	return sdk.JSON(map[string]string{"localListener": result}), nil
}

type transport struct {
	mu     sync.Mutex
	host   sdk.Host
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	stop   func() bool
	once   sync.Once
}

func openSSH(ctx context.Context, h sdk.Host, hostID string) (*transport, error) {
	var reply struct {
		ID string `json:"id"`
	}
	if err := h.Call(ctx, "ssh.open", map[string]string{"hostID": hostID}, &reply); err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(context.Background())
	t := &transport{host: h, id: reply.ID, ctx: life, cancel: cancel}
	t.mu.Lock()
	t.stop = context.AfterFunc(ctx, func() { t.Close() })
	t.mu.Unlock()
	return t, nil
}
func (t *transport) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return t.host.Open(t.ctx, "ssh.tcp", map[string]string{"id": t.id, "network": network, "address": address})
}
func (t *transport) Wait() error {
	return t.host.Call(t.ctx, "ssh.wait", map[string]string{"id": t.id}, nil)
}
func (t *transport) Close() error {
	t.once.Do(func() {
		t.cancel()
		t.mu.Lock()
		stop := t.stop
		t.mu.Unlock()
		if stop != nil {
			stop()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = t.host.Call(ctx, "ssh.close", map[string]string{"id": t.id}, nil)
	})
	return nil
}
