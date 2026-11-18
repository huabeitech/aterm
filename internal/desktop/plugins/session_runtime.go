package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// SessionRuntime dispatches script and native workers through the same session
// authorization, visibility, refresh and stream contracts. It knows no plugin IDs.
type SessionRuntime struct {
	script         *RuntimeHost
	native         *NativeManager
	mu             sync.Mutex
	entries        map[string]*nativeSessionEntry
	locale         string
	localeRevision uint64
}
type nativeSessionEntry struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	bundle  *Localizations
	scopes  map[string]bool
	retired []string
}

func NewSessionRuntime(script *RuntimeHost, native *NativeManager) *SessionRuntime {
	return &SessionRuntime{script: script, native: native, entries: map[string]*nativeSessionEntry{}, locale: "en"}
}
func SessionScoped(m Manifest) bool {
	if m.Contributes.TerminalStatusBar != nil {
		return true
	}
	for _, v := range m.Contributes.Views {
		if v.Placement == "terminal.workspace" || v.Placement == "terminal.sidePanel" {
			return true
		}
	}
	return false
}
func (r *SessionRuntime) Start(ctx context.Context, id string, p Package, fault func(string)) (string, error) {
	if p.Manifest.Runtime != "native" {
		return r.script.Start(ctx, id, p, fault)
	}
	ctx, cancel := context.WithCancel(ctx)
	instance := "native:" + uuid.NewString()
	r.mu.Lock()
	locale, revision := r.locale, r.localeRevision
	r.mu.Unlock()
	_, err := r.native.Call(ctx, id, "runtime.activate", jsonSnapshot(map[string]any{"locale": locale, "localeRevision": revision, "localizations": p.Localizations}))
	if err != nil {
		cancel()
		return "", err
	}
	r.mu.Lock()
	r.entries[instance] = &nativeSessionEntry{id: id, ctx: ctx, cancel: cancel, bundle: p.Localizations, scopes: map[string]bool{}}
	r.mu.Unlock()
	return instance, nil
}
func jsonSnapshot(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func (r *SessionRuntime) entry(id string) (*nativeSessionEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	return e, ok
}
func (r *SessionRuntime) Stop(instance string) {
	e, ok := r.entry(instance)
	if !ok {
		if !strings.HasPrefix(instance, "native:") {
			r.script.Stop(instance)
		}
		return
	}
	r.mu.Lock()
	delete(r.entries, instance)
	tokens := make([]string, 0, len(e.scopes))
	for token := range e.scopes {
		e.scopes[token] = false
		tokens = append(tokens, token)
	}
	r.mu.Unlock()
	e.cancel()
	for _, token := range tokens {
		_ = r.native.Visibility(e.id, "scope:"+token, false)
	}
	_ = r.native.Change(e.id, func() error { return nil })
}
func (r *SessionRuntime) Refresh(instance, token string, value any) error {
	e, ok := r.entry(instance)
	if !ok {
		if strings.HasPrefix(instance, "native:") {
			return errors.New("native session runtime unavailable")
		}
		return r.script.Refresh(instance, token, value)
	}
	r.mu.Lock()
	active, seen := e.scopes[token]
	if !seen {
		e.scopes[token] = true
		active = true
	}
	r.mu.Unlock()
	if !active {
		return nil
	}
	if err := r.native.Visibility(e.id, "scope:"+token, true); err != nil {
		return err
	}
	r.mu.Lock()
	locale, revision := r.locale, r.localeRevision
	active = e.scopes[token]
	r.mu.Unlock()
	if !active {
		_ = r.native.Visibility(e.id, "scope:"+token, false)
		return nil
	}
	go func() {
		_, _ = r.native.Call(e.ctx, e.id, "runtime.refresh", jsonSnapshot(map[string]any{"token": token, "context": value, "locale": locale, "localeRevision": revision, "localizations": e.bundle}))
	}()
	return nil
}
func (r *SessionRuntime) Command(ctx context.Context, instance, token, command string, args json.RawMessage) (json.RawMessage, error) {
	e, ok := r.entry(instance)
	if !ok {
		if strings.HasPrefix(instance, "native:") {
			return nil, errors.New("native session runtime unavailable")
		}
		return r.script.Command(ctx, instance, token, command, args)
	}
	return r.native.Call(ctx, e.id, "runtime.command", jsonSnapshot(map[string]any{"token": token, "command": command, "args": args}))
}
func (r *SessionRuntime) StreamEvent(instance string, event StreamEvent) error {
	e, ok := r.entry(instance)
	if !ok {
		if strings.HasPrefix(instance, "native:") {
			return nil
		}
		return r.script.StreamEvent(instance, event)
	}
	// Delivery must not block executor cancellation, which may emit a final frame.
	go func() { _, _ = r.native.CallRunning(e.ctx, e.id, "runtime.stream", jsonSnapshot(event)) }()
	return nil
}
func (r *SessionRuntime) ReleaseScope(instance, token string) {
	if e, ok := r.entry(instance); ok {
		r.mu.Lock()
		if active, seen := e.scopes[token]; !seen || active {
			e.retired = append(e.retired, token)
		}
		e.scopes[token] = false
		if len(e.retired) > 1024 {
			delete(e.scopes, e.retired[0])
			e.retired = e.retired[1:]
		}
		r.mu.Unlock()
		go func() {
			_ = r.native.Visibility(e.id, "scope:"+token, false)
			_, _ = r.native.CallRunning(e.ctx, e.id, "runtime.release", jsonSnapshot(map[string]string{"token": token}))
		}()
	}
}
func (r *SessionRuntime) SetLocale(locale string) {
	if !canonicalLocale(locale) {
		locale = "en"
	}
	if r.script != nil {
		r.script.SetLocale(locale)
	}
	r.mu.Lock()
	if r.locale != locale {
		r.localeRevision++
	}
	r.locale = locale
	revision := r.localeRevision
	entries := make([]*nativeSessionEntry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}
	r.mu.Unlock()
	for _, e := range entries {
		go func(e *nativeSessionEntry) {
			_, _ = r.native.CallRunning(e.ctx, e.id, "runtime.locale", jsonSnapshot(map[string]any{"locale": locale, "localeRevision": revision}))
		}(e)
	}
}
