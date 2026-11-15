package openssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

func SubsystemArguments(socket string) []string {
	return []string{"-F", "/dev/null", "-T", "-S", socket, "-o", "ControlMaster=no", "-o", "BatchMode=yes", "-o", "ProxyCommand=false", "-o", "ClearAllForwardings=yes", "-s", "localhost", "sftp"}
}

type diagnosticTail struct {
	mu   sync.Mutex
	data string
}

func (t *diagnosticTail) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data += string(data)
	if len(t.data) > 4096 {
		t.data = t.data[len(t.data)-4096:]
	}
	return len(data), nil
}
func (t *diagnosticTail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(t.data)
}

type fileProcess struct {
	once   sync.Once
	input  io.WriteCloser
	output io.ReadCloser
	cancel context.CancelFunc
}

func (p *fileProcess) Close() error {
	p.once.Do(func() {
		p.cancel()
		p.input.Close()
		p.output.Close()
	})
	return nil
}

// OpenFiles uses only the already authenticated master. ProxyCommand=false
// prevents OpenSSH from falling back to a fresh network connection.
func (i *Integration) OpenFiles(ctx context.Context, id string) (*sftp.Client, io.Closer, error) {
	socket, lifetime, err := i.Connection(id)
	if err != nil {
		return nil, nil, err
	}
	processContext, cancel := context.WithCancel(lifetime)
	command := exec.CommandContext(processContext, i.binary, SubsystemArguments(socket)...)
	diagnostic := &diagnosticTail{}
	command.Stderr = diagnostic
	input, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, nil, err
	}
	if err = command.Start(); err != nil {
		input.Close()
		output.Close()
		cancel()
		return nil, nil, err
	}
	process := &fileProcess{input: input, output: output, cancel: cancel}
	go func() { _ = command.Wait(); process.Close() }()
	type result struct {
		client *sftp.Client
		err    error
	}
	ready := make(chan result, 1)
	go func() { client, err := sftp.NewClientPipe(output, input); ready <- result{client, err} }()
	bounded, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	select {
	case result := <-ready:
		if result.err != nil {
			process.Close()
			if detail := diagnostic.String(); detail != "" {
				return nil, nil, fmt.Errorf("OpenSSH SFTP connection failed: %s: %w", detail, result.err)
			}
			return nil, nil, fmt.Errorf("OpenSSH SFTP connection failed: %w", result.err)
		}
		if _, _, err = i.Connection(id); err != nil {
			result.client.Close()
			process.Close()
			return nil, nil, err
		}
		return result.client, process, nil
	case <-bounded.Done():
		process.Close()
		go func() {
			result := <-ready
			if result.client != nil {
				result.client.Close()
			}
		}()
		return nil, nil, bounded.Err()
	case <-lifetime.Done():
		process.Close()
		go func() {
			result := <-ready
			if result.client != nil {
				result.client.Close()
			}
		}()
		return nil, nil, errors.New("SSH connection is no longer active")
	}
}
