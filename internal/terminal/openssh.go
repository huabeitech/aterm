package terminal

import "ssh-connect/internal/desktop/openssh"

func (m *Manager) watchOpenSSH(session *Session) {
	if session.integration == nil {
		return
	}
	session.integration.Start(func(remote *openssh.RemoteContext) {
		m.mu.Lock()
		if m.sessions[session.ID] != session || session.State != "running" {
			m.mu.Unlock()
			return
		}
		session.Remote = remote
		refreshLocalTitle(session)
		m.mu.Unlock()
		// The existing state dispatcher publishes the updated shared terminal
		// contract and invalidates plugin work tied to the previous environment.
		m.emitEvent(terminalEventName, OutputEvent{SessionID: session.ID, State: "running"})
	})
}
