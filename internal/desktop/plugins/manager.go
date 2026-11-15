package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Session struct {
	ID                 string
	Kind               string
	Label              string
	Running            bool
	ExecutionSupported bool
	Target             GrantTarget
}
type PermissionRequest struct {
	Plugin  Package
	Session Session
	Grant   Grant
}
type Runtime interface {
	Start(context.Context, string, Package, func(string)) (string, error)
	Stop(string)
	Refresh(string, string, any) error
	Command(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error)
}
type CapabilityScope struct {
	PluginID   string
	Manifest   Manifest
	Owner      uint
	Session    Session
	GrantID    string
	Context    context.Context
	Authorized func() bool
	RuntimeID  string
}
type CapabilityExecutor interface {
	Request(context.Context, string, json.RawMessage, CapabilityScope) (any, error)
	Release(pluginID string, owner uint, sessionID string)
	ReleaseGrant(pluginID, grantID string)
}
type ManagerOptions struct {
	Context        context.Context
	Registry       *Registry
	Runtime        Runtime
	Storage        *Storage
	Views          *ViewHost
	ResolveSession func(uint, string) (Session, error)
	Authorize      func(context.Context, PermissionRequest) bool
	SendStatus     func(uint, Status)
	Changed        func()
	Capabilities   CapabilityExecutor
	RefreshTimeout time.Duration
}
type demandKey struct{ pluginID, sessionID string }
type demand struct {
	owner     uint
	consumers map[string]bool
}
type managedRuntime struct {
	instanceID string
	generation uint64
	cancel     context.CancelFunc
}
type pluginJob struct {
	key        demandKey
	owner      uint
	token      string
	grantID    string
	generation uint64
	runtimeID  string
	ctx        context.Context
	cancel     context.CancelFunc
	busy       bool
	commands   int
	failures   int
	status     *Status
	timer      *time.Timer
	watchdog   *time.Timer
}
type Manager struct {
	mu       sync.Mutex
	options  ManagerOptions
	ctx      context.Context
	cancel   context.CancelFunc
	wake     chan struct{}
	done     chan struct{}
	demands  map[demandKey]*demand
	jobs     map[demandKey]*pluginJob
	tokens   map[string]*pluginJob
	denied   map[demandKey]bool
	runtimes map[string]*managedRuntime
	closed   bool
	epoch    uint64
}

var consumerID = regexp.MustCompile(`^[a-zA-Z0-9:._-]{1,150}$`)

