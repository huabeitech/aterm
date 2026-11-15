package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (m *Manager) Request(ctx context.Context, id, method string, input json.RawMessage) (any, error) {
	var params struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
		Token string          `json:"token"`
		State string          `json:"state"`
		Items json.RawMessage `json:"items"`
		Data  json.RawMessage `json:"data"`
		Views json.RawMessage `json:"views"`
	}
	if err := json.Unmarshal(input, &params); err != nil || string(input) == "null" {
		return nil, errors.New("invalid plugin request")
	}
	if method == "storage.get" || method == "storage.set" {
		if !m.available(id) || m.options.Storage == nil {
			return nil, errors.New("plugin disabled")
		}
		if method == "storage.get" {
			return m.options.Storage.Get(id, params.Key)
		}
		return nil, m.options.Storage.Set(id, params.Key, params.Value)
	}
	m.mu.Lock()
	job := m.tokens[params.Token]
	m.mu.Unlock()
	if job == nil || job.key.pluginID != id {
		return nil, errors.New("permission denied")
	}
	plugin, config, session, valid := m.validJob(job)
	if !valid {
		return nil, errors.New("permission denied")
	}
	switch method {
	case "process.execute", "terminal.create", "process.stream.start", "process.stream.stop", "process.stream.ack":
		if m.options.Capabilities == nil {
			return map[string]any{"error": "Execution environment unavailable"}, nil
		}
		// Request context and the grant's lifetime both bound the capability. Streams
		// use the job context inside their executor so RPC completion cannot kill them.
		scope := CapabilityScope{PluginID: id, Manifest: plugin.Manifest, Owner: job.owner, Session: session, GrantID: job.grantID, Context: job.ctx, RuntimeID: job.runtimeID, Authorized: func() bool { _, _, _, valid := m.validJob(job); return valid }}
		result, err := m.options.Capabilities.Request(ctx, method, input, scope)
		if err != nil {
			return map[string]any{"error": truncateText(err.Error(), 2000)}, nil
		}
		return result, nil
	case "status.publish":
		if params.State != "ready" && params.State != "delayed" && params.State != "unsupported" && params.State != "unavailable" {
			return nil, errors.New("invalid contribution")
		}
		items, err := ValidateStatusItems(params.Items, plugin.Manifest)
		if err != nil {
			return nil, err
		}
		var data json.RawMessage
		if len(params.Data) > 0 {
			data, err = JSONSnapshot(params.Data, 196608)
			if err != nil {
				return nil, err
			}
		}
		var views map[string]ViewAvailability
		if len(params.Views) > 0 {
			views, err = ValidateViewAvailability(params.Views, plugin.Manifest)
			if err != nil {
				return nil, err
			}
		}
		m.mu.Lock()
		if m.jobs[job.key] != job || job.ctx.Err() != nil {
			m.mu.Unlock()
			return nil, errors.New("permission denied")
		}
		completing := job.busy
		job.busy = false
		if job.watchdog != nil {
			job.watchdog.Stop()
			job.watchdog = nil
		}
		if params.State == "delayed" || params.State == "unavailable" {
			job.failures++
		} else {
			job.failures = 0
		}
		status := emptyStatus(job.key, params.State)
		status.Items = items
		status.Data = data
		status.Views = views
		if job.status != nil {
			status.UpdatedAt = job.status.UpdatedAt
			if len(params.Views) == 0 {
				status.Views = job.status.Views
			}
			if params.State == "delayed" {
				status.Items = job.status.Items
				status.Data = job.status.Data
			}
		}
		if params.State == "ready" {
			now := time.Now().UnixMilli()
			status.UpdatedAt = &now
		}
		job.status = &status
		if completing && params.State != "unsupported" {
			interval := config.Interval
			if job.failures >= 3 && interval < 30 {
				interval = 30
			}
			m.scheduleLocked(job, time.Duration(interval)*time.Second)
		}
		m.mu.Unlock()
		m.send(job.owner, status)
		return nil, nil
	default:
		return nil, errors.New("invalid plugin request")
	}
}
func (m *Manager) scheduleLocked(job *pluginJob, delay time.Duration) {
	if job.timer != nil {
		job.timer.Stop()
	}
	job.timer = time.AfterFunc(delay, func() { m.poll(job) })
}
func (m *Manager) ExecuteCommand(ctx context.Context, owner uint, id, sessionID, commandID string, args json.RawMessage) (any, error) {
	plugin, _, _, err := m.options.Registry.Package(id)
	if err != nil || !m.available(id) {
		return nil, errors.New("plugin unavailable")
	}
	if _, err = m.options.ResolveSession(owner, sessionID); err != nil {
		return nil, err
	}
	var command *Command
	for _, declared := range plugin.Manifest.Contributes.Commands {
		if declared.ID == commandID {
			copy := declared
			command = &copy
			break
		}
	}
	if command == nil {
		return nil, errors.New("command not declared")
	}
	if len(args) == 0 {
		args = json.RawMessage("null")
	}
	args, err = JSONSnapshot(args, 4096)
	if err != nil {
		return nil, err
	}
	if command.Action != nil {
		return map[string]any{"action": command.Action}, nil
	}
	m.mu.Lock()
	job := m.jobs[demandKey{id, sessionID}]
	m.mu.Unlock()
	if job == nil || job.owner != owner {
		return nil, errors.New("permission denied")
	}
	_, config, _, valid := m.validJob(job)
	if !valid {
		return nil, errors.New("permission denied")
	}
	m.mu.Lock()
	if m.jobs[job.key] != job || job.commands >= 16 {
		m.mu.Unlock()
		return nil, errors.New("too many pending commands or plugin unavailable")
	}
	job.commands++
	if job.timer != nil {
		job.timer.Stop()
	}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		job.commands--
		if job.commands == 0 && m.jobs[job.key] == job && job.ctx.Err() == nil {
			m.scheduleLocked(job, time.Duration(config.Interval)*time.Second)
		}
		m.mu.Unlock()
	}()
	commandCtx, cancel := context.WithCancel(job.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	defer cancel()
	value, err := m.options.Runtime.Command(commandCtx, job.runtimeID, job.token, command.ID, args)
	if err != nil {
		return nil, err
	}
	if _, _, _, valid = m.validJob(job); !valid {
		return nil, errors.New("execution cancelled")
	}
	normalized, err := JSONSnapshot(value, 196608)
	if err != nil {
		return nil, err
	}
	return map[string]any{"data": normalized}, nil
}

