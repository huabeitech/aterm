package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"sync"
	"time"
)

type RuntimeControl struct {
	Kind          string         `json:"kind"`
	InstanceID    string         `json:"instanceId"`
	Source        string         `json:"source,omitempty"`
	Localizations *Localizations `json:"localizations,omitempty"`
	Locale        string         `json:"locale,omitempty"`
	Context       any            `json:"context,omitempty"`
	Event         any            `json:"event,omitempty"`
}
type RuntimeRequest func(context.Context, string, string, json.RawMessage) (any, error)
type commandResult struct {
	value json.RawMessage
	err   error
}
type runtimeEntry struct {
	id       string
	pluginID string
	ready    bool
	started  chan error
	pending  map[string]chan commandResult
	second   int64
	calls    int
	active   int
	fault    func(string)
}

// RuntimeHost brokers a sandbox transport. It never evaluates plugin code or
// trusts a plugin-supplied identity: an opaque instance binds every RPC.
type RuntimeHost struct {
	mu                sync.Mutex
	entries           map[string]*runtimeEntry
	send              func(RuntimeControl)
	request           RuntimeRequest
	closed            bool
	locale            string
	activationTimeout time.Duration
	commandTimeout    time.Duration
}

func NewRuntimeHost(send func(RuntimeControl), request RuntimeRequest) *RuntimeHost {
	return &RuntimeHost{entries: map[string]*runtimeEntry{}, send: send, request: request, locale: "en", activationTimeout: 5 * time.Second, commandTimeout: 45 * time.Second}
}
func (r *RuntimeHost) Start(ctx context.Context, pluginID string, plugin Package, fault func(string)) (string, error) {
	instanceID := uuid.NewString()
	entry := &runtimeEntry{id: instanceID, pluginID: pluginID, started: make(chan error, 1), pending: map[string]chan commandResult{}, fault: fault}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", errors.New("plugin runtime is closed")
	}
	r.entries[instanceID] = entry
	locale := r.locale
	r.mu.Unlock()
	r.send(RuntimeControl{Kind: "start", InstanceID: instanceID, Source: plugin.Source, Localizations: clone(plugin.Localizations), Locale: locale})
	r.mu.Lock()
	current := r.entries[instanceID] == entry
	if current && locale != r.locale {
		r.send(RuntimeControl{Kind: "locale", InstanceID: instanceID, Locale: r.locale})
	}
	r.mu.Unlock()
	if !current {
		r.send(RuntimeControl{Kind: "stop", InstanceID: instanceID})
		return "", errors.New("plugin stopped during activation")
	}
	timer := time.NewTimer(r.activationTimeout)
	defer timer.Stop()
	var err error
	select {
	case err = <-entry.started:
	case <-ctx.Done():
		err = ctx.Err()
	case <-timer.C:
		err = errors.New("plugin activation timed out")
	}
	if err != nil {
		r.Stop(instanceID)
		return "", err
	}
	r.mu.Lock()
	current = r.entries[instanceID] == entry
	r.mu.Unlock()
	if !current {
		return "", errors.New("plugin stopped during activation")
	}
	return instanceID, nil
}
func (r *RuntimeHost) Handle(ctx context.Context, instanceID, method string, params json.RawMessage) (any, error) {
	if len(params) > 262144 || method == "" {
		return nil, errors.New("plugin request limit")
	}
	r.mu.Lock()
	entry := r.entries[instanceID]
	if entry == nil {
		r.mu.Unlock()
		return nil, errors.New("plugin unavailable")
	}
	if method == "runtime.fault" {
		fault := entry.fault
		r.mu.Unlock()
		r.Stop(instanceID)
		if fault != nil {
			fault("Plugin worker failed or stopped responding")
		}
		return nil, nil
	}
	second := time.Now().Unix()
	if entry.second != second {
		entry.second = second
		entry.calls = 0
	}
	entry.calls++
	if entry.calls > 50 || entry.active >= 16 {
		r.mu.Unlock()
		return nil, errors.New("plugin request limit")
	}
	if method == "runtime.ready" {
		if !entry.ready {
			entry.ready = true
			entry.started <- nil
		}
		r.mu.Unlock()
		return nil, nil
	}
	if !entry.ready {
		r.mu.Unlock()
		return nil, errors.New("plugin is not ready")
	}
	switch method {
	case "runtime.heartbeat":
		r.mu.Unlock()
		return nil, nil
	case "runtime.commandResult":
		var result struct {
			CallID string          `json:"callId"`
			Error  string          `json:"error"`
			Value  json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(params, &result); err != nil {
			r.mu.Unlock()
			return nil, err
		}
		pending := entry.pending[result.CallID]
		if pending == nil {
			r.mu.Unlock()
			return nil, errors.New("unknown plugin command response")
		}
		delete(entry.pending, result.CallID)
		r.mu.Unlock()
		if result.Error != "" {
			message := []rune(result.Error)
			if len(message) > 300 {
				message = message[:300]
			}
			pending <- commandResult{err: errors.New(string(message))}
			return nil, nil
		}
		if len(result.Value) == 0 {
			result.Value = json.RawMessage("null")
		}
		value, err := JSONSnapshot(result.Value, 196608)
		pending <- commandResult{value: value, err: err}
		return nil, err
	}
	entry.active++
	request := r.request
	pluginID := entry.pluginID
	r.mu.Unlock()
	defer func() { r.mu.Lock(); entry.active--; r.mu.Unlock() }()
	if request == nil {
		return nil, errors.New("plugin capability unavailable")
	}
	return request(ctx, pluginID, method, params)
}
func (r *RuntimeHost) Command(ctx context.Context, instanceID, token, command string, args json.RawMessage) (json.RawMessage, error) {
	if len(args) == 0 {
		args = json.RawMessage("null")
	}
	normalized, err := JSONSnapshot(args, 196608)
	if err != nil {
		return nil, err
	}
	callID := uuid.NewString()
	pending := make(chan commandResult, 1)
	r.mu.Lock()
	entry := r.entries[instanceID]
	if entry == nil || !entry.ready {
		r.mu.Unlock()
		return nil, errors.New("plugin unavailable")
	}
	if len(entry.pending) >= 16 {
		r.mu.Unlock()
		return nil, errors.New("too many pending plugin commands")
	}
	entry.pending[callID] = pending
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(entry.pending, callID); r.mu.Unlock() }()
	r.send(RuntimeControl{Kind: "command", InstanceID: instanceID, Context: map[string]any{"callId": callID, "token": token, "command": command, "args": normalized}})
	timer := time.NewTimer(r.commandTimeout)
	defer timer.Stop()
	select {
	case result := <-pending:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("plugin command timed out")
	}
}
func (r *RuntimeHost) Refresh(instanceID, token string, context any) error {
	data, err := marshalJSON(context)
	if err != nil {
		return err
	}
	var values map[string]any
	if err = json.Unmarshal(data, &values); err != nil || values == nil {
		return errors.New("invalid plugin refresh context")
	}
	values["token"] = token
	return r.deliver(instanceID, "refresh", values, nil)
}
func (r *RuntimeHost) StreamEvent(instanceID string, event any) error {
	return r.deliver(instanceID, "process-stream", nil, event)
}
func (r *RuntimeHost) deliver(instanceID, kind string, context, event any) error {
	r.mu.Lock()
	entry := r.entries[instanceID]
	ready := entry != nil && entry.ready
	r.mu.Unlock()
	if !ready {
		return errors.New("plugin unavailable")
	}
	r.send(RuntimeControl{Kind: kind, InstanceID: instanceID, Context: context, Event: event})
	return nil
}
func (r *RuntimeHost) Stop(instanceID string) {
	r.mu.Lock()
	entry := r.entries[instanceID]
	delete(r.entries, instanceID)
	if entry != nil {
		if !entry.ready {
			entry.started <- errors.New("plugin stopped")
		}
		for callID, pending := range entry.pending {
			pending <- commandResult{err: errors.New("plugin stopped")}
			delete(entry.pending, callID)
		}
	}
	r.mu.Unlock()
	if entry != nil {
		r.send(RuntimeControl{Kind: "stop", InstanceID: instanceID})
	}
}
func (r *RuntimeHost) SetLocale(locale string) {
	if !canonicalLocale(locale) {
		locale = "en"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locale == locale {
		return
	}
	r.locale = locale
	for id := range r.entries {
		r.send(RuntimeControl{Kind: "locale", InstanceID: id, Locale: locale})
	}
}

func (r *RuntimeHost) Reset() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.entries))
	for id := range r.entries {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.Stop(id)
	}
}
func (r *RuntimeHost) Close() { r.mu.Lock(); r.closed = true; r.mu.Unlock(); r.Reset() }
