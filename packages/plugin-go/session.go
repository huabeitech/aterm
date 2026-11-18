package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// SessionInfo is a host-issued execution lease, not a credential or terminal.
type SessionInfo struct {
	Token        string `json:"token"`
	SessionID    string `json:"sessionId"`
	HostKey      string `json:"hostKey"`
	SessionLabel string `json:"sessionLabel"`
	Interval     int    `json:"interval"`
}
type ProcessRequest struct {
	Executable     string             `json:"executable"`
	Args           []string           `json:"args"`
	Env            map[string]*string `json:"env,omitempty"`
	TimeoutMS      int                `json:"timeoutMs,omitempty"`
	Output         string             `json:"output,omitempty"`
	MaxOutputBytes int                `json:"maxOutputBytes,omitempty"`
	Title          string             `json:"title,omitempty"`
	Checks         []ProcessCheck     `json:"checks,omitempty"`
}
type ProcessCheck struct {
	Args   []string `json:"args"`
	Stdout string   `json:"stdout"`
}
type ProcessResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  *int   `json:"exitCode"`
	Truncated bool   `json:"truncated"`
}
type StreamEvent struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Done      bool   `json:"done"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	Error     string `json:"error,omitempty"`
}
type SessionProgram interface {
	Refresh(context.Context, SessionInfo) error
	Command(context.Context, SessionInfo, string, json.RawMessage) (any, error)
	Stream(context.Context, StreamEvent) error
	Release(string)
	Tasks() int
	Close()
}
type SessionRuntime struct {
	Host           Host
	Program        SessionProgram
	mu             sync.RWMutex
	sessions       map[string]*sessionLease
	released       map[string]bool
	releaseOrder   []string
	locale         string
	localeRevision uint64
	bundle         struct {
		DefaultLocale string                       `json:"defaultLocale"`
		Messages      map[string]map[string]string `json:"messages"`
	}
}

type sessionLease struct {
	info   SessionInfo
	ctx    context.Context
	cancel context.CancelFunc
	op     sync.Mutex
}

// bind cancels in-flight work as soon as the host releases its execution lease.
func (s *sessionLease) bind(ctx context.Context) (context.Context, func()) {
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	if s.ctx.Err() != nil {
		cancel()
	}
	return child, func() { stop(); cancel() }
}
func ServeSession(factory func(*SessionRuntime) SessionProgram) {
	Serve(func(host Host) Handler {
		r := &SessionRuntime{Host: host, sessions: map[string]*sessionLease{}, released: map[string]bool{}, locale: "en"}
		r.Program = factory(r)
		return r
	})
}
func (r *SessionRuntime) Open(context.Context, string, json.RawMessage) (io.ReadWriteCloser, error) {
	return nil, errors.New("unsupported extension stream")
}
func (r *SessionRuntime) Call(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "runtime.activate", "runtime.locale":
		r.mu.Lock()
		var p struct {
			Locale         string          `json:"locale"`
			LocaleRevision uint64          `json:"localeRevision"`
			Localizations  json.RawMessage `json:"localizations"`
		}
		err := json.Unmarshal(raw, &p)
		if err == nil {
			if p.LocaleRevision >= r.localeRevision {
				r.locale = p.Locale
				r.localeRevision = p.LocaleRevision
			}
			if len(p.Localizations) > 0 {
				err = json.Unmarshal(p.Localizations, &r.bundle)
			}
		}
		sessions := make([]SessionInfo, 0, len(r.sessions))
		for _, s := range r.sessions {
			sessions = append(sessions, s.info)
		}
		r.mu.Unlock()
		if err == nil {
			if localized, ok := r.Program.(interface {
				LocaleChanged(context.Context, []SessionInfo)
			}); ok {
				localized.LocaleChanged(ctx, sessions)
			}
		}
		return JSON(nil), err
	case "runtime.refresh":
		var p struct {
			Token          string          `json:"token"`
			Context        SessionInfo     `json:"context"`
			Locale         string          `json:"locale"`
			LocaleRevision uint64          `json:"localeRevision"`
			Localizations  json.RawMessage `json:"localizations"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		p.Context.Token = p.Token
		r.mu.Lock()
		if p.Locale != "" {
			if p.LocaleRevision >= r.localeRevision {
				r.locale = p.Locale
				r.localeRevision = p.LocaleRevision
			}
		}
		if len(p.Localizations) > 0 {
			if err := json.Unmarshal(p.Localizations, &r.bundle); err != nil {
				r.mu.Unlock()
				return nil, err
			}
		}
		if p.Token == "" {
			r.mu.Unlock()
			return nil, errors.New("missing execution lease")
		}
		if r.released[p.Token] {
			r.mu.Unlock()
			return JSON(nil), nil
		}
		lease := r.sessions[p.Token]
		if lease == nil {
			lifetime, cancel := context.WithCancel(context.Background())
			lease = &sessionLease{info: p.Context, ctx: lifetime, cancel: cancel}
			r.sessions[p.Token] = lease
		}
		r.mu.Unlock()
		lease.op.Lock()
		defer lease.op.Unlock()
		ctx, done := lease.bind(ctx)
		defer done()
		if ctx.Err() != nil {
			return JSON(nil), nil
		}
		err := r.Program.Refresh(ctx, p.Context)
		if err != nil {
			_ = r.Publish(ctx, p.Token, map[string]any{"state": "delayed", "items": []any{}})
		}
		return JSON(nil), nil
	case "runtime.command":
		var p struct {
			Token, Command string
			Args           json.RawMessage
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		r.mu.RLock()
		s, ok := r.sessions[p.Token]
		r.mu.RUnlock()
		if !ok {
			return nil, errors.New("execution session unavailable")
		}
		s.op.Lock()
		defer s.op.Unlock()
		ctx, done := s.bind(ctx)
		defer done()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		out, err := r.Program.Command(ctx, s.info, p.Command, p.Args)
		return JSON(out), err
	case "runtime.stream":
		var e StreamEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		return JSON(nil), r.Program.Stream(ctx, e)
	case "runtime.release":
		var p struct{ Token string }
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		r.mu.Lock()
		lease := r.sessions[p.Token]
		delete(r.sessions, p.Token)
		if !r.released[p.Token] {
			r.released[p.Token] = true
			r.releaseOrder = append(r.releaseOrder, p.Token)
		}
		if len(r.releaseOrder) > 1024 {
			delete(r.released, r.releaseOrder[0])
			r.releaseOrder = r.releaseOrder[1:]
		}
		r.mu.Unlock()
		if lease != nil {
			lease.cancel()
			lease.op.Lock()
			defer lease.op.Unlock()
		}
		r.Program.Release(p.Token)
		return JSON(nil), nil
	case "runtime.status":
		return JSON(map[string]int{"tasks": r.Program.Tasks()}), nil
	case "runtime.shutdown":
		r.mu.Lock()
		for _, lease := range r.sessions {
			lease.cancel()
		}
		r.mu.Unlock()
		r.Program.Close()
		return JSON(nil), nil
	default:
		return nil, errors.New("unknown native session method")
	}
}
func (r *SessionRuntime) Capability(ctx context.Context, method string, args any, result any) error {
	var raw json.RawMessage
	if err := r.Host.Call(ctx, method, args, &raw); err != nil {
		return err
	}
	var failure struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &failure)
	if failure.Error != "" {
		return errors.New(failure.Error)
	}
	if result != nil {
		return json.Unmarshal(raw, result)
	}
	return nil
}
func (r *SessionRuntime) Run(ctx context.Context, token string, p ProcessRequest) (ProcessResult, error) {
	var out ProcessResult
	err := r.Capability(ctx, "process.execute", map[string]any{"token": token, "request": p}, &out)
	return out, err
}
func (r *SessionRuntime) Publish(ctx context.Context, token string, p map[string]any) error {
	p["token"] = token
	return r.Capability(ctx, "status.publish", p, nil)
}
func (r *SessionRuntime) T(key string, args map[string]any) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, locale := range []string{r.locale, strings.Split(r.locale, "-")[0], r.bundle.DefaultLocale, "en"} {
		if text, ok := r.bundle.Messages[locale][key]; ok {
			for k, v := range args {
				text = strings.ReplaceAll(text, "{{"+k+"}}", fmt.Sprint(v))
			}
			return text
		}
	}
	return key
}
