package files

import (
	"errors"
	"testing"
)

func TestEditorPreservesNewDraftWhileSaveIsInFlight(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	editor := NewEditor(Text{Path: "/config", Text: "original", Revision: "one"}, "Test", func(document Text) (Text, error) {
		close(started)
		<-finish
		document.Revision = "two"
		return document, nil
	})
	if err := editor.Change("first edit"); err != nil {
		t.Fatal(err)
	}
	go func() { done <- editor.Save(nil) }()
	<-started
	if !editor.State().Saving {
		t.Fatal("editor did not expose in-flight save")
	}
	if err := editor.Change("latest edit"); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	state := editor.State()
	if state.Saving || state.Document.Text != "first edit" || state.Text != "latest edit" {
		t.Fatalf("concurrent edit was lost: %#v", state)
	}
}
func TestEditorKeepsUnsavedTextAfterConflictOrDisconnect(t *testing.T) {
	attempts := 0
	editor := NewEditor(Text{Path: "/config", Text: "original", Revision: "one"}, "Test", func(document Text) (Text, error) { attempts++; return Text{}, errors.New("remote file changed") })
	editor.Change("unsaved")
	if err := editor.Save(nil); err == nil {
		t.Fatal("save failure disappeared")
	}
	state := editor.State()
	if state.Text != "unsaved" || state.Document.Text != "original" || state.Error != "remote file changed" {
		t.Fatalf("conflict damaged draft: %#v", state)
	}
	editor.Disconnect()
	if err := editor.Save(nil); err == nil {
		t.Fatal("disconnected save accepted")
	}
	if editor.State().Text != "unsaved" || attempts != 1 {
		t.Fatal("disconnect lost draft or attempted remote save")
	}
}
