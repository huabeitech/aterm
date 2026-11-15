package plugins

import (
	"errors"
	"strings"
	"sync"
	"unicode/utf8"
)

type ProcessOutput struct {
	mu              sync.Mutex
	bytes, channels []byte
	cursor, length  int
	tail, truncated bool
}

func NewProcessOutput(limit int, tail bool) *ProcessOutput {
	return &ProcessOutput{bytes: make([]byte, limit), channels: make([]byte, limit), tail: tail}
}
func (o *ProcessOutput) Push(data []byte, stderr bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	limit := len(o.bytes)
	if len(data)+o.length > limit {
		if !o.tail {
			return errors.New("execution output limit exceeded")
		}
		o.truncated = true
	}
	if len(data) > limit {
		data = data[len(data)-limit:]
	}
	channel := byte(0)
	if stderr {
		channel = 1
	}
	for _, b := range data {
		o.bytes[o.cursor] = b
		o.channels[o.cursor] = channel
		o.cursor = (o.cursor + 1) % limit
	}
	o.length += len(data)
	if o.length > limit {
		o.length = limit
	}
	return nil
}
func (o *ProcessOutput) Finish() ProcessResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	out, errBytes := make([]byte, 0, o.length), make([]byte, 0, o.length)
	limit := len(o.bytes)
	start := (o.cursor - o.length + limit) % limit
	for n := 0; n < o.length; n++ {
		index := (start + n) % limit
		if o.channels[index] == 1 {
			errBytes = append(errBytes, o.bytes[index])
		} else {
			out = append(out, o.bytes[index])
		}
	}
	decode := func(data []byte) string {
		if o.truncated {
			for len(data) > 0 && data[0]&0xc0 == 0x80 {
				data = data[1:]
			}
		}
		var builder strings.Builder
		for len(data) > 0 {
			r, n := utf8.DecodeRune(data)
			builder.WriteRune(r)
			data = data[n:]
		}
		return builder.String()
	}
	result := ProcessResult{Stdout: decode(out), Stderr: decode(errBytes)}
	if o.tail {
		truncated := o.truncated
		result.Truncated = &truncated
	}
	return result
}
