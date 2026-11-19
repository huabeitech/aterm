//go:build darwin && !mas

package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"ssh-connect/internal/agentbridge"
	"ssh-connect/internal/fsutil"
	"ssh-connect/internal/service"
)

type agentConfig struct {
	Enabled bool `json:"enabled"`
}
type agentState struct {
	mu        sync.Mutex
	dir       string
	server    *http.Server
	config    agentConfig
	pending   *agentOpen
	leaseRoot context.Context
	revoke    context.CancelFunc
}
type agentOpen struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan bool
}

func (a *App) startAgents(dir string) error {
	s := &a.agents
	s.dir = dir
	data, err := os.ReadFile(filepath.Join(dir, "agent-access.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &s.config); err != nil {
		return err
	}
	if s.config.Enabled {
		return a.listenAgents()
	}
	return nil
}
func (s *agentState) socket() string { return filepath.Join(s.dir, "agent.sock") }
func (a *App) listenAgents() error {
	s := &a.agents
	// The desktop profile lock is held until shutdown.
	path := s.socket()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("invalid agent socket path")
		}
		conn, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			conn.Close()
			return errors.New("agent socket already active")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	if err = os.Chmod(path, 0600); err != nil {
		l.Close()
		return err
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, Handler: http.HandlerFunc(a.agentHTTP), ConnState: func(c net.Conn, state http.ConnState) {
		if state != http.StateNew {
			return
		}
		raw, err := c.(*net.UnixConn).SyscallConn()
		ok := false
		if err == nil {
			_ = raw.Control(func(fd uintptr) {
				cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
				ok = e == nil && cred.Uid == uint32(os.Getuid())
			})
		}
		if !ok {
			c.Close()
		}
	}}
	s.leaseRoot, s.revoke = context.WithCancel(context.Background())
	s.server = server
	go server.Serve(l)
	return nil
}
func (a *App) stopAgents() {
	s := &a.agents
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending != nil {
		s.pending.cancel()
	}
	if s.server != nil {
		s.revoke()
		s.server.Close()
		s.server = nil
	}
}
func (a *App) agentHTTP(w http.ResponseWriter, r *http.Request) {
	var in agentbridge.Request
	if r.Method != "POST" || r.URL.Path != "/request" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	s := &a.agents
	s.mu.Lock()
	result := agentbridge.Result{}
	switch {
	case !s.config.Enabled:
		result.Error = "AGENT_DISABLED"
	case in.Method == "list":
		hosts, err := a.ListHosts()
		if err != nil {
			result.Error = "LIBRARY_UNAVAILABLE"
			break
		}
		for _, h := range hosts {
			if strings.Contains(strings.ToLower(h.DisplayName()+" "+h.Remark+" "+h.HostName), strings.ToLower(in.Query)) {
				result.Hosts = append(result.Hosts, agentbridge.Host{ID: h.ID, Name: h.DisplayName(), Group: h.DirectoryID, Address: h.HostName, Port: sshPort(h.Port), User: h.User, Remark: h.Remark})
			}
		}
	case in.Method != "prepare" || in.HostID == "":
		result.Error = "HOST_NOT_AVAILABLE"
	case s.pending != nil:
		result.Error = "REQUEST_IN_PROGRESS"
	default:
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		p := &agentOpen{id: uuid.NewString(), ctx: ctx, cancel: cancel, done: make(chan bool, 1)}
		root := s.leaseRoot
		s.pending = p
		s.mu.Unlock()
		route, err := a.approveAgentRoute(ctx, in.HostID)
		if err == nil {
			var locked bool
			locked, err = a.svc.AgentRouteRequiresUnlock(ctx, route)
			if err == nil && locked {
				a.emitEvent("agent:unlock", map[string]string{"id": p.id})
				select {
				case ok := <-p.done:
					if !ok {
						err = errors.New("VAULT_LOCKED")
					}
				case <-ctx.Done():
					err = ctx.Err()
				}
			}
		}
		if err == nil && ctx.Err() == nil {
			lease, revoke := context.WithTimeout(root, 2*time.Minute)
			result.Connection, err = a.agentCommand(ctx, lease, route)
			if err != nil || ctx.Err() != nil {
				revoke()
			}
		}
		s.mu.Lock()
		if err != nil || ctx.Err() != nil || !s.config.Enabled {
			result.Connection = nil
			result.Error = "PREPARATION_FAILED"
			if errors.Is(err, service.ErrHostNotFound) {
				result.Error = "HOST_NOT_AVAILABLE"
			}
		}
		cancel()
		s.pending = nil
	}
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(result)
}
func (a *App) agentDesktop(op string, args []json.RawMessage) (any, error) {
	s := &a.agents
	s.mu.Lock()
	defer s.mu.Unlock()
	switch op {
	case "config":
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		return map[string]any{"config": s.config, "socket": s.socket(), "helper": filepath.Join(filepath.Dir(exe), "aterm-mcp")}, nil
	case "save":
		cfg, err := argument[agentConfig](args, 0)
		if err != nil {
			return nil, err
		}
		if cfg.Enabled && s.server == nil {
			if err = a.listenAgents(); err != nil {
				return nil, err
			}
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		if err = fsutil.AtomicWriteFile(filepath.Join(s.dir, "agent-access.json"), data, 0600); err != nil {
			if !s.config.Enabled && s.server != nil {
				s.revoke()
				s.server.Close()
				s.server = nil
			}
			return nil, err
		}
		s.config = cfg
		if s.pending != nil {
			s.pending.cancel()
		}
		if !cfg.Enabled && s.server != nil {
			s.revoke()
			s.server.Close()
			s.server = nil
		}
		return nil, nil
	}
	id, err := argument[string](args, 0)
	p := s.pending
	if err != nil || p == nil || id != p.id || p.ctx.Err() != nil || !s.config.Enabled {
		return nil, errors.New("REQUEST_EXPIRED")
	}
	if op != "unlocked" {
		return nil, errors.New("unknown agent operation")
	}
	ok, err := argument[bool](args, 1)
	if err != nil {
		return nil, err
	}
	select {
	case p.done <- ok:
	default:
	}
	return nil, nil
}