// Revocation is not a new implicit permission request. Keep matching demands
// denied until the user explicitly retries authorization.
func (m *Manager) Revoked(id, grantID string) {
	plugin, _, _, err := m.options.Registry.Package(id)
	if err != nil {
		return
	}
	m.mu.Lock()
	demands := map[demandKey]uint{}
	for key, d := range m.demands {
		if key.pluginID == id {
			demands[key] = d.owner
		}
	}
	m.mu.Unlock()
	denied := map[demandKey]bool{}
	for key, owner := range demands {
		_, grant, valid := m.resolve(owner, key, plugin.Manifest)
		if !valid || grant.ID != grantID {
			continue
		}
		denied[key] = true
	}
	m.mu.Lock()
	for key := range denied {
		if m.demands[key] != nil {
			m.denied[key] = true
		}
		m.cancelJobLocked(key)
	}
	m.mu.Unlock()
	if m.options.Capabilities != nil {
		m.options.Capabilities.ReleaseGrant(id, grantID)
	}
	for key := range denied {
		if m.options.Views != nil {
			owner := demands[key]
			m.options.Views.Release(id, &owner, key.sessionID)
		}
		m.send(demands[key], emptyStatus(key, "permission-required"))
	}

	m.signal()
}

func (m *Manager) OpenView(owner uint, id, sessionID, viewID string) (ViewDocument, error) {
	m.mu.Lock()
	epoch := m.epoch
	m.mu.Unlock()
	plugin, _, generation, err := m.options.Registry.Package(id)
	if err != nil || !m.available(id) {
		return ViewDocument{}, errors.New("plugin unavailable")
	}
	if _, err = m.options.ResolveSession(owner, sessionID); err != nil {
		return ViewDocument{}, err
	}
	if m.options.Views == nil {
		return ViewDocument{}, errors.New("plugin views are unavailable")
	}
	for _, view := range plugin.Manifest.Contributes.Views {
		if view.ID == viewID {
			html := plugin.Views[view.Entry]
			if html == "" {
				break
			}
			document, err := m.options.Views.Open(owner, id, sessionID, html, plugin.Localizations)
			if err != nil {
				return ViewDocument{}, err
			}
			_, _, currentGeneration, currentErr := m.options.Registry.Package(id)
			m.mu.Lock()
			currentEpoch := m.epoch
			m.mu.Unlock()
			if currentErr != nil || currentGeneration != generation || currentEpoch != epoch || !m.available(id) {
				_ = m.options.Views.Close(owner, document.InstanceID)
				return ViewDocument{}, errors.New("plugin changed while opening the view")
			}
			return document, nil
		}
	}
	return ViewDocument{}, errors.New("view not declared")
}
func (m *Manager) CloseView(owner uint, instanceID string) error {
	if m.options.Views == nil {
		return nil
	}
	return m.options.Views.Close(owner, instanceID)
}
