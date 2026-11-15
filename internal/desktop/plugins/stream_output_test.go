package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func nextStreamEvent(t *testing.T, events <-chan StreamEvent) StreamEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("missing stream event")
		return StreamEvent{}
	}
}
func TestStreamOutputBackpressureUTF8AndFinalAck(t *testing.T) {
	events := make(chan StreamEvent, 20)
	var disposed atomic.Int32
	output := NewStreamOutput("fixture", 8, func(event StreamEvent) error { events <- event; return nil }, func() { disposed.Add(1) }, func() time.Duration { return time.Millisecond })
	chinese := []byte("中")
	output.Push(chinese[:1], false)
	output.Push(chinese[1:], false)
	first := nextStreamEvent(t, events)
	if first.Text != "中" || first.Done {
		t.Fatal(first)
	}
	for i := 0; i < 10; i++ {
		output.Push([]byte("abcdef"), i%2 == 0)
	}
	// A producer can exceed the budget repeatedly while one batch is unacknowledged.
	output.mu.Lock()
	length := output.buffer.length
	output.mu.Unlock()
	if length > 8 {
		t.Fatal("tail buffer grew beyond its budget")
	}
	select {
	case event := <-events:
		t.Fatal("consumer received an unacknowledged second batch", event)
	default:
	}
	code := 0
	output.Finish(&code, "")
	if disposed.Load() != 0 {
		t.Fatal("final payload disposed before acknowledgement")
	}
	output.Acknowledge()
	last := nextStreamEvent(t, events)
	if !last.Done || !last.Truncated || last.Text != "efabcdef" || last.ExitCode == nil || *last.ExitCode == nil || **last.ExitCode != 0 {
		t.Fatal(last)
	}
	output.Acknowledge()
	if disposed.Load() != 1 {
		t.Fatal("stream not disposed exactly once")
	}
	output.Cancel("late")
	output.Push([]byte("late"), false)
	select {
	case event := <-events:
		t.Fatal("closed stream emitted again", event)
	default:
	}
}
func TestStreamCancellationDiscardsPendingBytesAndStaleTimeout(t *testing.T) {
	events := make(chan StreamEvent, 20)
	var disposed atomic.Int32
	output := NewStreamOutput("fixture", 32, func(event StreamEvent) error { events <- event; return nil }, func() { disposed.Add(1) }, func() time.Duration { return time.Millisecond })
	output.Push([]byte("first"), false)
	nextStreamEvent(t, events)
	output.mu.Lock()
	oldBatch := output.batch
	output.mu.Unlock()
	output.mu.Lock()
	output.interval = func() time.Duration { return time.Hour }
	output.mu.Unlock()
	output.Acknowledge()
	output.cancel("stale timeout", &oldBatch)
	if disposed.Load() != 0 {
		t.Fatal("an acknowledged batch's watchdog cancelled its successor")
	}
	output.Push([]byte("must-not-be-flushed"), false)
	output.Cancel("Execution cancelled")
	last := nextStreamEvent(t, events)
	if !last.Done || last.Text != "" || last.Error != "Execution cancelled" || disposed.Load() != 1 {
		t.Fatal(last)
	}
}
func TestStreamConsumerTimeoutReleasesWithoutAck(t *testing.T) {
	events := make(chan StreamEvent, 20)
	disposed := make(chan struct{})
	output := NewStreamOutput("fixture", 32, func(event StreamEvent) error { events <- event; return nil }, func() { close(disposed) }, func() time.Duration { return time.Millisecond })
	output.ackTimeout = 15 * time.Millisecond
	output.Push([]byte("data"), false)
	nextStreamEvent(t, events)
	last := nextStreamEvent(t, events)
	if !last.Done || last.Text != "" || last.Error != "Stream consumer timed out" {
		t.Fatal(last)
	}
	select {
	case <-disposed:
	case <-time.After(time.Second):
		t.Fatal("stalled consumer leaked stream")
	}
}
func TestExecutorStreamsAreGrantBoundAndShareConcurrency(t *testing.T) {
	events := make(chan StreamEvent, 30)
	started := make(chan struct{}, 20)
	executor := NewExecutor(ExecutionOptions{Audit: NewAudit(filepath.Join(t.TempDir(), "audit.json")), StreamEvent: func(_ string, event StreamEvent) error { events <- event; return nil }, Run: func(ctx context.Context, _ Session, _ ProcessRequest, _ ExecutionPolicy, onData ProcessData) (ProcessResult, error) {
		started <- struct{}{}
		if onData != nil {
			onData([]byte("fixture output"), false)
		}
		<-ctx.Done()
		return ProcessResult{}, ctx.Err()
	}})
	manifest := fixturePackage(t).Manifest
	manifest.Permissions = []string{"process.execute"}
	manifest.Execution = &ExecutionPolicy{Programs: []string{"printf"}, Targets: []string{"local"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := CapabilityScope{PluginID: manifest.ID, Manifest: manifest, Owner: 1, Session: Session{ID: "source", Kind: "local"}, GrantID: strings.Repeat("a", 64), Context: ctx, Authorized: func() bool { return ctx.Err() == nil }}
	request := func(id string) json.RawMessage {
		return json.RawMessage(`{"id":"` + id + `","request":{"executable":"printf","args":[]}}`)
	}
	for _, id := range []string{"one", "two", "three", "four"} {
		if _, err := executor.Request(context.Background(), "process.stream.start", request(id), scope); err != nil {
			t.Fatal(err)
		}
		<-started
	}
	if _, err := executor.Request(context.Background(), "process.stream.start", request("fifth"), scope); err == nil {
		t.Fatal("streams escaped the shared execution budget")
	}
	other := scope
	other.Context = context.Background()
	if _, err := executor.Request(context.Background(), "process.stream.ack", json.RawMessage(`{"id":"one"}`), other); err == nil {
		t.Fatal("another job controlled an existing stream")
	}
	cancel()
	for ended := 0; ended < 4; {
		event := nextStreamEvent(t, events)
		if event.Done {
			ended++
			if event.Text != "" {
				t.Fatal("cancelled stream flushed buffered data", event)
			}
		}
	}
	deadline := time.After(time.Second)
	for {
		executor.mu.Lock()
		remaining, total := len(executor.streams), executor.total
		executor.mu.Unlock()
		if remaining == 0 && total == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("stream lifetime did not release resources")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestRealProcessStreamCompletesAndIsAcknowledged(t *testing.T) {
	events := make(chan StreamEvent, 20)
	executor := NewExecutor(ExecutionOptions{Audit: NewAudit(filepath.Join(t.TempDir(), "audit.json")), StreamEvent: func(_ string, event StreamEvent) error { events <- event; return nil }, Run: func(ctx context.Context, _ Session, r ProcessRequest, p ExecutionPolicy, onData ProcessData) (ProcessResult, error) {
		return RunLocalProcess(ctx, r, p, onData)
	}})
	manifest := fixturePackage(t).Manifest
	manifest.Permissions = []string{"process.execute"}
	manifest.Execution = &ExecutionPolicy{Programs: []string{"sh"}, Targets: []string{"local"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := CapabilityScope{PluginID: manifest.ID, Manifest: manifest, Owner: 1, Session: Session{ID: "fixture", Kind: "local"}, GrantID: strings.Repeat("b", 64), Context: ctx, Authorized: func() bool { return ctx.Err() == nil }}
	input := json.RawMessage(`{"id":"actual","request":{"executable":"sh","args":["-c","printf '\\344'; sleep 0.01; printf '\\270\\255'; printf stderr >&2"]}}`)
	if _, err := executor.Request(context.Background(), "process.stream.start", input, scope); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for {
		event := nextStreamEvent(t, events)
		text.WriteString(event.Text)
		if _, err := executor.Request(context.Background(), "process.stream.ack", json.RawMessage(`{"id":"actual"}`), scope); err != nil {
			t.Fatal(err)
		}
		if event.Done {
			if event.Error != "" || event.ExitCode == nil || *event.ExitCode == nil || **event.ExitCode != 0 {
				t.Fatal(event)
			}
			break
		}
	}
	if !strings.Contains(text.String(), "中") || !strings.Contains(text.String(), "stderr") {
		t.Fatal("real pipe stream lost UTF-8 or stderr", text.String())
	}
	executor.mu.Lock()
	remaining := len(executor.streams)
	executor.mu.Unlock()
	if remaining != 0 {
		t.Fatal("acknowledged final stream retained its state")
	}
}
