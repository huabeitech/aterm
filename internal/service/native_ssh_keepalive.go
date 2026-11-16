package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const defaultServerAliveCountMax = 3

var ErrSSHKeepaliveTimeout = errors.New("ssh keepalive timeout")

type SSHKeepaliveTimeoutError struct {
	Alias    string
	Interval time.Duration
	Count    int
}

func (e *SSHKeepaliveTimeoutError) Error() string {
	return fmt.Sprintf("%s: host %q did not respond after %d keepalive checks at %s intervals", ErrSSHKeepaliveTimeout, e.Alias, e.Count, e.Interval)
}

func (e *SSHKeepaliveTimeoutError) Unwrap() error { return ErrSSHKeepaliveTimeout }

type nativeSSHKeepaliveRequester interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
}

type nativeSSHKeepaliveConfig struct {
	alias    string
	interval time.Duration
	count    int
}

func nativeSSHKeepaliveConfigForHost(host Host) nativeSSHKeepaliveConfig {
	count := host.ServerAliveCountMax
	if host.ServerAliveInterval > 0 && count <= 0 {
		count = defaultServerAliveCountMax
	}
	return nativeSSHKeepaliveConfig{
		alias:    host.Alias,
		interval: time.Duration(host.ServerAliveInterval) * time.Second,
		count:    count,
	}
}

// NativeSSHClient owns one authenticated SSH transport, its keepalive worker,
// optional forwarding-agent connection, and (for a destination reached through
// ProxyJump) the parent client.
type NativeSSHClient struct {
	client     *ssh.Client
	proxy      *NativeSSHClient
	forwarding net.Conn
	cancel     context.CancelFunc

	closeOnce sync.Once
	closeErr  error
	causeMu   sync.RWMutex
	cause     error
}

func newNativeSSHClient(client *ssh.Client, host Host, proxy *NativeSSHClient, forwarding net.Conn) *NativeSSHClient {
	ctx, cancel := context.WithCancel(context.Background())
	connection := &NativeSSHClient{client: client, proxy: proxy, cancel: cancel, forwarding: forwarding}
	config := nativeSSHKeepaliveConfigForHost(host)
	if config.interval > 0 {
		go runNativeSSHKeepalive(ctx, client, config, connection.closeWithCause)
	}
	go func() {
		_ = client.Wait()
		_ = connection.Close()
	}()
	return connection
}

func (c *NativeSSHClient) NewSession() (*ssh.Session, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	if c.forwarding != nil {
		if err := agent.RequestAgentForwarding(session); err != nil {
			session.Close()
			return nil, fmt.Errorf("request SSH agent forwarding: %w", err)
		}
	}
	return session, nil
}

func (c *NativeSSHClient) Dial(network, address string) (net.Conn, error) {
	return c.client.Dial(network, address)
}

func (c *NativeSSHClient) RawClient() *ssh.Client { return c.client }

func (c *NativeSSHClient) Cause() error {
	c.causeMu.RLock()
	defer c.causeMu.RUnlock()
	return c.cause
}

func (c *NativeSSHClient) Close() error { return c.closeWithCause(nil) }

func (c *NativeSSHClient) closeWithCause(cause error) error {
	c.closeOnce.Do(func() {
		if cause != nil {
			c.causeMu.Lock()
			c.cause = cause
			c.causeMu.Unlock()
		}
		c.cancel()
		c.closeErr = c.client.Close()
		if c.forwarding != nil {
			c.forwarding.Close()
		}
		if c.proxy != nil {
			_ = c.proxy.Close()
		}
	})
	return c.closeErr
}

type nativeSSHKeepaliveResult struct{ err error }

func runNativeSSHKeepalive(ctx context.Context, requester nativeSSHKeepaliveRequester, config nativeSSHKeepaliveConfig, fail func(error) error) {
	if config.interval <= 0 || config.count <= 0 {
		return
	}

	ticker := time.NewTicker(config.interval)
	defer ticker.Stop()

	// An initial probe lets a completely unresponsive transport reach the
	// configured count after count * interval while still keeping at most one
	// SSH global request in flight.
	result := sendNativeSSHKeepalive(requester)
	misses := 0
	for {
		select {
		case <-ctx.Done():
			return
		case response := <-result:
			if response.err == nil {
				misses = 0
			} else {
				misses++
				if misses >= config.count {
					_ = fail(newSSHKeepaliveTimeoutError(config))
					return
				}
			}
			result = nil
		case <-ticker.C:
			if result == nil {
				result = sendNativeSSHKeepalive(requester)
				continue
			}
			// Prefer a response that arrived on the interval boundary over
			// counting that same request as unanswered.
			select {
			case response := <-result:
				result = nil
				if response.err == nil {
					misses = 0
					continue
				}
				misses++
			default:
				misses++
			}
			if misses >= config.count {
				_ = fail(newSSHKeepaliveTimeoutError(config))
				return
			}
		}
	}
}

func sendNativeSSHKeepalive(requester nativeSSHKeepaliveRequester) <-chan nativeSSHKeepaliveResult {
	result := make(chan nativeSSHKeepaliveResult, 1)
	go func() {
		_, _, err := requester.SendRequest("keepalive@openssh.com", true, nil)
		result <- nativeSSHKeepaliveResult{err: err}
	}()
	return result
}

func newSSHKeepaliveTimeoutError(config nativeSSHKeepaliveConfig) error {
	return &SSHKeepaliveTimeoutError{Alias: config.alias, Interval: config.interval, Count: config.count}
}

func preferNativeSSHCause(client *NativeSSHClient, err error) error {
	if cause := client.Cause(); cause != nil {
		return cause
	}
	return err
}

// DialContext and Wait allow non-terminal capabilities to own an SSH transport.
func (c *NativeSSHClient) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return c.client.DialContext(ctx, network, address)
}

func (c *NativeSSHClient) Wait() error {
	err := c.client.Wait()
	if cause := c.Cause(); cause != nil {
		return cause
	}
	return err
}
