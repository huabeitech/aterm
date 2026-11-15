package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"sync"
	"time"
)

type ExecutionOptions struct {
	Audit          *Audit
	Run            func(context.Context, Session, ProcessRequest, ExecutionPolicy, ProcessData) (ProcessResult, error)
	OpenTerminal   func(context.Context, uint, Session, ProcessRequest, ExecutionPolicy) (string, error)
	CloseTerminal  func(uint, string)
	TerminalOpened func(uint, string)
	StreamEvent    func(string, StreamEvent) error
}
type executionOperation struct {
	scope  CapabilityScope
	cancel context.CancelFunc
}
type terminalLease struct {
	scope      CapabilityScope
	terminalID string
}
type Executor struct {
	mu         sync.Mutex
	options    ExecutionOptions
	active     map[string]int
	total      int
	operations map[string]executionOperation
	terminals  map[string]terminalLease
	streams    map[streamKey]*executionStream
}

func NewExecutor(options ExecutionOptions) *Executor {
	return &Executor{options: options, active: map[string]int{}, operations: map[string]executionOperation{}, terminals: map[string]terminalLease{}, streams: map[streamKey]*executionStream{}}
}
func (e *Executor) Activity(id string) []Activity { return e.options.Audit.List(id) }
func (e *Executor) Request(ctx context.Context, method string, input json.RawMessage, scope CapabilityScope) (any, error) {
	switch method {
	case "process.stream.start":
		return e.startStream(input, scope)
	case "process.stream.stop", "process.stream.ack":
		return e.controlStream(input, scope, method == "process.stream.stop")
	}

	if method != "process.execute" && method != "terminal.create" {
		return nil, errors.New("unknown execution capability")
	}
	var params struct {
		Request json.RawMessage `json:"request"`
	}
	if json.Unmarshal(input, &params) != nil {
		return nil, errors.New("invalid capability request")
	}
	request, err := ValidateProcessRequest(params.Request, method == "terminal.create", false)
	if err != nil {
		return nil, err
	}
	record := func(outcome string) error {
		return e.options.Audit.Add(Activity{PluginID: scope.PluginID, Target: scope.GrantID, Capability: method, Executable: request.Executable, ArgumentCount: len(request.Args), Outcome: outcome})
	}
	if scope.Context.Err() != nil || !scope.Authorized() {
		_ = record("denied")
		return nil, errors.New("execution permission denied")
	}
	if err = AuthorizeExecution(scope.Manifest, method, scope.Session.Kind, request); err != nil {
		_ = record("denied")
		return nil, err
	}
	operationCtx, cancel := context.WithTimeout(scope.Context, time.Duration(request.TimeoutMS)*time.Millisecond)
	stopCaller := context.AfterFunc(ctx, cancel)
	defer stopCaller()
	defer cancel()
	operationID := uuid.NewString()
	e.mu.Lock()
	if e.active[scope.PluginID] >= 4 || e.total >= 16 {
		e.mu.Unlock()
		_ = record("denied")
		return nil, errors.New("execution concurrency limit exceeded")
	}
	if err = record("started"); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	e.active[scope.PluginID]++
	e.total++
	e.operations[operationID] = executionOperation{scope: scope, cancel: cancel}
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.active[scope.PluginID]--
		e.total--
		delete(e.operations, operationID)
		e.mu.Unlock()
	}()
	var result any
	var created string
	if method == "terminal.create" {
		if e.options.OpenTerminal == nil {
			err = errors.New("interactive terminal transport is unavailable")
		} else {
			created, err = e.options.OpenTerminal(operationCtx, scope.Owner, scope.Session, request, *scope.Manifest.Execution)
			if err == nil {
				result = map[string]string{"terminalSessionId": created}
			}
		}
	} else {
		if e.options.Run == nil {
			err = errors.New("execution transport is unavailable")
		} else {
			result, err = e.options.Run(operationCtx, scope.Session, request, *scope.Manifest.Execution, nil)
		}
	}
	cancelled := scope.Context.Err() != nil || !scope.Authorized()
	if cancelled {
		err = errors.New("execution cancelled")
	} else if operationCtx.Err() != nil {
		err = errors.New("execution timed out")
	}
	if err != nil {
		if created != "" && e.options.CloseTerminal != nil {
			e.options.CloseTerminal(scope.Owner, created)
		}
		outcome := "failed"
		if cancelled {
			outcome = "cancelled"
		}
		auditErr := record(outcome)
		return nil, errors.Join(err, auditErr)
	}
	if method == "terminal.create" {
		e.mu.Lock()
		e.terminals[created] = terminalLease{scope: scope, terminalID: created}
		e.mu.Unlock()
		if !scope.Authorized() {
			e.Release(scope.PluginID, scope.Owner, scope.Session.ID)
			return nil, errors.New("execution cancelled")
		}
		if err = record("succeeded"); err != nil {
			e.mu.Lock()
			delete(e.terminals, created)
			e.mu.Unlock()
			if e.options.CloseTerminal != nil {
				e.options.CloseTerminal(scope.Owner, created)
			}
			return nil, err
		}
		if e.options.TerminalOpened != nil {
			e.options.TerminalOpened(scope.Owner, created)
		}
	} else {
		data, encodeErr := marshalJSON(result)
		if encodeErr != nil {
			return nil, encodeErr
		}
		if _, err = JSONSnapshot(data, 196608); err != nil {
			_ = record("failed")
			return nil, err
		}
		outcome := "failed"
		if process, ok := result.(ProcessResult); ok && process.ExitCode != nil && *process.ExitCode == 0 {
			outcome = "succeeded"
		}
		if err = record(outcome); err != nil {
			return nil, err
		}
	}
	return result, nil
}
func matchesScope(scope CapabilityScope, id string, owner uint, sessionID string) bool {
	return (id == "" || scope.PluginID == id) && (owner == 0 || scope.Owner == owner) && (sessionID == "" || scope.Session.ID == sessionID)
}
func (e *Executor) Release(id string, owner uint, sessionID string) {
	e.cancelStreams(func(scope CapabilityScope) bool { return matchesScope(scope, id, owner, sessionID) })
	e.mu.Lock()
	for _, operation := range e.operations {
		if matchesScope(operation.scope, id, owner, sessionID) {
			operation.cancel()
		}
	}
	leases := []terminalLease{}
	for terminalID, lease := range e.terminals {
		if matchesScope(lease.scope, id, owner, sessionID) {
			delete(e.terminals, terminalID)
			leases = append(leases, lease)
		}
	}
	e.mu.Unlock()
	for _, lease := range leases {
		if e.options.CloseTerminal != nil {
			e.options.CloseTerminal(lease.scope.Owner, lease.terminalID)
		}
	}
}
func (e *Executor) ForgetTerminal(owner uint, id string) {
	e.mu.Lock()
	if lease, ok := e.terminals[id]; ok && lease.scope.Owner == owner {
		delete(e.terminals, id)
	}
	e.mu.Unlock()
}

func (e *Executor) ReleaseGrant(id, grantID string) {
	e.cancelStreams(func(scope CapabilityScope) bool { return scope.PluginID == id && scope.GrantID == grantID })
	e.mu.Lock()
	for _, operation := range e.operations {
		if operation.scope.PluginID == id && operation.scope.GrantID == grantID {
			operation.cancel()
		}
	}
	leases := []terminalLease{}
	for terminalID, lease := range e.terminals {
		if lease.scope.PluginID == id && lease.scope.GrantID == grantID {
			delete(e.terminals, terminalID)
			leases = append(leases, lease)
		}
	}
	e.mu.Unlock()
	for _, lease := range leases {
		if e.options.CloseTerminal != nil {
			e.options.CloseTerminal(lease.scope.Owner, lease.terminalID)
		}
	}
}

func (e *Executor) cancelStreams(matches func(CapabilityScope) bool) {
	e.mu.Lock()
	streams := []*executionStream{}
	for _, stream := range e.streams {
		if matches(stream.scope) {
			streams = append(streams, stream)
		}
	}
	e.mu.Unlock()
	for _, stream := range streams {
		stream.cancel()
		stream.output.Cancel("Execution cancelled")
	}
}
