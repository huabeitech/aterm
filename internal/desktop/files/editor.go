package files

import (
	"errors"
	"sync"
)

type EditorState struct {
	Document  Text   `json:"document"`
	Text      string `json:"text"`
	Host      string `json:"host"`
	Connected bool   `json:"connected"`
	Saving    bool   `json:"saving"`
	Error     string `json:"error"`
}

// Editor owns unsaved content independently of its SFTP view and native window.
// A save snapshots the draft; edits arriving during I/O remain dirty afterward.
type Editor struct {
	mu    sync.Mutex
	state EditorState
	save  func(Text) (Text, error)
}

func NewEditor(document Text, host string, save func(Text) (Text, error)) *Editor {
	return &Editor{state: EditorState{Document: document, Text: document.Text, Host: host, Connected: true}, save: save}
}
func (e *Editor) State() EditorState { e.mu.Lock(); defer e.mu.Unlock(); return e.state }
func (e *Editor) Change(text string) error {
	if len(text) > MaxTextBytes {
		return errors.New("text must be no larger than 1 MB")
	}
	e.mu.Lock()
	e.state.Text = text
	e.mu.Unlock()
	return nil
}
func (e *Editor) Disconnect() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state.Connected = false
	e.state.Error = "SFTP connection closed. Your edits are kept here; copy them before closing this window."
}
func (e *Editor) Save(notify func()) error {
	e.mu.Lock()
	if e.state.Saving || e.state.Text == e.state.Document.Text {
		e.mu.Unlock()
		return nil
	}
	if !e.state.Connected {
		err := errors.New(e.state.Error)
		e.mu.Unlock()
		return err
	}
	e.state.Saving = true
	e.state.Error = ""
	document := e.state.Document
	document.Text = e.state.Text
	e.mu.Unlock()
	if notify != nil {
		notify()
	}
	saved, err := e.save(document)
	e.mu.Lock()
	e.state.Saving = false
	if err != nil {
		e.state.Error = err.Error()
	} else {
		e.state.Document = saved
	}
	e.mu.Unlock()
	if notify != nil {
		notify()
	}
	return err
}
