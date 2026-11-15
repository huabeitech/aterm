package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNativeSSHKeepaliveConfigDisabled(t *testing.T) {
	config := nativeSSHKeepaliveConfigForHost(Host{Alias: "prod", ServerAliveCountMax: 9})
	if config.interval != 0 {
		t.Fatalf("interval = %s, want disabled", config.interval)
	}
	requester := newRecordingKeepaliveRequester(0)
	runNativeSSHKeepalive(context.Background(), requester, config, func(error) error {
		t.Fatal("disabled keepalive failed")
		return nil
	})
	requester.mu.Lock()
	defer requester.mu.Unlock()
	if requester.calls != 0 {
		t.Fatalf("request count = %d, want 0", requester.calls)
	}
}

func TestNativeSSHKeepaliveConfigUsesDefaultCount(t *testing.T) {
	config := nativeSSHKeepaliveConfigForHost(Host{Alias: "prod", ServerAliveInterval: 30})
	if config.interval != 30*time.Second {
		t.Fatalf("interval = %s, want 30s", config.interval)
	}
	if config.count != defaultServerAliveCountMax {
		t.Fatalf("count = %d, want %d", config.count, defaultServerAliveCountMax)
	}
}

func TestNativeSSHKeepaliveSendsOpenSSHRequest(t *testing.T) {
	requester := newRecordingKeepaliveRequester(1)
	requester.responses <- nil
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNativeSSHKeepalive(ctx, requester, nativeSSHKeepaliveConfig{alias: "prod", interval: time.Hour, count: 3}, func(error) error {
			t.Error("healthy keepalive failed")
			return nil
		})
		close(done)
	}()

	request := waitForKeepaliveRequest(t, requester.requests)
	if request.name != "keepalive@openssh.com" {
		t.Fatalf("request name = %q", request.name)
	}
	if !request.wantReply {
		t.Fatal("keepalive request did not request a reply")
	}
	cancel()
	waitForKeepaliveDone(t, done)
}

func TestNativeSSHKeepaliveTimesOutWithOneOutstandingRequest(t *testing.T) {
	requester := newRecordingKeepaliveRequester(0)
	failed := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		runNativeSSHKeepalive(context.Background(), requester, nativeSSHKeepaliveConfig{alias: "prod", interval: 5 * time.Millisecond, count: 2}, func(err error) error {
			failed <- err
			close(requester.responses)
			return nil
		})
		close(done)
	}()

	err := waitForKeepaliveFailure(t, failed)
	if !errors.Is(err, ErrSSHKeepaliveTimeout) {
		t.Fatalf("failure = %v, want ErrSSHKeepaliveTimeout", err)
	}
	var timeout *SSHKeepaliveTimeoutError
	if !errors.As(err, &timeout) || timeout.Alias != "prod" || timeout.Count != 2 {
		t.Fatalf("timeout details = %#v", timeout)
	}
	waitForKeepaliveDone(t, done)

	requester.mu.Lock()
	defer requester.mu.Unlock()
	if requester.calls != 1 {
		t.Fatalf("request count = %d, want one in-flight request", requester.calls)
	}
	if requester.maxConcurrent != 1 {
		t.Fatalf("maximum concurrent requests = %d, want 1", requester.maxConcurrent)
	}
}

func TestNativeSSHKeepaliveReplyResetsMisses(t *testing.T) {
	requester := newRecordingKeepaliveRequester(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		runNativeSSHKeepalive(ctx, requester, nativeSSHKeepaliveConfig{alias: "prod", interval: 15 * time.Millisecond, count: 2}, func(err error) error {
			failed <- err
			return nil
		})
		close(done)
	}()

	waitForKeepaliveRequest(t, requester.requests)
	time.Sleep(8 * time.Millisecond)
	requester.responses <- nil
	waitForKeepaliveRequest(t, requester.requests)

	select {
	case err := <-failed:
		t.Fatalf("keepalive failed after a response reset the miss count: %v", err)
	case <-time.After(8 * time.Millisecond):
	}

	cancel()
	requester.responses <- nil
	waitForKeepaliveDone(t, done)
}

func TestPreferNativeSSHCausePreservesKeepaliveTimeout(t *testing.T) {
	cause := &SSHKeepaliveTimeoutError{Alias: "prod", Interval: 30 * time.Second, Count: 3}
	client := &NativeSSHClient{cause: cause}
	got := preferNativeSSHCause(client, errors.New("connection closed"))
	if !errors.Is(got, ErrSSHKeepaliveTimeout) {
		t.Fatalf("error = %v, want keepalive cause", got)
	}
}

type keepaliveRequest struct {
	name      string
	wantReply bool
}

type recordingKeepaliveRequester struct {
	responses chan error
	requests  chan keepaliveRequest

	mu            sync.Mutex
	calls         int
	concurrent    int
	maxConcurrent int
}

func newRecordingKeepaliveRequester(responseBuffer int) *recordingKeepaliveRequester {
	return &recordingKeepaliveRequester{
		responses: make(chan error, responseBuffer),
		requests:  make(chan keepaliveRequest, 16),
	}
}

func (r *recordingKeepaliveRequester) SendRequest(name string, wantReply bool, _ []byte) (bool, []byte, error) {
	r.mu.Lock()
	r.calls++
	r.concurrent++
	if r.concurrent > r.maxConcurrent {
		r.maxConcurrent = r.concurrent
	}
	requests := r.requests
	r.mu.Unlock()

	requests <- keepaliveRequest{name: name, wantReply: wantReply}
	err, ok := <-r.responses

	r.mu.Lock()
	r.concurrent--
	r.mu.Unlock()
	if !ok {
		return false, nil, errors.New("connection closed")
	}
	return false, nil, err
}

func waitForKeepaliveRequest(t *testing.T, requests <-chan keepaliveRequest) keepaliveRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for keepalive request")
		return keepaliveRequest{}
	}
}

func waitForKeepaliveFailure(t *testing.T, failed <-chan error) error {
	t.Helper()
	select {
	case err := <-failed:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for keepalive failure")
		return nil
	}
}

func waitForKeepaliveDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for keepalive worker to stop")
	}
}
