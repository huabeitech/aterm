package terminal

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestOutputSnapshotHasAbsoluteByteWatermark(t *testing.T) {
	var events []OutputEvent
	manager := NewManager(context.Background(), func(_ string, data any) { events = append(events, data.(OutputEvent)) })
	session := &Session{ID: "one"}
	manager.sessions[session.ID] = session
	_, _ = manager.AttachOutput(session.ID)
	manager.appendOutput(session, []byte("你好"))
	snapshot, err := manager.OutputSnapshot(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Offset != 6 || events[0].Offset != 6 {
		t.Fatal("watermark must count UTF8 bytes")
	}
	decoded, _ := base64.StdEncoding.DecodeString(snapshot.Data)
	if string(decoded) != "你好" {
		t.Fatalf("snapshot %q", decoded)
	}
	manager.appendOutput(session, make([]byte, (terminalReplayLimit+64*1024)))
	snapshot, err = manager.OutputSnapshot(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ = base64.StdEncoding.DecodeString(snapshot.Data)
	if len(decoded) != terminalReplayLimit || snapshot.Offset != (terminalReplayLimit+64*1024)+6 {
		t.Fatal("truncation reset absolute watermark")
	}
}
