// Package openssh scopes shell integration and multiplexed file channels to one
// PTY. It never modifies global shell startup files or SSH configuration.
package openssh

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RemoteContext struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Host   string `json:"host"`
	User   string `json:"user"`
	Port   int    `json:"port"`
	State  string `json:"state"`
}
type Integration struct {
	mu                sync.Mutex
	notifyMu          sync.Mutex
	directory, binary string
	ctx               context.Context
	cancel            context.CancelFunc
	active            *RemoteContext
	activeContext     context.Context
	activeCancel      context.CancelFunc
	closed            bool
	started           bool
	changed           func(*RemoteContext)
}

func New(binary string) (*Integration, error) {
	root := os.TempDir()
	if runtime.GOOS == "darwin" {
		root = "/tmp"
	}
	directory, err := os.MkdirTemp(root, "at-")
	if err != nil {
		return nil, err
	}
	if err = os.Mkdir(filepath.Join(directory, "connections"), 0700); err == nil {
		err = os.WriteFile(filepath.Join(directory, "ssh"), []byte(wrapper(directory, binary)), 0700)
	}
	if err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Integration{directory: directory, binary: binary, ctx: ctx, cancel: cancel}, nil
}
func ParseConfig(text string) *RemoteContext {
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if key, value, ok := strings.Cut(line, " "); ok {
			values[key] = value
		}
	}
	port, err := strconv.Atoi(values["port"])
	if err != nil || port < 1 || port > 65535 || values["hostname"] == "" || values["user"] == "" {
		return nil
	}
	return &RemoteContext{Source: "openssh", Host: values["hostname"], User: values["user"], Port: port, State: "ready"}
}

var connectionName = regexp.MustCompile(`^c\.[a-zA-Z0-9]+$`)

func (i *Integration) Start(changed func(*RemoteContext)) {
	i.mu.Lock()
	if i.closed || i.started {
		i.mu.Unlock()
		return
	}
	i.started = true
	i.changed = changed
	i.mu.Unlock()
	go func() {
		for {
			i.update(i.discover())
			select {
			case <-i.ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}
func (i *Integration) discover() *RemoteContext {
	entries, err := os.ReadDir(filepath.Join(i.directory, "connections"))
	if err != nil {
		return nil
	}
	var next *RemoteContext
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || !connectionName.MatchString(entry.Name()) {
			continue
		}
		count++
		if count > 32 {
			break
		}
		directory := filepath.Join(i.directory, "connections", entry.Name())
		socket := filepath.Join(directory, "socket")
		info, err := os.Lstat(socket)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			continue
		}
		file, err := os.Open(filepath.Join(directory, "config"))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
		file.Close()
		if err != nil || len(data) > 1024*1024 {
			continue
		}
		config := string(data)
		endpoint := ParseConfig(config)
		if endpoint == nil || strings.Contains(config, "\nremotecommand ") || strings.Contains(config, "\nsessiontype none") {
			continue
		}
		ctx, cancel := context.WithTimeout(i.ctx, time.Second)
		command := exec.CommandContext(ctx, i.binary, "-F", "/dev/null", "-S", socket, "-O", "check", "localhost")
		err = command.Run()
		cancel()
		if err != nil {
			continue
		}
		if next != nil {
			return nil
		}
		endpoint.ID = entry.Name()
		next = endpoint
	}
	return next
}
func (i *Integration) update(next *RemoteContext) {
	i.notifyMu.Lock()
	defer i.notifyMu.Unlock()
	i.mu.Lock()
	if i.closed {
		next = nil
	}
	if (i.active == nil && next == nil) || (i.active != nil && next != nil && i.active.ID == next.ID) {
		i.mu.Unlock()
		return
	}
	if i.activeCancel != nil {
		i.activeCancel()
		i.activeCancel = nil
	}
	i.active = next
	if next != nil {
		i.activeContext, i.activeCancel = context.WithCancel(i.ctx)
	}
	changed := i.changed
	i.mu.Unlock()
	if changed != nil {
		changed(next)
	}
}
func (i *Integration) Connection(id string) (string, context.Context, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed || i.active == nil || i.active.ID != id {
		return "", nil, errors.New("SSH connection is no longer active")
	}
	return filepath.Join(i.directory, "connections", id, "socket"), i.activeContext, nil
}
func (i *Integration) Close() error {
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return nil
	}
	i.closed = true
	i.cancel()
	i.mu.Unlock()
	i.update(nil)
	entries, _ := os.ReadDir(filepath.Join(i.directory, "connections"))
	var group sync.WaitGroup
	count := 0
	for _, entry := range entries {
		if !connectionName.MatchString(entry.Name()) {
			continue
		}
		count++
		if count > 32 {
			break
		}
		socket := filepath.Join(i.directory, "connections", entry.Name(), "socket")
		if info, err := os.Lstat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
			continue
		}
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, i.binary, "-F", "/dev/null", "-S", socket, "-O", "exit", "localhost").Run()
		}()
	}
	group.Wait()
	return os.RemoveAll(i.directory)
}
