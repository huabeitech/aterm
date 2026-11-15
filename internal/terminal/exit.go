package terminal

import "time"

// Descendants can inherit a PTY and hold it open after the shell exits. Give
// already produced output time to drain, without leaving an exited shell's
// transport open indefinitely. Renderer backpressure suspends this grace period.
const terminalExitDrainGrace = 2 * time.Second

func (m *Manager) drainTerminalExit(session *Session, interrupt func()) {
	if session.outputDone == nil {
		return
	}
	timer := time.NewTimer(terminalExitDrainGrace)
	defer timer.Stop()
	for {
		select {
		case <-session.outputDone:
			return
		case <-session.closed:
			return
		case <-m.ctx.Done():
			interrupt()
			return
		case <-timer.C:
			m.mu.RLock()
			paused := session.outputPaused
			resumedAt := session.outputResumedAt
			m.mu.RUnlock()
			if paused {
				timer.Reset(terminalExitDrainGrace)
				continue
			}
			if remaining := time.Until(resumedAt.Add(terminalExitDrainGrace)); remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			// Interrupt an inherited/stalled transport, then let its reader finish its
			// already accepted bytes. Close/whole-app cancellation still wake the wait.
			if session.transfer != nil {
				session.transfer.Close()
			}
			interrupt()
			select {
			case <-session.outputDone:
			case <-session.closed:
			case <-m.ctx.Done():
			}
			return
		}
	}
}

func (m *Manager) finishTerminalExit(session *Session, exitCode int) {
	m.mu.Lock()
	if m.sessions[session.ID] != session || session.State == "exited" {
		m.mu.Unlock()
		return
	}
	session.Remote = nil
	session.State = "exited"
	session.ExitCode = &exitCode
	m.mu.Unlock()
	m.emitEvent(terminalEventName, OutputEvent{SessionID: session.ID, State: "exited", ExitCode: &exitCode})
}
