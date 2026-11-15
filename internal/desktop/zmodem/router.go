package zmodem

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type RouterOptions struct {
	Output   func([]byte)
	Write    func([]byte) error
	Offer    func(context.Context, FileOffer) (Download, error)
	Select   func(context.Context) ([]Upload, error)
	Progress func(Progress)
	Timeout  time.Duration
}

// Router owns the canonical separation between terminal text and protocol bytes.
// Transfer frames never enter scrollback or the renderer acknowledgement window.
// Native file capabilities are supplied separately for each transfer direction.
type transferSession interface {
	Feed([]byte) ([]byte, error)
	Done() bool
	Cancel() error
	Stop(error) error
}
type activeTransfer struct{ transferSession }

type Router struct {
	mu         sync.Mutex
	progressMu sync.Mutex
	ctx        context.Context
	options    RouterOptions
	pending    []byte
	active     atomic.Pointer[activeTransfer]
	draining   atomic.Pointer[abortDrain]
	timer      *time.Timer
	closed     bool
	started    time.Time
	deadline   atomic.Int64
	last       Progress
	lastTime   time.Time
}

func NewRouter(ctx context.Context, options RouterOptions) *Router {
	if options.Timeout <= 0 {
		options.Timeout = 120 * time.Second
	}
	return &Router{ctx: ctx, options: options, started: time.Now()}
}
func (r *Router) Busy() bool { return r.active.Load() != nil }
func (r *Router) Cancel() error {
	if transfer := r.active.Load(); transfer != nil {
		drain := newAbortDrain(Hex, false)
		installed := r.draining.CompareAndSwap(nil, drain)
		if err := transfer.Cancel(); err == nil && installed {
			r.draining.CompareAndSwap(drain, nil)
		}
		r.active.CompareAndSwap(transfer, nil)
	}
	return nil
}

func (r *Router) progress(value Progress) {
	r.progressMu.Lock()
	defer r.progressMu.Unlock()
	now := time.Now()
	if value.State == r.last.State && value.Name == r.last.Name && value.State == "running" && now.Sub(r.lastTime) < 120*time.Millisecond {
		return
	}
	r.last = value
	r.lastTime = now
	if r.options.Progress != nil {
		r.options.Progress(value)
	}
}
func (r *Router) output(data []byte) {
	if len(data) > 0 && r.options.Output != nil {
		r.options.Output(data)
	}
}
func (r *Router) start(kind FrameType) {
	if r.timer != nil {
		r.timer.Stop()
	}
	transfer := &activeTransfer{}
	if kind == ZRINIT {
		transfer.transferSession = NewSender(r.ctx, SendOptions{Write: r.options.Write, Select: r.options.Select, Progress: r.progress, Abort: r.abort})
	} else {
		transfer.transferSession = NewReceiver(r.ctx, ReceiveOptions{Write: r.options.Write, Offer: r.options.Offer, Progress: r.progress, Abort: r.abort})
	}
	r.active.Store(transfer)
	r.deadline.Store(time.Since(r.started).Nanoseconds() + r.options.Timeout.Nanoseconds())
	r.timer = time.AfterFunc(r.options.Timeout, func() {
		if active := r.active.Load(); active == transfer && time.Since(r.started).Nanoseconds() >= r.deadline.Load() {
			_ = transfer.Stop(errors.New("ZMODEM transfer timed out"))
			r.active.CompareAndSwap(transfer, nil)
		}
	})
}
func (r *Router) abort(format Encoding, peer bool) { r.draining.Store(newAbortDrain(format, peer)) }

func (r *Router) Consume(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if transfer := r.active.Load(); transfer != nil {
		r.deadline.Store(time.Since(r.started).Nanoseconds() + r.options.Timeout.Nanoseconds())
		r.timer.Reset(r.options.Timeout)
		tail, _ := transfer.Feed(data)
		if transfer.Done() {
			r.active.CompareAndSwap(transfer, nil)
			r.timer.Stop()
			r.timer = nil
			r.consumeIdle(tail)
		}
		return
	}
	r.consumeIdle(data)
}
func (r *Router) consumeIdle(data []byte) {
	if drain := r.draining.Load(); drain != nil {
		tail, done := drain.consume(data)
		if !done {
			return
		}
		r.draining.CompareAndSwap(drain, nil)
		data = tail
	}
	r.pending = append(r.pending, data...)
	for len(r.pending) > 0 {
		index := 0
		for index < len(r.pending) && r.pending[index] != '*' {
			index++
		}
		if index > 0 {
			r.output(r.pending[:index])
			r.pending = r.pending[index:]
			continue
		}
		header, size, err := DecodeHeader(r.pending)
		if errors.Is(err, ErrIncomplete) {
			if len(r.pending) > 256 {
				r.output(r.pending[:len(r.pending)-3])
				r.pending = r.pending[len(r.pending)-3:]
				continue
			}
			return
		}
		if err != nil {
			r.output(r.pending[:1])
			r.pending = r.pending[1:]
			continue
		}
		if header.Type != ZRQINIT && header.Type != ZRINIT {
			r.output(r.pending[:size])
			r.pending = r.pending[size:]
			continue
		}
		r.start(header.Type)
		transfer := r.active.Load()
		input := r.pending
		r.pending = nil
		tail, _ := transfer.Feed(input)
		if transfer.Done() {
			r.active.CompareAndSwap(transfer, nil)
			r.timer.Stop()
			r.timer = nil
			r.pending = append(r.pending, tail...)
			continue
		}
		return
	}
	r.pending = nil
}
func (r *Router) Close() {
	_ = r.Cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	if r.timer != nil {
		r.timer.Stop()
	}
	r.active.Store(nil)
	r.draining.Store(nil)
	r.output(r.pending)
	r.pending = nil
}
