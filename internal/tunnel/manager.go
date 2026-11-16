// Package tunnel owns local TCP forwarding independently of terminal sessions.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"ssh-connect/internal/domain"
)

type Transport interface {
	DialContext(context.Context, string, string) (net.Conn, error)
	Wait() error
	Close() error
}
type Connector func(context.Context) (Transport, error)

type Status struct {
	domain.Tunnel
	State        string         `json:"state"`
	Error        string         `json:"error"`
	LastError    string         `json:"lastError"`
	ActiveConfig *domain.Tunnel `json:"activeConfig"`
}

type run struct {
	config                    domain.Tunnel
	state, failure, lastError string
	ctx                       context.Context
	cancel                    context.CancelFunc
	listener                  net.Listener
	client                    Transport
	connections               map[net.Conn]struct{}
	wg                        sync.WaitGroup
}

type Manager struct {
	opMu    sync.Mutex
	mu      sync.Mutex
	runs    map[string]*run
	closed  bool
	changed func()
}

func NewManager(changed func()) *Manager {
	if changed == nil {
		changed = func() {}
	}
	return &Manager{runs: make(map[string]*run), changed: changed}
}

func (m *Manager) Status(config domain.Tunnel) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{Tunnel: config, State: "stopped"}
	if r := m.runs[config.ID]; r != nil {
		status.State, status.Error, status.LastError = r.state, r.failure, r.lastError
		if r.state == "starting" || r.state == "running" || r.state == "stopping" {
			active := r.config
			status.ActiveConfig = &active
		}
	}
	return status
}

// Start returns immediately so authentication prompts and Stop remain usable.
func (m *Manager) Start(config domain.Tunnel, connect Connector) error {
	if err := domain.ValidateTunnelInput(config.TunnelInput); err != nil {
		return err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("tunnel manager is closed")
	}
	previous := m.runs[config.ID]
	if previous != nil && (previous.state == "starting" || previous.state == "running") {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	if previous != nil {
		m.stop(previous)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &run{config: config, state: "starting", ctx: ctx, cancel: cancel, connections: make(map[net.Conn]struct{})}
	r.wg.Add(1)
	m.mu.Lock()
	m.runs[config.ID] = r
	m.mu.Unlock()
	m.changed()
	go m.serve(r, connect)
	return nil
}

func (m *Manager) serve(r *run, connect Connector) {
	defer r.wg.Done()
	// Reserve the exact port before asking for credentials. Never choose another.
	listener, err := (&net.ListenConfig{}).Listen(r.ctx, "tcp", Address(r.config.LocalAddress, r.config.LocalPort))
	if err != nil {
		m.fail(r, fmt.Errorf("listen on %s: %w", Address(r.config.LocalAddress, r.config.LocalPort), err))
		return
	}
	m.mu.Lock()
	if r.ctx.Err() != nil {
		m.mu.Unlock()
		listener.Close()
		return
	}
	r.listener = listener
	m.mu.Unlock()
	client, err := connect(r.ctx)
	if err != nil {
		m.fail(r, fmt.Errorf("connect SSH host: %w", err))
		return
	}
	m.mu.Lock()
	if r.ctx.Err() != nil {
		m.mu.Unlock()
		client.Close()
		return
	}
	r.client, r.state = client, "running"
	r.wg.Add(1)
	m.mu.Unlock()
	m.changed()
	go func() {
		defer r.wg.Done()
		err := client.Wait()
		if err == nil {
			err = errors.New("SSH connection closed")
		}
		m.fail(r, err)
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if r.ctx.Err() == nil {
				m.fail(r, fmt.Errorf("accept local connection: %w", err))
			}
			return
		}
		m.mu.Lock()
		if r.ctx.Err() != nil {
			m.mu.Unlock()
			connection.Close()
			return
		}
		r.connections[connection] = struct{}{}
		r.wg.Add(1)
		m.mu.Unlock()
		go m.forward(r, connection)
	}
}

func (m *Manager) forward(r *run, local net.Conn) {
	defer r.wg.Done()
	defer func() { local.Close(); m.mu.Lock(); delete(r.connections, local); m.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	remote, err := r.client.DialContext(ctx, "tcp", Address(r.config.TargetAddress, r.config.TargetPort))
	cancel()
	if err != nil {
		m.mu.Lock()
		if r.ctx.Err() == nil {
			r.lastError = fmt.Sprintf("connect target %s: %v", Address(r.config.TargetAddress, r.config.TargetPort), err)
		}
		m.mu.Unlock()
		m.changed()
		return
	}
	m.mu.Lock()
	if r.ctx.Err() != nil {
		m.mu.Unlock()
		remote.Close()
		return
	}
	r.connections[remote] = struct{}{}
	hadError := r.lastError != ""
	r.lastError = ""
	m.mu.Unlock()
	if hadError {
		m.changed()
	}
	defer func() { remote.Close(); m.mu.Lock(); delete(r.connections, remote); m.mu.Unlock() }()
	done := make(chan struct{})
	go func() { copyStream(remote, local); close(done) }()
	copyStream(local, remote)
	<-done
}

func copyStream(destination, source net.Conn) {
	_, err := io.Copy(destination, source)
	if err != nil {
		destination.Close()
		source.Close()
		return
	}
	if half, ok := destination.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
	} else {
		destination.Close()
	}
}

func (m *Manager) fail(r *run, err error) {
	m.mu.Lock()
	if r.ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	r.state, r.failure = "failed", err.Error()
	r.cancel()
	m.mu.Unlock()
	m.closeResources(r)
	m.changed()
}

func (m *Manager) closeResources(r *run) {
	m.mu.Lock()
	listener, client := r.listener, r.client
	connections := make([]net.Conn, 0, len(r.connections))
	for conn := range r.connections {
		connections = append(connections, conn)
	}
	m.mu.Unlock()
	if listener != nil {
		listener.Close()
	}
	for _, conn := range connections {
		conn.Close()
	}
	if client != nil {
		client.Close()
	}
}

func (m *Manager) stop(r *run) {
	m.mu.Lock()
	r.state = "stopping"
	r.cancel()
	m.mu.Unlock()
	m.changed()
	m.closeResources(r)
	r.wg.Wait()
	m.mu.Lock()
	r.state, r.failure, r.lastError = "stopped", "", ""
	m.mu.Unlock()
	m.changed()
}

func (m *Manager) Stop(id string) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r != nil {
		m.stop(r)
	}
}

func (m *Manager) Forget(id string) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r != nil {
		m.stop(r)
	}
	m.mu.Lock()
	delete(m.runs, id)
	m.mu.Unlock()
	m.changed()
}

func (m *Manager) StopAll() { m.stopAll(false) }
func (m *Manager) Close()   { m.stopAll(true) }
func (m *Manager) stopAll(closeManager bool) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	m.closed = m.closed || closeManager
	runs := make([]*run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	for _, r := range runs {
		m.stop(r)
	}
	m.mu.Lock()
	m.runs = make(map[string]*run)
	m.mu.Unlock()
	m.changed()
}

func Address(host string, port int) string { return net.JoinHostPort(host, strconv.Itoa(port)) }
