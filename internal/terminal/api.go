package terminal

import (
	"context"
	"errors"
	"github.com/pkg/sftp"
	"io"
	"ssh-connect/internal/desktop/zmodem"
	"ssh-connect/internal/service"
)

// TransferOptions connects native file selection without coupling the terminal
// runtime to Wails windows or dialogs. Configure before opening any sessions.
type TransferOptions struct {
	Download func(context.Context, string, zmodem.FileOffer) (zmodem.Download, error)
	Upload   func(context.Context, string) ([]zmodem.Upload, error)
}

func (m *Manager) ConfigureTransfers(options TransferOptions) {
	m.download, m.upload = options.Download, options.Upload
}

// OpenSSH publishes a session before connecting. The connector receives only its
// ID; the manager owns identity updates and synchronization.
func (m *Manager) OpenSSH(alias string, options LaunchOptions, connect func(context.Context, string) (*service.NativeSSHClient, service.Host, error)) (Session, error) {
	return m.openSSHWithOptions(alias, options, func(ctx context.Context, entry *Session) (*service.NativeSSHClient, string, error) {
		client, host, err := connect(ctx, entry.ID)
		if err == nil {
			m.mu.Lock()
			identity := host
			entry.connectionIdentity = &identity
			m.mu.Unlock()
		}
		return client, host.DisplayName(), err
	})
}

func (m *Manager) Snapshot(id string) (Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.sessions[id]
	if s == nil {
		return Session{}, errors.New("terminal session was not found")
	}
	return s.snapshot(), nil
}
func (m *Manager) IDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Present sets the workbench metadata once a session has been opened.
func (m *Manager) Present(id, hostID string, local bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[id]; s != nil {
		s.HostID = hostID
		if s.Columns == 0 || s.Rows == 0 {
			s.Columns, s.Rows = 80, 24
		}
		if local {
			m.localCounter++
			s.LocalNumber = m.localCounter
		}
	}
}
func (m *Manager) CancelTransfer(id string) error {
	s, err := m.session(id)
	if err != nil {
		return err
	}
	if s.transfer != nil {
		return s.transfer.Cancel()
	}
	return nil
}

// ExecutionContext returns a consistent execution context. Nested OpenSSH sessions
// cannot fall back to executing on their enclosing local shell.
func (m *Manager) ExecutionContext(id string) (Session, *service.NativeSSHClient, *service.Host, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.sessions[id]
	if s == nil {
		return Session{}, nil, nil, errors.New("terminal disconnected")
	}
	if s.Remote != nil {
		return Session{}, nil, nil, errors.New("terminal disconnected: Nested SSH execution is unavailable. Open a saved SSH connection instead.")
	}
	var identity *service.Host
	if s.connectionIdentity != nil {
		copy := *s.connectionIdentity
		identity = &copy
	}
	return s.snapshot(), s.sshClient, identity, nil
}
func (m *Manager) ClientForHost(id, hostID string) *service.NativeSSHClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.sessions[id]
	if s != nil && s.Kind == "ssh" && s.State == "running" && s.HostID == hostID {
		return s.sshClient
	}
	return nil
}
func (m *Manager) OpenFiles(ctx context.Context, id, contextID string) (*sftp.Client, io.Closer, string, error) {
	m.mu.RLock()
	s := m.sessions[id]
	valid := s != nil && s.Kind == "local" && s.State == "running" && s.Remote != nil && s.Remote.ID == contextID && s.integration != nil
	if !valid {
		m.mu.RUnlock()
		return nil, nil, "", errors.New("SSH session does not match this file workspace")
	}
	integration := s.integration
	title := s.Remote.User + "@" + s.Remote.Host
	m.mu.RUnlock()
	client, owner, err := integration.OpenFiles(ctx, contextID)
	return client, owner, title, err
}
