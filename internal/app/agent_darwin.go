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
	"ssh-connect/internal/terminal"
)

type agentConfig struct {
	Enabled bool            `json:"enabled"`
	Hosts   map[string]bool `json:"hosts"`
}
type agentState struct {
	mu      sync.Mutex
	dir     string
	server  *http.Server
	config  agentConfig
	pending *agentOpen
}
type agentOpen struct {
	id, host, session string
	ctx               context.Context
	cancel            context.CancelFunc
	route             service.AgentRoute
	done              chan bool
	claimed           bool
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
			if s.config.Hosts[h.ID] && strings.Contains(strings.ToLower(h.DisplayName()), strings.ToLower(in.Query)) {
				result.Hosts = append(result.Hosts, agentbridge.Host{ID: h.ID, Name: h.DisplayName(), Group: h.DirectoryID})
			}
		}
	case in.Method != "open" || !s.config.Hosts[in.HostID]:
		result.Error = "HOST_NOT_AVAILABLE"
	case s.pending != nil:
		result.Error = "REQUEST_IN_PROGRESS"
	default:
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		p := &agentOpen{id: uuid.NewString(), host: in.HostID, ctx: ctx, cancel: cancel, done: make(chan bool, 1)}
		s.pending = p
		s.mu.Unlock()
		route, err := a.approveAgentRoute(ctx, p.host)
		if err == nil {
			var locked bool
			locked, err = a.svc.AgentRouteRequiresUnlock(ctx, route)
			if err == nil && ctx.Err() == nil {
				s.mu.Lock()
				p.route = route
				s.mu.Unlock()
				a.emitEvent("agent:open", map[string]any{"id": p.id, "locked": locked})
				select {
				case success := <-p.done:
					if success {
						result.Status = "terminal_opened"
					}
				case <-ctx.Done():
				}
			}
		}
		cancel()
		s.mu.Lock()
		if result.Status == "" {
			result.Error = "OPEN_CANCELLED"
			if p.session != "" {
				_ = a.CloseTerminal(p.session)
			}
		}
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
			s.server.Close()
			s.server = nil
		}
		return nil, nil
	}
	id, err := argument[string](args, 0)
	p := s.pending
	if err != nil || p == nil || id != p.id || p.ctx.Err() != nil || !s.config.Hosts[p.host] {
		return nil, errors.New("REQUEST_EXPIRED")
	}
	switch op {
	case "claim":
		if p.claimed || len(p.route.Endpoints()) == 0 {
			return nil, errors.New("REQUEST_CONSUMED")
		}
		p.claimed = true
		locked, err := a.svc.AgentRouteRequiresUnlock(p.ctx, p.route)
		if err != nil || locked {
			return nil, errors.New("VAULT_LOCKED")
		}
		session, err := a.terminal.OpenSSH(p.host, terminal.LaunchOptions{HostID: p.host}, func(ctx context.Context, id string) (*service.NativeSSHClient, service.Host, error) {
			return a.svc.OpenApprovedAgentRoute(ctx, p.route, a.hostKeyConfirmer(id), a.credentialPrompter(id))
		})
		if err != nil {
			return nil, err
		}
		p.session = session.ID
		a.terminal.Present(session.ID, p.host, false)
		return session.ID, nil
	case "finish":
		ok, err := argument[bool](args, 1)
		if err != nil {
			return nil, err
		}
		select {
		case p.done <- ok && p.session != "":
		default:
		}
		return nil, nil
	}
	return nil, errors.New("unknown agent operation")
}
