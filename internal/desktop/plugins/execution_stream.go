package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

type streamKey struct{ pluginID, id string }
type executionStream struct {
	scope  CapabilityScope
	ctx    context.Context
	cancel context.CancelFunc
	output *StreamOutput
}

var streamID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func (e *Executor) startStream(input json.RawMessage, scope CapabilityScope) (any, error) {
	var params struct {
		ID      string          `json:"id"`
		Request json.RawMessage `json:"request"`
	}
	if json.Unmarshal(input, &params) != nil || !streamID.MatchString(params.ID) {
		return nil, errors.New("invalid stream ID")
	}
	request, err := ValidateProcessRequest(params.Request, false, true)
	if err != nil {
		return nil, err
	}
	key := streamKey{scope.PluginID, params.ID}
	record := func(outcome string) error {
		return e.options.Audit.Add(Activity{PluginID: scope.PluginID, Target: scope.GrantID, Capability: "process.execute", Executable: request.Executable, ArgumentCount: len(request.Args), Outcome: outcome})
	}
	if scope.Context.Err() != nil || !scope.Authorized() {
		_ = record("denied")
		return nil, errors.New("execution permission denied")
	}
	if err = AuthorizeExecution(scope.Manifest, "process.execute", scope.Session.Kind, request); err != nil {
		_ = record("denied")
		return nil, err
	}
	e.mu.Lock()
	count := 0
	for k := range e.streams {
		if k.pluginID == scope.PluginID {
			count++
		}
	}
	if e.streams[key] != nil || len(e.streams) >= 16 || count >= 4 || e.active[scope.PluginID] >= 4 || e.total >= 16 {
		e.mu.Unlock()
		_ = record("denied")
		return nil, errors.New("execution concurrency limit exceeded")
	}
	if err = record("started"); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithCancel(scope.Context)
	stream := &executionStream{scope: scope, ctx: ctx, cancel: cancel}
	stream.output = NewStreamOutput(params.ID, request.MaxOutputBytes, func(event StreamEvent) error {
		if e.options.StreamEvent == nil {
			return errors.New("plugin runtime unavailable")
		}
		if ctx.Err() != nil || !scope.Authorized() {
			_ = e.options.StreamEvent(scope.RuntimeID, StreamEvent{ID: params.ID, Done: true, Error: "Execution cancelled"})
			return errStreamUnauthorized
		}
		return e.options.StreamEvent(scope.RuntimeID, event)
	}, func() {
		e.mu.Lock()
		if e.streams[key] == stream {
			delete(e.streams, key)
		}
		e.mu.Unlock()
		cancel()
	}, func() time.Duration {
		e.mu.Lock()
		count := 0
		for k := range e.streams {
			if k.pluginID == scope.PluginID {
				count++
			}
		}
		e.mu.Unlock()
		if count < 1 {
			count = 1
		}
		return time.Duration(count) * 100 * time.Millisecond
	})
	e.streams[key] = stream
	e.active[scope.PluginID]++
	e.total++
	e.mu.Unlock()
	context.AfterFunc(ctx, func() { stream.output.Cancel("Execution cancelled") })
	go func() {
		defer func() { e.mu.Lock(); e.active[scope.PluginID]--; e.total--; e.mu.Unlock() }()
		var result ProcessResult
		var err error
		if e.options.Run == nil {
			err = errors.New("execution transport unavailable")
		} else {
			result, err = e.options.Run(ctx, scope.Session, request, *scope.Manifest.Execution, func(data []byte, stderr bool) {
				if ctx.Err() != nil || !scope.Authorized() {
					cancel()
					return
				}
				stream.output.Push(data, stderr)
			})
		}
		cancelled := ctx.Err() != nil || !scope.Authorized()
		outcome := "failed"
		if cancelled {
			outcome = "cancelled"
		} else if err == nil && result.ExitCode != nil && *result.ExitCode == 0 {
			outcome = "succeeded"
		}
		auditErr := record(outcome)
		if cancelled {
			stream.output.Cancel("Execution cancelled")
			return
		}
		if err != nil || auditErr != nil {
			stream.output.Finish(nil, truncateText(errors.Join(err, auditErr).Error(), 2000))
		} else {
			stream.output.Finish(result.ExitCode, "")
		}
	}()
	return map[string]string{"id": params.ID}, nil
}
func (e *Executor) controlStream(input json.RawMessage, scope CapabilityScope, stop bool) (any, error) {
	var params struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(input, &params) != nil || !streamID.MatchString(params.ID) {
		return nil, errors.New("invalid stream ID")
	}
	e.mu.Lock()
	stream := e.streams[streamKey{scope.PluginID, params.ID}]
	e.mu.Unlock()
	if stream == nil {
		return nil, nil
	}
	if stream.scope.Owner != scope.Owner || stream.scope.Session.ID != scope.Session.ID || stream.scope.Context != scope.Context || stream.scope.GrantID != scope.GrantID {
		return nil, errors.New("stream permission denied")
	}
	if stop {
		stream.cancel()
		stream.output.Cancel("Execution cancelled")
	} else {
		stream.output.Acknowledge()
	}
	return nil, nil
}