func NewManager(options ManagerOptions) *Manager {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.RefreshTimeout == 0 {
		options.RefreshTimeout = 8 * time.Second
	}
	ctx, cancel := context.WithCancel(options.Context)
	m := &Manager{options: options, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), done: make(chan struct{}), demands: map[demandKey]*demand{}, jobs: map[demandKey]*pluginJob{}, tokens: map[string]*pluginJob{}, denied: map[demandKey]bool{}, runtimes: map[string]*managedRuntime{}}
	go m.loop()
	return m
}
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) notify() {
	if m.options.Changed != nil {
		m.options.Changed()
	}
}
func (m *Manager) send(owner uint, status Status) {
	if m.options.SendStatus != nil {
		m.options.SendStatus(owner, clone(status))
	}
}
func (m *Manager) Visibility(owner uint, pluginID, sessionID, consumer string, visible, passive bool) error {
	if _, _, _, err := m.options.Registry.Package(pluginID); err != nil {
		return err
	}
	if !consumerID.MatchString(consumer) {
		return errors.New("invalid session visibility")
	}
	key := demandKey{pluginID, sessionID}
	if visible {
		if _, err := m.options.ResolveSession(owner, sessionID); err != nil {
			return err
		}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("plugin manager is closed")
	}
	d := m.demands[key]
	job := m.jobs[key]
	if (d != nil && d.owner != owner) || (job != nil && job.owner != owner) {
		m.mu.Unlock()
		return errors.New("session not found")
	}
	var status *Status
	if visible {
		if d == nil {
			d = &demand{owner: owner, consumers: map[string]bool{}}
			m.demands[key] = d
		}
		if len(d.consumers) >= 32 {
			if _, ok := d.consumers[consumer]; !ok {
				m.mu.Unlock()
				return errors.New("too many plugin consumers")
			}
		}
		d.consumers[consumer] = passive
		if job != nil && job.status != nil {
			copy := clone(*job.status)
			status = &copy
		} else if m.denied[key] {
			copy := emptyStatus(key, "permission-required")
			status = &copy
		}
	} else if d != nil {
		wasPassive, existed := d.consumers[consumer]
		delete(d.consumers, consumer)
		if len(d.consumers) == 0 {
			delete(m.demands, key)
			m.cancelJobLocked(key)
		} else if existed && !wasPassive && allPassive(d) {
			m.cancelJobLocked(key)
		}
	}
	m.mu.Unlock()
	if status != nil {
		m.send(owner, *status)
	}
	m.signal()
	return nil
}
func allPassive(d *demand) bool {
	for _, passive := range d.consumers {
		if !passive {
			return false
		}
	}
	return true
}
func emptyStatus(key demandKey, state string) Status {
	return Status{PluginID: key.pluginID, SessionID: key.sessionID, State: state, Items: []StatusItem{}}
}
func (m *Manager) loop() {
	defer close(m.done)
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
			m.reconcile()
		}
	}
}
func (m *Manager) resolve(owner uint, key demandKey, manifest Manifest) (Session, Grant, bool) {
	session, err := m.options.ResolveSession(owner, key.sessionID)
	if err != nil || !session.Running {
		return session, Grant{}, false
	}
	target := session.Target
	if manifest.Execution != nil {
		if !session.ExecutionSupported {
			return session, Grant{}, false
		}
		matched := false
		for _, kind := range manifest.Execution.Targets {
			if kind == session.Kind {
				matched = true
			}
		}
		if !matched {
			return session, Grant{}, false
		}
	} else {
		target = GrantTarget{ID: session.ID, HostName: session.ID}
	}
	label := session.Label
	if target.User != "" {
		label += " (" + target.User + "@" + target.HostName
		if target.Port != 0 {
			label += ":" + formatPort(target.Port)
		}
		label += ")"
	}
	return session, Grant{ID: GrantID(target, manifest), Label: label}, true
}
func hasGrant(config Config, id string) bool {
	for _, grant := range config.Grants {
		if grant.ID == id {
			return true
		}
	}
	return false
}
func (m *Manager) reconcile() {
	m.mu.Lock()
	keys := make([]demandKey, 0, len(m.demands))
	for key := range m.demands {
		keys = append(keys, key)
	}
	m.mu.Unlock()
	for _, key := range keys {
		if m.ctx.Err() != nil {
			return
		}
		plugin, config, generation, err := m.options.Registry.Package(key.pluginID)
		if err != nil || !m.available(key.pluginID) {
			continue
		}
		m.mu.Lock()
		d := m.demands[key]
		skip := d == nil || m.jobs[key] != nil || m.denied[key]
		var owner uint
		if d != nil {
			owner = d.owner
		}
		m.mu.Unlock()
		if skip {
			continue
		}
		session, grant, supported := m.resolve(owner, key, plugin.Manifest)
		if !supported {
			if plugin.Manifest.Execution != nil {
				status := emptyStatus(key, "unsupported")
				status.Data = json.RawMessage(`{"message":"Execution is unavailable for this session. Open a saved SSH connection or a local terminal."}`)
				m.send(owner, status)
			}
			continue
		}
		if len(plugin.Manifest.Permissions) > 0 && !hasGrant(config, grant.ID) {
			m.mu.Lock()
			d = m.demands[key]
			passive := d == nil || allPassive(d)
			m.mu.Unlock()
			if passive {
				m.send(owner, emptyStatus(key, "permission-required"))
				continue
			}
			epoch := m.options.Registry.Revision()
			approved := m.options.Authorize != nil && m.options.Authorize(m.ctx, PermissionRequest{Plugin: plugin, Session: session, Grant: grant})
			_, currentGrant, current := m.resolve(owner, key, plugin.Manifest)
			m.mu.Lock()
			d = m.demands[key]
			stillVisible := d != nil && d.owner == owner
			m.mu.Unlock()
			if !stillVisible || !current || currentGrant.ID != grant.ID || epoch != m.options.Registry.Revision() {
				continue
			}
			if !approved {
				m.mu.Lock()
				m.denied[key] = true
				m.mu.Unlock()
				m.send(owner, emptyStatus(key, "permission-required"))
				continue
			}
			if err = m.options.Registry.Grant(key.pluginID, grant, generation); err != nil {
				continue
			}
			config.Grants = append(config.Grants, grant)
			m.notify()
		}
		m.mu.Lock()
		record := m.runtimes[key.pluginID]
		if record == nil {
			startCtx, cancel := context.WithCancel(m.ctx)
			record = &managedRuntime{generation: generation, cancel: cancel}
			m.runtimes[key.pluginID] = record
			m.mu.Unlock()
			instanceID, startErr := m.options.Runtime.Start(startCtx, key.pluginID, plugin, func(reason string) { m.fail(key.pluginID, record, reason) })
			m.mu.Lock()
			valid := m.runtimes[key.pluginID] == record
			if valid && startErr == nil {
				record.instanceID = instanceID
			}
			m.mu.Unlock()
			if !valid {
				if instanceID != "" {
					m.options.Runtime.Stop(instanceID)
				}
				continue
			}
			if startErr != nil {
				m.fail(key.pluginID, record, "Plugin activation failed")
				continue
			}
			m.options.Registry.SetRuntimeState(key.pluginID, generation, "active", "")
			m.notify()
		} else {
			m.mu.Unlock()
		}
		_, grant, current := m.resolve(owner, key, plugin.Manifest)
		if !current {
			continue
		}
		m.mu.Lock()
		_, stillVisible := m.demands[key]
		if !stillVisible || m.jobs[key] != nil || m.runtimes[key.pluginID] != record || record.instanceID == "" || !m.available(key.pluginID) {
			m.mu.Unlock()
			continue
		}
		jobCtx, cancel := context.WithCancel(m.ctx)
		job := &pluginJob{key: key, owner: owner, token: uuid.NewString(), grantID: grant.ID, generation: generation, runtimeID: record.instanceID, ctx: jobCtx, cancel: cancel}
		m.jobs[key] = job
		m.tokens[job.token] = job
		m.mu.Unlock()
		m.poll(job)
	}
}
func (m *Manager) cancelJobLocked(key demandKey) {
	job := m.jobs[key]
	if job == nil {
		return
	}
	delete(m.jobs, key)
	delete(m.tokens, job.token)
	job.cancel()
	if job.timer != nil {
		job.timer.Stop()
	}
	if job.watchdog != nil {
		job.watchdog.Stop()
	}
}
func (m *Manager) validJob(job *pluginJob) (Package, Config, Session, bool) {
	plugin, config, generation, err := m.options.Registry.Package(job.key.pluginID)
	if err != nil || generation != job.generation || !m.available(job.key.pluginID) {
		return plugin, config, Session{}, false
	}
	session, grant, ok := m.resolve(job.owner, job.key, plugin.Manifest)
	if !ok || grant.ID != job.grantID || (len(plugin.Manifest.Permissions) > 0 && !hasGrant(config, grant.ID)) {
		return plugin, config, session, false
	}
	m.mu.Lock()
	valid := m.jobs[job.key] == job && !m.closed
	m.mu.Unlock()
	return plugin, config, session, valid
}
func (m *Manager) poll(job *pluginJob) {
	_, config, session, valid := m.validJob(job)
	if !valid {
		m.mu.Lock()
		if m.jobs[job.key] == job {
			m.cancelJobLocked(job.key)
		}
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	if m.jobs[job.key] != job || job.busy || job.commands > 0 {
		m.mu.Unlock()
		return
	}
	job.busy = true
	first := job.status == nil
	if first {
		status := emptyStatus(job.key, "loading")
		job.status = &status
	}
	record := m.runtimes[job.key.pluginID]
	job.watchdog = time.AfterFunc(m.options.RefreshTimeout, func() { m.fail(job.key.pluginID, record, "Plugin refresh timed out") })
	m.mu.Unlock()
	if first {
		m.send(job.owner, emptyStatus(job.key, "loading"))
	}
	if err := m.options.Runtime.Refresh(job.runtimeID, job.token, map[string]any{"interval": config.Interval, "sessionId": job.key.sessionID, "sessionLabel": session.Label, "hostKey": job.grantID}); err != nil {
		m.fail(job.key.pluginID, record, "Plugin unavailable")
	}
}
func (m *Manager) fail(id string, record *managedRuntime, reason string) {
	m.mu.Lock()
	if record == nil || m.runtimes[id] != record {
		m.mu.Unlock()
		return
	}
	delete(m.runtimes, id)
	record.cancel()
	for key := range m.jobs {
		if key.pluginID == id {
			m.cancelJobLocked(key)
		}
	}
	instanceID := record.instanceID
	m.mu.Unlock()
	if instanceID != "" {
		m.options.Runtime.Stop(instanceID)
	}
	m.options.Registry.SetRuntimeState(id, record.generation, "failed", reason)
	if m.options.Views != nil {
		m.options.Views.Release(id, nil, "")
	}
	if m.options.Capabilities != nil {
		m.options.Capabilities.Release(id, 0, "")
	}
	m.notify()
}
func (m *Manager) Changed(id string) { m.stopPlugin(id, true, nil); m.signal() }
func (m *Manager) stopPlugin(id string, clearDenied bool, deny map[demandKey]bool) {
	if m.options.Views != nil {
		m.options.Views.Release(id, nil, "")
	}
	m.mu.Lock()
	record := m.runtimes[id]
	delete(m.runtimes, id)
	for key := range m.jobs {
		if key.pluginID == id {
			m.cancelJobLocked(key)
		}
	}
	if clearDenied {
		for key := range m.denied {
			if key.pluginID == id {
				delete(m.denied, key)
			}
		}
	}
	for key := range deny {
		if m.demands[key] != nil {
			m.denied[key] = true
		}
	}
	if record != nil {
		record.cancel()
	}
	m.mu.Unlock()
	if record != nil && record.instanceID != "" {
		m.options.Runtime.Stop(record.instanceID)
	}
	if _, _, generation, err := m.options.Registry.Package(id); err == nil {
		m.options.Registry.SetRuntimeState(id, generation, "inactive", "")
	}
	if m.options.Capabilities != nil {
		m.options.Capabilities.Release(id, 0, "")
	}
}
func (m *Manager) Authorize(owner uint, id, sessionID string) error {
	if !m.available(id) {
		return errors.New("plugin unavailable")
	}
	if _, err := m.options.ResolveSession(owner, sessionID); err != nil {
		return err
	}
	m.mu.Lock()
	key := demandKey{id, sessionID}
	if d := m.demands[key]; d != nil && d.owner != owner {
		m.mu.Unlock()
		return errors.New("session not found")
	}
	delete(m.denied, key)
	m.mu.Unlock()
	m.signal()
	return nil
}
func (m *Manager) Refresh(owner uint, id, sessionID string) error {
	m.mu.Lock()
	job := m.jobs[demandKey{id, sessionID}]
	if job != nil && job.owner == owner && job.timer != nil {
		job.timer.Stop()
	}
	m.mu.Unlock()
	if job != nil && job.owner == owner {
		m.poll(job)
	}
	return nil
}
func (m *Manager) SessionChanged(owner uint, sessionID string, closed bool) {
	if closed && m.options.Views != nil {
		m.options.Views.Release("", &owner, sessionID)
	}
	m.mu.Lock()
	jobs := []*pluginJob{}
	for key, job := range m.jobs {
		if job.owner == owner && key.sessionID == sessionID {
			jobs = append(jobs, job)
		}
	}
	if closed {
		for key, d := range m.demands {
			if d.owner == owner && key.sessionID == sessionID {
				delete(m.demands, key)
				delete(m.denied, key)
			}
		}
	}
	m.mu.Unlock()
	for _, job := range jobs {
		_, _, _, valid := m.validJob(job)
		if closed || !valid {
			m.mu.Lock()
			if m.jobs[job.key] == job {
				m.cancelJobLocked(job.key)
			}
			m.mu.Unlock()
		}
	}
	session, sessionErr := m.options.ResolveSession(owner, sessionID)
	if m.options.Capabilities != nil && (closed || sessionErr != nil || !session.Running) {
		m.options.Capabilities.Release("", owner, sessionID)
	}
	m.signal()
}
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.cancel()
	if m.options.Views != nil {
		m.options.Views.Shutdown()
	}
	ids := make([]string, 0, len(m.runtimes))
	for id := range m.runtimes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Changed(id)
	}
	<-m.done
}

func formatPort(port int) string { return strconv.Itoa(port) }

func (m *Manager) Reset() {
	if m.options.Views != nil {
		m.options.Views.Release("", nil, "")
	}
	m.mu.Lock()
	m.epoch++
	ids := make([]string, 0, len(m.runtimes))
	for id := range m.runtimes {
		ids = append(ids, id)
	}
	for key := range m.jobs {
		m.cancelJobLocked(key)
	}
	m.demands = map[demandKey]*demand{}
	m.denied = map[demandKey]bool{}
	m.mu.Unlock()
	for _, id := range ids {
		m.Changed(id)
	}
}

func (m *Manager) available(id string) bool {
	return m.ctx.Err() == nil && m.options.Registry.Available(id)
}
