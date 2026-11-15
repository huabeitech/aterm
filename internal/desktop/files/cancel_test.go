package files

import (
	"context"
	"io"
	"sync"
	"testing"
)

type blockingReadCloser struct {
	once    sync.Once
	closed  chan struct{}
	entered chan struct{}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	close(r.entered)
	<-r.closed
	return 0, io.ErrClosedPipe
}
func (r *blockingReadCloser) Close() error { r.once.Do(func() { close(r.closed) }); return nil }

type discardCloser struct{}

func (discardCloser) Write(data []byte) (int, error) { return len(data), nil }
func (discardCloser) Close() error                   { return nil }
func TestCancellationClosesBlockedTransferStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &blockingReadCloser{closed: make(chan struct{}), entered: make(chan struct{})}
	task := &transferTask{info: Transfer{ID: "task", SessionID: "session", State: "running"}, ctx: ctx, cancel: cancel}
	manager := &Transfers{tasks: map[string]*transferTask{"task": task}}
	done := make(chan error, 1)
	go func() { done <- manager.copy(ctx, task, discardCloser{}, source) }()
	<-source.entered
	if err := manager.Cancel("session", "task"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("cancelled stream reported success")
	}
}
