package agentconn

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const pageantFrameLimit = 8192
const pageantQueryTimeout = 30 * time.Second

type agentQuery func(context.Context, []byte) ([]byte, error)

// queryConnection adapts request/response IPC to a real blocking stream. In
// particular x/crypto's pipelined agent client starts a reader before writing.
// net.Pipe handles partial I/O, deadlines, backpressure and close wakeups; one
// worker retains at most a single bounded request/response, without an RPC queue.
type queryConnection struct {
	net.Conn
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	readDeadline time.Time
	failure      error
	done         chan struct{}
}

func newQueryConnection(query agentQuery) *queryConnection {
	local, remote := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	connection := &queryConnection{Conn: local, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go connection.serve(remote, query)
	return connection
}
func readAgentFrame(reader io.Reader) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(prefix[:])
	if size == 0 || size > pageantFrameLimit-4 {
		return nil, errors.New("Pageant agent request exceeds its frame limit")
	}
	frame := make([]byte, 4+int(size))
	copy(frame, prefix[:])
	_, err := io.ReadFull(reader, frame[4:])
	return frame, err
}
func validAgentFrame(frame []byte) bool {
	return len(frame) >= 5 && len(frame) <= pageantFrameLimit && binary.BigEndian.Uint32(frame[:4]) == uint32(len(frame)-4)
}
func (c *queryConnection) setFailure(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure == nil {
		c.failure = err
	}
}
func (c *queryConnection) serve(remote net.Conn, query agentQuery) {
	defer close(c.done)
	defer remote.Close()
	defer c.cancel()
	for {
		frame, err := readAgentFrame(remote)
		if err != nil {
			c.setFailure(err)
			return
		}
		c.mu.Lock()
		deadline := c.readDeadline
		c.mu.Unlock()
		limit := time.Now().Add(pageantQueryTimeout)
		if !deadline.IsZero() && deadline.Before(limit) {
			limit = deadline
		}
		operation, cancel := context.WithDeadline(c.ctx, limit)
		var response []byte
		if err = operation.Err(); err == nil {
			response, err = query(operation, frame)
		}
		if err == nil {
			err = operation.Err()
		}
		cancel()
		clear(frame)
		if err != nil {
			clear(response)
			c.setFailure(err)
			return
		}
		if !validAgentFrame(response) {
			clear(response)
			c.setFailure(errors.New("invalid Pageant agent response frame"))
			return
		}
		reply := response
		for len(response) > 0 {
			count, err := remote.Write(response)
			if err != nil {
				c.setFailure(err)
				clear(reply)
				return
			}
			if count == 0 {
				c.setFailure(io.ErrShortWrite)
				clear(reply)
				return
			}
			response = response[count:]
		}
		clear(reply)
	}
}
func (c *queryConnection) Read(data []byte) (int, error) {
	count, err := c.Conn.Read(data)
	if err != nil {
		c.mu.Lock()
		failure := c.failure
		c.mu.Unlock()
		if failure != nil {
			return count, failure
		}
	}
	return count, err
}
func (c *queryConnection) Close() error {
	c.setFailure(net.ErrClosed)
	c.cancel()
	return c.Conn.Close()
}
func (c *queryConnection) SetDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.readDeadline = deadline
	c.mu.Unlock()
	return c.Conn.SetDeadline(deadline)
}
func (c *queryConnection) SetReadDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.readDeadline = deadline
	c.mu.Unlock()
	return c.Conn.SetReadDeadline(deadline)
}

type pageantConnection struct{ *queryConnection }

func IsPageant(connection net.Conn) bool { _, ok := connection.(*pageantConnection); return ok }
