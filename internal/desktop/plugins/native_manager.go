package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	hp "github.com/hashicorp/go-plugin"
	sdk "ssh-connect/packages/plugin-go"
)

type NativeHost interface {
	sdk.Handler
	Close() error
}
type NativeOptions struct {
	LogRoot     string
	Registry    *Registry
	Host        func(context.Context, string, Manifest) (NativeHost, error)
	Changed     func()
	Failed      func(string, uint64, string)
	IdleTimeout time.Duration
}
type nativeSlot struct {
	command    *exec.Cmd
	lease      *os.File
	op         sync.Mutex
	client     *hp.Client
	rpc        *sdk.Client
	host       NativeHost
	generation uint64
	cancel     context.CancelFunc
	consumers  map[string]bool
	requests   int
	idle       *time.Timer
}

// NativeManager is the sole owner of native processes. Slots survive process
// replacement: stop/start/update always serialize on the same per-plugin lock.
type NativeManager struct {
	workspace sync.RWMutex
	mu        sync.Mutex
	slots     map[string]*nativeSlot
	options   NativeOptions
	closed    bool
}

func NewNativeManager(o NativeOptions) *NativeManager {
	if o.IdleTimeout == 0 {
		o.IdleTimeout = 30 * time.Second
	}
	return &NativeManager{slots: map[string]*nativeSlot{}, options: o}
}
func (m *NativeManager) slot(id string) (*nativeSlot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("plugin manager is closed")
	}
	s := m.slots[id]
	if s == nil {
		s = &nativeSlot{consumers: map[string]bool{}}
		m.slots[id] = s
	}
	return s, nil
}
func (m *NativeManager) start(s *nativeSlot, id string) error {
	p, _, g, err := m.options.Registry.Describe(id)
	if err != nil {
		return err
	}
	if !m.options.Registry.Available(id) {
		return errors.New("plugin unavailable")
	}
	if p.Manifest.Runtime != "native" || p.Manifest.Backend.Platform != runtime.GOOS+"-"+runtime.GOARCH {
		return errors.New("native plugin platform mismatch")
	}
	if s.rpc != nil && s.generation == g {
		if s.client.Exited() {
			m.stop(s)
			m.options.Registry.SetRuntimeState(id, g, "failed", "Plugin process exited")
			m.failed(id, s.generation, "Plugin process exited")
			if m.options.Changed != nil {
				m.options.Changed()
			}
			return errors.New("plugin process exited")
		}
		return nil
	}
	m.stop(s)
	path, err := m.options.Registry.Executable(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	host, err := m.options.Host(ctx, id, p.Manifest)
	if err != nil {
		cancel()
		return err
	}
	cmd := exec.Command(path)
	reader, writer, err := os.Pipe()
	if err != nil {
		cancel()
		host.Close()
		return err
	}
	defer reader.Close()
	cmd.Stdin = reader
	attach := prepareNativeProcess(cmd)
	cmd.Env = []string{"ATERM_PLUGIN_LEASE=1"}
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	if runtime.GOOS == "windows" {
		configureWorkerCommand(cmd)
	}
	var diagnostics io.Writer = io.Discard
	if m.options.LogRoot != "" {
		log, err := newNativeLog(m.options.LogRoot, id)
		if err != nil {
			writer.Close()
			cancel()
			host.Close()
			return err
		}
		diagnostics = log
	}
	c := hp.NewClient(&hp.ClientConfig{HandshakeConfig: sdk.Handshake, Plugins: map[string]hp.Plugin{sdk.Name: &sdk.Endpoint{}}, AllowedProtocols: []hp.Protocol{hp.ProtocolGRPC}, StartTimeout: 10 * time.Second, Logger: hclog.New(&hclog.LoggerOptions{Name: id, Level: hclog.Info, Output: diagnostics}), Stderr: diagnostics, GRPCBrokerMultiplex: true, SkipHostEnv: true, AutoMTLS: true, RunnerFunc: nativeProcessRunner(cmd, reader, attach)})
	protocol, err := c.Client()
	if err == nil {
		var raw interface{}
		raw, err = protocol.Dispense(sdk.Name)
		if err == nil {
			s.rpc = raw.(*sdk.Client)
			initCtx, done := context.WithTimeout(ctx, 10*time.Second)
			err = s.rpc.Initialize(initCtx, host)
			done()
		}
	}
	if err != nil {
		cancel()
		host.Close()
		c.Kill()
		writer.Close()
		terminateNativeTree(cmd)
		s.rpc = nil
		m.options.Registry.SetRuntimeState(id, g, "failed", err.Error())
		m.failed(id, g, err.Error())
		if m.options.Changed != nil {
			m.options.Changed()
		}
		return err
	}
	s.client, s.host, s.cancel, s.generation = c, host, cancel, g
	s.command = cmd
	s.lease = writer
	m.options.Registry.SetRuntimeState(id, g, "active", "")
	if m.options.Changed != nil {
		m.options.Changed()
	}
	go m.watch(s, id, c, ctx)
	return nil
}
func (m *NativeManager) watch(s *nativeSlot, id string, c *hp.Client, ctx context.Context) {
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.op.Lock()
			if s.client != c {
				s.op.Unlock()
				return
			}
			if c.Exited() {
				m.stop(s)
				m.options.Registry.SetRuntimeState(id, s.generation, "failed", "Plugin process exited")
				m.failed(id, s.generation, "Plugin process exited")
				s.op.Unlock()
				if m.options.Changed != nil {
					m.options.Changed()
				}
				return
			}
			s.op.Unlock()
		}
	}
}
func (m *NativeManager) stop(s *nativeSlot) {
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.host != nil {
		s.host.Close()
	}
	if s.rpc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.rpc.Call(ctx, "runtime.shutdown", nil, nil)
		cancel()
	}
	if s.lease != nil {
		s.lease.Close()
		s.lease = nil
	}
	if s.client != nil {
		s.client.Kill()
	}
	if s.command != nil {
		terminateNativeTree(s.command)
		s.command = nil
	}
	s.client = nil
	s.rpc = nil
	s.host = nil
	s.cancel = nil
}
func (m *NativeManager) Call(ctx context.Context, id, command string, args json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.workspace.RLock()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	s, err := m.slot(id)
	if err != nil {
		m.workspace.RUnlock()
		return nil, err
	}
	s.op.Lock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		s.op.Unlock()
		m.workspace.RUnlock()
		return nil, errors.New("plugin manager is closed")
	}
	if err = ctx.Err(); err == nil {
		err = m.start(s, id)
	}
	if err != nil {
		s.op.Unlock()
		m.workspace.RUnlock()
		return nil, err
	}
	s.requests++
	if s.idle != nil {
		s.idle.Stop()
	}
	rpc := s.rpc
	s.op.Unlock()
	m.workspace.RUnlock()
	var result json.RawMessage
	err = rpc.Call(ctx, command, args, &result)
	s.op.Lock()
	s.requests--
	if rpc != s.rpc {
		err = errors.New("plugin instance changed during request")
	}
	m.schedule(s, id)
	s.op.Unlock()
	return result, err
}
func (m *NativeManager) schedule(s *nativeSlot, id string) {
	if s.idle != nil {
		s.idle.Stop()
	}
	if s.rpc == nil {
		return
	}
	s.idle = time.AfterFunc(m.options.IdleTimeout, func() {
		s.op.Lock()
		defer s.op.Unlock()
		if s.rpc == nil {
			return
		}
		if s.client.Exited() {
			m.stop(s)
			m.options.Registry.SetRuntimeState(id, s.generation, "failed", "Plugin process exited")
			m.failed(id, s.generation, "Plugin process exited")
			if m.options.Changed != nil {
				m.options.Changed()
			}
			return
		}
		if s.requests > 0 || len(s.consumers) > 0 {
			m.schedule(s, id)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var state struct {
			Tasks int `json:"tasks"`
		}
		err := s.rpc.Call(ctx, "runtime.status", nil, &state)
		if err != nil || state.Tasks < 0 {
			m.stop(s)
			m.options.Registry.SetRuntimeState(id, s.generation, "failed", "Plugin status request failed")
			m.failed(id, s.generation, "Plugin status request failed")
			if m.options.Changed != nil {
				m.options.Changed()
			}
		} else if state.Tasks == 0 {
			m.stop(s)
			m.options.Registry.SetRuntimeState(id, s.generation, "inactive", "")
			if m.options.Changed != nil {
				m.options.Changed()
			}
		} else {
			m.schedule(s, id)
		}
	})
}
func (m *NativeManager) Visibility(id, consumer string, visible bool) error {
	if !consumerID.MatchString(consumer) {
		return errors.New("invalid plugin consumer")
	}
	s, err := m.slot(id)
	if err != nil {
		return err
	}
	s.op.Lock()
	defer s.op.Unlock()
	if visible {
		s.consumers[consumer] = true
	} else {
		delete(s.consumers, consumer)
	}
	m.schedule(s, id)
	return nil
}

// Change drains the old version before committing registry mutations. New
// acquisitions cannot start until the complete transaction has returned.
func (m *NativeManager) Change(id string, change func() error) error {
	s, err := m.slot(id)
	if err != nil {
		return err
	}
	s.op.Lock()
	defer s.op.Unlock()
	m.stop(s)
	err = change()
	if _, _, _, lookup := m.options.Registry.Describe(id); lookup != nil {
		s.consumers = map[string]bool{}
	}
	return err
}
func (m *NativeManager) Reset() {
	m.mu.Lock()
	slots := make([]*nativeSlot, 0, len(m.slots))
	for _, s := range m.slots {
		slots = append(slots, s)
	}
	m.mu.Unlock()
	for _, s := range slots {
		s.op.Lock()
		m.stop(s)
		s.consumers = map[string]bool{}
		s.op.Unlock()
	}
}
func (m *NativeManager) Close() { m.mu.Lock(); m.closed = true; m.mu.Unlock(); m.Reset() }

// Pause prevents new acquisitions until a workspace replacement commits. It
// cancels existing RPC/resource owners before the caller imports any new data.
func (m *NativeManager) Pause() func() {
	m.workspace.Lock()
	m.Reset()
	var once sync.Once
	return func() { once.Do(m.workspace.Unlock) }
}

func (m *NativeManager) CallRunning(ctx context.Context, id, method string, args json.RawMessage) (json.RawMessage, error) {
	s, err := m.slot(id)
	if err != nil {
		return nil, err
	}
	s.op.Lock()
	rpc := s.rpc
	s.op.Unlock()
	if rpc == nil {
		return nil, errors.New("plugin is not running")
	}
	var out json.RawMessage
	err = rpc.Call(ctx, method, args, &out)
	return out, err
}

func (m *NativeManager) failed(id string, generation uint64, reason string) {
	if m.options.Failed != nil {
		go m.options.Failed(id, generation, reason)
	}
}
