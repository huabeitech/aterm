package plugin

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type noopHost struct{}

func (noopHost) Call(context.Context, string, any, any) error        { return nil }
func (noopHost) Open(context.Context, string, any) (net.Conn, error) { return nil, nil }

type blockedProgram struct {
	started   chan struct{}
	canceled  atomic.Bool
	released  atomic.Bool
	refreshes atomic.Int32
}

func (p *blockedProgram) Refresh(ctx context.Context, _ SessionInfo) error {
	p.refreshes.Add(1)
	close(p.started)
	<-ctx.Done()
	p.canceled.Store(true)
	return ctx.Err()
}
func (p *blockedProgram) Command(context.Context, SessionInfo, string, json.RawMessage) (any, error) {
	return nil, nil
}
func (p *blockedProgram) Stream(context.Context, StreamEvent) error { return nil }
func (p *blockedProgram) Release(string) {
	if !p.canceled.Load() {
		panic("released before in-flight sampling ended")
	}
	p.released.Store(true)
}
func (p *blockedProgram) Tasks() int { return 0 }
func (p *blockedProgram) Close()     {}
func TestSessionReleaseCancelsSamplingAndRejectsLateRefresh(t *testing.T) {
	p := &blockedProgram{started: make(chan struct{})}
	r := &SessionRuntime{Host: noopHost{}, Program: p, sessions: map[string]*sessionLease{}, released: map[string]bool{}}
	done := make(chan struct{})
	go func() {
		r.Call(context.Background(), "runtime.refresh", JSON(map[string]any{"token": "lease", "context": SessionInfo{SessionID: "fixture"}}))
		close(done)
	}()
	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("sampling did not start")
	}
	if _, e := r.Call(context.Background(), "runtime.release", JSON(map[string]string{"token": "lease"})); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sampling survived release")
	}
	if !p.released.Load() {
		t.Fatal("scope resources not released")
	}
	r.Call(context.Background(), "runtime.refresh", JSON(map[string]any{"token": "lease", "context": SessionInfo{SessionID: "fixture"}}))
	if p.refreshes.Load() != 1 {
		t.Fatal("late refresh resurrected released scope")
	}
	if _, e := r.Call(context.Background(), "runtime.command", JSON(map[string]string{"token": "lease", "command": "anything"})); e == nil {
		t.Fatal("released scope retained commands")
	}
}
func TestLocaleRevisionPreventsDelayedRefreshOverridingCurrentLanguage(t *testing.T) {
	p := &blockedProgram{started: make(chan struct{})}
	r := &SessionRuntime{Host: noopHost{}, Program: p, sessions: map[string]*sessionLease{}, released: map[string]bool{}}
	bundle := map[string]any{"defaultLocale": "en", "messages": map[string]map[string]string{"en": {"label": "Memory"}, "zh-CN": {"label": "内存"}}}
	r.Call(context.Background(), "runtime.activate", JSON(map[string]any{"locale": "en", "localeRevision": 0, "localizations": bundle}))
	r.Call(context.Background(), "runtime.locale", JSON(map[string]any{"locale": "zh-CN", "localeRevision": 1}))
	r.released["late"] = true
	// A delayed refresh still supplies the original language, but cannot roll back
	// an already-open view's newer language revision.
	r.Call(context.Background(), "runtime.refresh", JSON(map[string]any{"token": "late", "locale": "en", "localeRevision": 0, "localizations": bundle}))
	if r.T("label", nil) != "内存" {
		t.Fatal("stale refresh reset current language")
	}
}
