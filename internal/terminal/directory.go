package terminal

import (
	"os"
	"path/filepath"
	"runtime"
	"ssh-connect/internal/desktop/terminalmeta"
	"time"
)

func refreshLocalTitle(session *Session) {
	if session.pluginTitle != "" {
		session.Title = session.pluginTitle
		return
	}
	if session.Remote != nil {
		session.Title = session.Remote.User + "@" + session.Remote.Host
		return
	}
	if session.Cwd == "" {
		session.Title = "Local"
		return
	}
	home, _ := os.UserHomeDir()
	if session.Cwd == home {
		session.Title = "~"
		return
	}
	session.Title = filepath.Base(session.Cwd)
}
func (m *Manager) startDirectoryPolling() {
	// Windows directory changes are reported by shell integration, not Unix PID probes.
	if runtime.GOOS == "windows" {
		return
	}
	m.mu.Lock()
	if m.directoryPolling {
		m.mu.Unlock()
		return
	}
	m.directoryPolling = true
	m.mu.Unlock()
	go m.pollDirectories()
}
func (m *Manager) pollDirectories() {
	for {
		m.mu.Lock()
		sessions := []*Session{}
		pids := []int{}
		for _, session := range m.sessions {
			if session.Kind == "local" && session.State == "running" && session.PID > 0 {
				sessions = append(sessions, session)
				pids = append(pids, session.PID)
			}
		}
		if len(sessions) == 0 || m.ctx.Err() != nil {
			m.directoryPolling = false
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
		directories := terminalmeta.ReadWorkingDirectories(m.ctx, pids)
		changed := []string{}
		m.mu.Lock()
		for _, session := range sessions {
			if m.sessions[session.ID] != session || session.State != "running" {
				continue
			}
			cwd := directories[session.PID]
			if session.Cwd == cwd {
				continue
			}
			session.Cwd = cwd
			refreshLocalTitle(session)
			changed = append(changed, session.ID)
		}
		m.mu.Unlock()
		for _, id := range changed {
			m.emitEvent(terminalEventName, OutputEvent{SessionID: id, State: "running"})
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-m.ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// Reports arrive on the PTY reader; recheck ownership before publishing them.
func (m *Manager) updateReportedDirectory(session *Session, path string) {
	m.mu.Lock()
	if m.sessions[session.ID] != session || session.State != "running" || session.Cwd == path {
		m.mu.Unlock()
		return
	}
	session.Cwd = path
	refreshLocalTitle(session)
	m.mu.Unlock()
	m.emitEvent(terminalEventName, OutputEvent{SessionID: session.ID, State: "running"})
}
