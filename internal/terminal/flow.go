package terminal

import (
	"encoding/base64"
	"errors"
	"time"
)

const terminalReplayLimit = 1024 * 1024

// All flow state is protected by Manager.mu. Closing a wake channel
// broadcasts to both SSH readers without holding the lock during transport I/O.
func wakeTerminalOutput(s *Session) {
	if s.outputWake != nil {
		close(s.outputWake)
		s.outputWake = nil
	}
}

func (m *Manager) AttachOutput(id string) (OutputSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil {
		return OutputSnapshot{}, errors.New("terminal session was not found")
	}
	s.outputAttached = true
	s.outputAcknowledged = s.outputOffset
	s.outputPaused = false
	s.outputResumedAt = time.Now()
	wakeTerminalOutput(s)
	return OutputSnapshot{Data: base64.StdEncoding.EncodeToString(s.output), Offset: s.outputOffset}, nil
}

func (m *Manager) AcknowledgeOutput(id string, seq uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil {
		return errors.New("terminal session was not found")
	}
	if seq > s.outputOffset {
		return errors.New("invalid terminal acknowledgement")
	}
	if seq > s.outputAcknowledged {
		s.outputAcknowledged = seq
	}
	if s.outputOffset-s.outputAcknowledged < 128*1024 {
		if s.outputPaused {
			s.outputResumedAt = time.Now()
		}
		s.outputPaused = false
		wakeTerminalOutput(s)
	}
	return nil
}
