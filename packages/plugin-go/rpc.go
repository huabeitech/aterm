// Package plugin supplies the versioned native aTerm extension protocol.
// Application implementation types never cross this boundary.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	hp "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var Handshake = hp.HandshakeConfig{ProtocolVersion: 1, MagicCookieKey: "ATERM_NATIVE_PLUGIN", MagicCookieValue: "1"}

const Name = "extension"

// Handler serves application-defined calls and opaque byte streams.
type Handler interface {
	Call(context.Context, string, json.RawMessage) (json.RawMessage, error)
	Open(context.Context, string, json.RawMessage) (io.ReadWriteCloser, error)
}
type Factory func(Host) Handler
type Host interface {
	Call(context.Context, string, any, any) error
	Open(context.Context, string, any) (net.Conn, error)
}
type Envelope struct {
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

func JSON(value any) json.RawMessage { b, _ := json.Marshal(value); return b }

type Endpoint struct {
	hp.NetRPCUnsupportedPlugin
	Factory Factory
}

func (p *Endpoint) GRPCServer(b *hp.GRPCBroker, s *grpc.Server) error {
	register(s, &server{broker: b, factory: p.Factory})
	return nil
}
func (p *Endpoint) GRPCClient(ctx context.Context, b *hp.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &Client{conn: c, broker: b}, nil
}

type Client struct {
	conn   *grpc.ClientConn
	broker *hp.GRPCBroker
}

func (c *Client) Initialize(ctx context.Context, host Handler) error {
	id := c.broker.NextId()
	go c.broker.AcceptAndServe(id, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		register(s, &server{handler: host})
		return s
	})
	return c.Call(ctx, "runtime.initialize", id, nil)
}
func (c *Client) Call(ctx context.Context, method string, args any, result any) error {
	var reply wrapperspb.BytesValue
	err := c.conn.Invoke(ctx, "/aterm.Extension/Call", wrapperspb.Bytes(JSON(Envelope{method, JSON(args)})), &reply)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(reply.Value, result)
	}
	return nil
}
func (c *Client) Open(ctx context.Context, method string, args any) (net.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	s, err := c.conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/aterm.Extension/Pipe")
	if err != nil {
		cancel()
		return nil, err
	}
	if err = s.SendMsg(wrapperspb.Bytes(JSON(Envelope{method, JSON(args)}))); err != nil {
		cancel()
		return nil, err
	}
	var ready wrapperspb.BytesValue
	if err = s.RecvMsg(&ready); err != nil {
		cancel()
		return nil, err
	}
	return &streamConn{wire: s, cancel: cancel}, nil
}

type server struct {
	mu         sync.Mutex
	broker     *hp.GRPCBroker
	factory    Factory
	handler    Handler
	connection *grpc.ClientConn
}
type service interface {
	call(context.Context, *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error)
	pipe(grpc.ServerStream) error
}

func (s *server) call(ctx context.Context, r *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error) {
	var e Envelope
	if len(r.Value) > 1<<20 {
		return nil, errors.New("request limit")
	}
	if err := json.Unmarshal(r.Value, &e); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if e.Method == "runtime.initialize" {
		defer s.mu.Unlock()
		if s.handler != nil || s.factory == nil {
			return nil, errors.New("already initialized")
		}
		var id uint32
		if err := json.Unmarshal(e.Args, &id); err != nil {
			return nil, err
		}
		c, err := s.broker.Dial(id)
		if err != nil {
			return nil, err
		}
		s.connection = c
		s.handler = s.factory(&Client{conn: c})
		return wrapperspb.Bytes([]byte("null")), nil
	}
	h := s.handler
	s.mu.Unlock()
	if h == nil {
		return nil, errors.New("not initialized")
	}
	out, err := h.Call(ctx, e.Method, e.Args)
	return wrapperspb.Bytes(out), err
}
func (s *server) pipe(w grpc.ServerStream) error {
	var r wrapperspb.BytesValue
	if err := w.RecvMsg(&r); err != nil {
		return err
	}
	var e Envelope
	if len(r.Value) > 8192 {
		return errors.New("stream request limit")
	}
	if err := json.Unmarshal(r.Value, &e); err != nil {
		return err
	}
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	if h == nil {
		return errors.New("not initialized")
	}
	conn, err := h.Open(w.Context(), e.Method, e.Args)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = w.SendMsg(wrapperspb.Bytes(nil)); err != nil {
		return err
	}
	remote := &streamConn{wire: w, cancel: func() { conn.Close() }}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(conn, remote)
		if err == nil {
			if c, ok := conn.(interface{ CloseWrite() error }); ok {
				err = c.CloseWrite()
			}
		}
		if err != nil {
			conn.Close()
		}
		done <- err
	}()
	_, err = io.Copy(remote, conn)
	if err != nil {
		conn.Close()
		return err
	}
	if err = remote.CloseWrite(); err != nil {
		return err
	}
	select {
	case err = <-done:
		return err
	case <-w.Context().Done():
		return w.Context().Err()
	}
}
func register(g *grpc.Server, s service) {
	g.RegisterService(&grpc.ServiceDesc{
		ServiceName: "aterm.Extension", HandlerType: (*service)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Call", Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
			r := new(wrapperspb.BytesValue)
			if err := dec(r); err != nil {
				return nil, err
			}
			if interceptor == nil {
				return srv.(service).call(ctx, r)
			}
			return interceptor(ctx, r, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/aterm.Extension/Call"}, func(ctx context.Context, r interface{}) (interface{}, error) {
				return srv.(service).call(ctx, r.(*wrapperspb.BytesValue))
			})
		}}},
		Streams: []grpc.StreamDesc{{StreamName: "Pipe", ServerStreams: true, ClientStreams: true, Handler: func(srv interface{}, stream grpc.ServerStream) error { return srv.(service).pipe(stream) }}},
	}, s)
}

