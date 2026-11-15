package plugins

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type StreamEvent struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Done      bool   `json:"done"`
	ExitCode  **int  `json:"exitCode,omitempty"`
	Error     string `json:"error,omitempty"`
}
type streamEnd struct {
	code *int
	err  string
}
type utf8Decoder struct{ pending []byte }

func (d *utf8Decoder) write(data []byte) string {
	if len(d.pending) > 0 {
		data = append(append([]byte{}, d.pending...), data...)
		d.pending = nil
	}
	var text strings.Builder
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			d.pending = append(d.pending[:0], data...)
			break
		}
		r, n := utf8.DecodeRune(data)
		text.WriteRune(r)
		data = data[n:]
	}
	return text.String()
}
func (d *utf8Decoder) end() string {
	if len(d.pending) == 0 {
		return ""
	}
	d.pending = nil
	return "\ufffd"
}

// StreamOutput permits one unacknowledged batch. Producer bytes continue into a
// fixed-size tail buffer; a stalled consumer cannot grow the IPC queue or block
// the child process. delivery orders final cancellation after any in-flight send.
type StreamOutput struct {
	mu                              sync.Mutex
	delivery                        sync.Mutex
	id                              string
	limit                           int
	buffer                          *ProcessOutput
	out, errOut                     utf8Decoder
	dirty, pending, sentEnd, closed bool
	batch                           uint64
	ended                           *streamEnd
	timer, watchdog                 *time.Timer
	ackTimeout                      time.Duration
	emit                            func(StreamEvent) error
	dispose                         func()
	interval                        func() time.Duration
}

func NewStreamOutput(id string, limit int, emit func(StreamEvent) error, dispose func(), interval func() time.Duration) *StreamOutput {
	if interval == nil {
		interval = func() time.Duration { return 100 * time.Millisecond }
	}
	return &StreamOutput{id: id, limit: limit, buffer: NewProcessOutput(limit, true), emit: emit, dispose: dispose, interval: interval, ackTimeout: 5 * time.Second}
}
func (o *StreamOutput) appendLocked(text string) {
	if text == "" {
		return
	}
	_ = o.buffer.Push([]byte(text), false)
	o.dirty = true
}
func (o *StreamOutput) scheduleLocked() {
	if o.timer != nil {
		return
	}
	delay := o.interval()
	o.timer = time.AfterFunc(delay, func() { o.mu.Lock(); o.timer = nil; o.mu.Unlock(); o.flush() })
}
func (o *StreamOutput) Push(data []byte, stderr bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || o.ended != nil {
		return
	}
	if stderr {
		o.appendLocked(o.errOut.write(data))
	} else {
		o.appendLocked(o.out.write(data))
	}
	o.scheduleLocked()
}
func (o *StreamOutput) Finish(code *int, message string) {
	o.mu.Lock()
	if o.closed || o.ended != nil {
		o.mu.Unlock()
		return
	}
	o.appendLocked(o.out.end())
	o.appendLocked(o.errOut.end())
	o.ended = &streamEnd{code: code, err: message}
	o.mu.Unlock()
	o.flush()
}
func (o *StreamOutput) Acknowledge() {
	o.mu.Lock()
	if o.closed || !o.pending {
		o.mu.Unlock()
		return
	}
	if o.watchdog != nil {
		o.watchdog.Stop()
		o.watchdog = nil
	}
	o.pending = false
	dispose := false
	if o.sentEnd {
		dispose = o.closeLocked()
	} else {
		o.scheduleLocked()
	}
	o.mu.Unlock()
	if dispose && o.dispose != nil {
		o.dispose()
	}
}
func (o *StreamOutput) flush() {
	o.delivery.Lock()
	o.mu.Lock()
	if o.closed || o.pending || (!o.dirty && o.ended == nil) {
		o.mu.Unlock()
		o.delivery.Unlock()
		return
	}
	output := o.buffer.Finish()
	o.buffer = NewProcessOutput(o.limit, true)
	o.dirty = false
	o.pending = true
	o.sentEnd = o.ended != nil
	event := StreamEvent{ID: o.id, Text: output.Stdout, Truncated: output.Truncated != nil && *output.Truncated, Done: o.sentEnd}
	if o.ended != nil {
		code := o.ended.code
		event.ExitCode = &code
		event.Error = o.ended.err
	}
	o.batch++
	batch := o.batch
	o.watchdog = time.AfterFunc(o.ackTimeout, func() { o.cancel("Stream consumer timed out", &batch) })
	o.mu.Unlock()
	err := o.emit(event)
	dispose := false
	if err != nil {
		o.mu.Lock()
		dispose = o.closeLocked()
		o.mu.Unlock()
	}
	o.delivery.Unlock()
	if dispose && o.dispose != nil {
		o.dispose()
	}
}
func (o *StreamOutput) closeLocked() bool {
	if o.closed {
		return false
	}
	o.closed = true
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
	if o.watchdog != nil {
		o.watchdog.Stop()
		o.watchdog = nil
	}
	o.buffer = nil
	o.out.pending = nil
	o.errOut.pending = nil
	return true
}
func (o *StreamOutput) Cancel(message string) { o.cancel(message, nil) }
func (o *StreamOutput) cancel(message string, expectedBatch *uint64) {
	o.delivery.Lock()
	o.mu.Lock()
	if o.closed || (expectedBatch != nil && (!o.pending || o.batch != *expectedBatch)) {
		o.mu.Unlock()
		o.delivery.Unlock()
		return
	}
	notify := !o.sentEnd
	o.sentEnd = true
	dispose := o.closeLocked()
	o.mu.Unlock()
	if notify {
		_ = o.emit(StreamEvent{ID: o.id, Done: true, Error: message})
	}
	o.delivery.Unlock()
	if dispose && o.dispose != nil {
		o.dispose()
	}
}

var errStreamUnauthorized = errors.New("stream permission expired")
