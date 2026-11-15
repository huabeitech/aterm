package terminal

import (
	"context"
	"errors"
	"io"
	"sync"

	"ssh-connect/internal/desktop/zmodem"
)

func (m *Manager) setupTransfer(session *Session) {
	if m.download == nil && m.upload == nil {
		return
	}
	session.transfer = zmodem.NewRouter(m.ctx, zmodem.RouterOptions{
		Write:  func(data []byte) error { return m.writeBytes(session.ID, data) },
		Output: func(data []byte) { m.appendTerminalText(session, data) },
		Offer: func(ctx context.Context, offer zmodem.FileOffer) (zmodem.Download, error) {
			if m.download == nil {
				return nil, errors.New("ZMODEM download selection is unavailable")
			}
			return m.download(ctx, session.ID, offer)
		},
		Select: func(ctx context.Context) ([]zmodem.Upload, error) {
			if m.upload == nil {
				return nil, errors.New("ZMODEM file selection is unavailable")
			}
			return m.upload(ctx, session.ID)
		},
		Progress: func(progress zmodem.Progress) {
			m.emitEvent("terminal:event", map[string]any{"type": "zmodem", "id": session.ID, "progress": progress})
		},
	})
}
func (m *Manager) forwardSSHOutput(session *Session, stdout, stderr io.Reader) {
	if session.outputDone != nil {
		defer close(session.outputDone)
	}
	var readers sync.WaitGroup
	readers.Add(2)
	for _, reader := range []io.Reader{stdout, stderr} {
		go func() { defer readers.Done(); m.forwardReader(session, reader) }()
	}
	readers.Wait()
	if session.transfer != nil {
		session.transfer.Close()
	}
}
func (m *Manager) WriteBinary(id string, data []byte) error {
	session, err := m.session(id)
	if err != nil {
		return err
	}
	if session.transfer != nil && session.transfer.Busy() {
		return nil
	}
	return m.writeBytes(id, data)
}