type wire interface {
	SendMsg(interface{}) error
	RecvMsg(interface{}) error
}
type streamConn struct {
	wire             wire
	cancel           context.CancelFunc
	readMu, writeMu  sync.Mutex
	pending          []byte
	eof, writeClosed bool
	once             sync.Once
}

func (c *streamConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	for len(c.pending) == 0 {
		if c.eof {
			return 0, io.EOF
		}
		var r wrapperspb.BytesValue
		if err := c.wire.RecvMsg(&r); err != nil {
			return 0, err
		}
		if len(r.Value) == 0 {
			c.eof = true
			return 0, io.EOF
		}
		if len(r.Value) > 32768 {
			return 0, errors.New("stream frame limit")
		}
		c.pending = r.Value
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}
func (c *streamConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeClosed {
		return 0, io.ErrClosedPipe
	}
	written := 0
	for len(p) > 0 {
		n := len(p)
		if n > 32768 {
			n = 32768
		}
		if err := c.wire.SendMsg(wrapperspb.Bytes(append([]byte(nil), p[:n]...))); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}
func (c *streamConn) CloseWrite() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeClosed {
		return nil
	}
	c.writeClosed = true
	return c.wire.SendMsg(wrapperspb.Bytes(nil))
}
func (c *streamConn) Close() error         { c.once.Do(c.cancel); return nil }
func (c *streamConn) LocalAddr() net.Addr  { return address("plugin") }
func (c *streamConn) RemoteAddr() net.Addr { return address("host") }
func (c *streamConn) SetDeadline(time.Time) error {
	return errors.New("use stream context cancellation")
}
func (c *streamConn) SetReadDeadline(time.Time) error {
	return errors.New("use stream context cancellation")
}
func (c *streamConn) SetWriteDeadline(time.Time) error {
	return errors.New("use stream context cancellation")
}

type address string

func (a address) Network() string { return "grpc" }
func (a address) String() string  { return string(a) }

// Serve also observes an inherited lease pipe. Parent crashes close the pipe,
// including on macOS where process groups alone do not provide parent-death signals.
func Serve(factory Factory) {
	var mu sync.Mutex
	var handler Handler
	if os.Getenv("ATERM_PLUGIN_LEASE") == "1" {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			time.AfterFunc(3*time.Second, exitWithChildren)
			mu.Lock()
			h := handler
			mu.Unlock()
			if h != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, _ = h.Call(ctx, "runtime.shutdown", JSON(nil))
				cancel()
			}
			exitWithChildren()
		}()
	}
	hp.Serve(&hp.ServeConfig{HandshakeConfig: Handshake, Plugins: map[string]hp.Plugin{Name: &Endpoint{Factory: func(host Host) Handler { h := factory(host); mu.Lock(); handler = h; mu.Unlock(); return h }}}, GRPCServer: hp.DefaultGRPCServer})
}
