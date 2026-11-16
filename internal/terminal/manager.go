package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"runtime"
	"ssh-connect/internal/desktop/openssh"
	"ssh-connect/internal/desktop/zmodem"
	"ssh-connect/internal/service"
	sshutil "ssh-connect/internal/ssh"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"ssh-connect/internal/desktop/localpty"
)

const terminalEventName = "terminal:output"

const terminalKeepaliveFailureMessage = "Connection lost: the server did not respond to keepalive checks.\r\n"
const terminalHostKeyInputLimit = 512

type terminalHostKeyDecision uint8

const (
	terminalHostKeyPending terminalHostKeyDecision = iota
	terminalHostKeyTrust
	terminalHostKeyReject
)

type terminalHostKeyPrompt struct {
	fingerprint string
	input       []byte
	response    chan terminalHostKeyDecision
	decided     bool
	lastWasCR   bool
	overflowed  bool
}

type Session struct {
	ID               string                 `json:"id"`
	HostID           string                 `json:"hostID,omitempty"`
	Columns          int                    `json:"cols"`
	Rows             int                    `json:"rows"`
	LocalNumber      int                    `json:"localNumber,omitempty"`
	Title            string                 `json:"title"`
	Alias            string                 `json:"alias"`
	Kind             string                 `json:"kind"`
	State            string                 `json:"state"`
	ExitCode         *int                   `json:"exitCode"`
	StartedAt        string                 `json:"startedAt"`
	Remote           *openssh.RemoteContext `json:"remote,omitempty"`
	PID              int                    `json:"pid,omitempty"`
	Cwd              string                 `json:"cwd,omitempty"`
	Shell            string                 `json:"shell,omitempty"`
	integration      *openssh.Integration
	transfer         *zmodem.Router
	directoryReports *localpty.DirectoryReports

	pty                localpty.Process
	sshClient          *service.NativeSSHClient
	sshSession         *ssh.Session
	stdin              io.WriteCloser
	output             []byte
	outputOffset       uint64
	outputAttached     bool
	outputAcknowledged uint64
	outputPaused       bool
	outputResumedAt    time.Time
	outputWake         chan struct{}
	outputDone         chan struct{}
	closed             chan struct{}
	writeMu            sync.Mutex
	cancel             context.CancelFunc

	pluginTitle        string
	initialCommand     string
	connectionIdentity *service.Host
	hostKeyPrompt      *terminalHostKeyPrompt
	hostKeyRejected    bool
}

type OutputEvent struct {
	SessionID string `json:"sessionID"`
	Data      string `json:"data"`
	Offset    uint64 `json:"offset"`
	State     string `json:"state,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
}

type Manager struct {
	ctx              context.Context
	emit             func(name string, payload any)
	mu               sync.RWMutex
	sessions         map[string]*Session
	localCounter     int
	directoryPolling bool
	download         func(context.Context, string, zmodem.FileOffer) (zmodem.Download, error)
	upload           func(context.Context, string) ([]zmodem.Upload, error)
}

func NewManager(ctx context.Context, emit func(name string, payload any)) *Manager {
	return &Manager{ctx: ctx, emit: emit, sessions: make(map[string]*Session)}
}

// openSSHAsync creates the visible session before the network handshake starts.
// That way a slow or unreachable host is represented by its own terminal tab
// instead of making the rest of the app appear unresponsive.
func (m *Manager) OpenSSHAsync(alias string, connect func(context.Context, service.HostKeyConfirmer) (*service.NativeSSHClient, string, error)) (Session, error) {
	return m.openSSHWithConnector(alias, func(ctx context.Context, entry *Session) (*service.NativeSSHClient, string, error) {
		confirmer := func(confirmCtx context.Context, key service.HostKeyInfo) (bool, error) {
			return m.confirmTerminalHostKey(confirmCtx, entry, key)
		}
		return connect(ctx, confirmer)
	})
}

type LaunchOptions struct {
	windowsDirectoryNonce string
	Title                 string
	InitialCommand        string
	HostID                string
	integration           *openssh.Integration
}

func (m *Manager) openSSHWithConnector(alias string, connect func(context.Context, *Session) (*service.NativeSSHClient, string, error)) (Session, error) {
	return m.openSSHWithOptions(alias, LaunchOptions{}, connect)
}
func (m *Manager) openSSHWithOptions(alias string, options LaunchOptions, connect func(context.Context, *Session) (*service.NativeSSHClient, string, error)) (Session, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return Session{}, errors.New("host alias is required")
	}
	if m == nil || m.ctx == nil {
		return Session{}, errors.New("terminal is not initialized")
	}
	ctx, cancel := context.WithCancel(m.ctx)
	entry := &Session{
		ID:           uuid.NewString(),
		Title:        alias,
		Alias:        alias,
		Kind:         "ssh",
		State:        "connecting",
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		output:       fmt.Appendf(nil, "Connecting to %s...\r\n", alias),
		outputOffset: uint64(len(fmt.Sprintf("Connecting to %s...\r\n", alias))),
		cancel:       cancel,
		outputDone:   make(chan struct{}), closed: make(chan struct{}),
	}
	entry.HostID = options.HostID
	entry.pluginTitle = options.Title
	entry.initialCommand = options.InitialCommand
	if options.Title != "" {
		entry.Title = options.Title
	}
	m.mu.Lock()
	m.setupTransfer(entry)
	m.sessions[entry.ID] = entry
	snapshot := entry.snapshot()
	m.mu.Unlock()
	go func() {
		client, resolvedAlias, err := connect(ctx, entry)
		if err != nil {
			m.failSSHConnection(entry, err)
			return
		}
		if err := m.attachSSH(ctx, entry, client, resolvedAlias); err != nil {
			m.failSSHConnection(entry, err)
		}
	}()
	return snapshot, nil
}

func (m *Manager) confirmTerminalHostKey(ctx context.Context, entry *Session, key service.HostKeyInfo) (bool, error) {
	prompt := &terminalHostKeyPrompt{
		fingerprint: key.Fingerprint,
		response:    make(chan terminalHostKeyDecision, 1),
	}
	message := terminalHostKeyPromptMessage(key)
	encoded := base64.StdEncoding.EncodeToString([]byte(message))

	m.mu.Lock()
	if m.sessions[entry.ID] != entry || entry.State != "connecting" {
		m.mu.Unlock()
		return false, context.Canceled
	}
	entry.hostKeyPrompt = prompt
	entry.State = "verifying-host-key"
	entry.output = append(entry.output, message...)
	entry.outputOffset += uint64(len(message))
	offset := entry.outputOffset
	if len(entry.output) > 256*1024 {
		entry.output = append([]byte(nil), entry.output[len(entry.output)-256*1024:]...)
	}
	m.mu.Unlock()
	m.emitEvent(terminalEventName, OutputEvent{
		SessionID: entry.ID,
		Data:      encoded,
		Offset:    offset,
		State:     "verifying-host-key",
	})

	var decision terminalHostKeyDecision
	select {
	case <-ctx.Done():
		m.clearTerminalHostKeyPrompt(entry, prompt, false)
		return false, ctx.Err()
	case decision = <-prompt.response:
	}

	approved := decision == terminalHostKeyTrust
	m.clearTerminalHostKeyPrompt(entry, prompt, !approved)
	return approved, nil
}

func (m *Manager) clearTerminalHostKeyPrompt(entry *Session, prompt *terminalHostKeyPrompt, rejected bool) {
	m.mu.Lock()
	if m.sessions[entry.ID] != entry || entry.hostKeyPrompt != prompt {
		m.mu.Unlock()
		return
	}
	entry.hostKeyPrompt = nil
	entry.hostKeyRejected = rejected
	if entry.State == "verifying-host-key" {
		entry.State = "connecting"
	}
	state := entry.State
	m.mu.Unlock()
	if state == "connecting" {
		m.emitEvent(terminalEventName, OutputEvent{SessionID: entry.ID, State: state})
	}
}

func terminalHostKeyPromptMessage(key service.HostKeyInfo) string {
	host := key.Host
	if key.Port != "" && key.Port != "22" {
		host = fmt.Sprintf("[%s]:%s", key.Host, key.Port)
	}
	if key.PreviousFingerprint != "" {
		return fmt.Sprintf("\r\nWARNING: SSH HOST KEY HAS CHANGED FOR %s.\r\nPrevious fingerprint: %s\r\nReceived fingerprint: %s\r\nVerify the new fingerprint with the server administrator.\r\nReplace the trusted key and continue (yes/no/[fingerprint])? ", host, key.PreviousFingerprint, key.Fingerprint)
	}
	return fmt.Sprintf(
		"\r\nThe authenticity of host '%s' can't be established.\r\n%s key fingerprint is %s.\r\nAre you sure you want to continue connecting (yes/no/[fingerprint])? ",
		host,
		terminalHostKeyTypeLabel(key.KeyType),
		key.Fingerprint,
	)
}

func terminalHostKeyTypeLabel(keyType string) string {
	switch keyType {
	case ssh.KeyAlgoED25519:
		return "ED25519"
	case ssh.KeyAlgoRSA:
		return "RSA"
	default:
		if strings.HasPrefix(keyType, "ecdsa-") {
			return "ECDSA"
		}
		return keyType
	}
}

func (m *Manager) attachSSH(ctx context.Context, entry *Session, client *service.NativeSSHClient, alias string) error {
	session, err := sshutil.OpenSession(ctx, client)
	if err != nil {
		client.Close()
		return fmt.Errorf("create SSH session: %w", err)
	}
	stopSetup := context.AfterFunc(ctx, func() { client.Close(); session.Close() })
	defer stopSetup()
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("open SSH input: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("open SSH output: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("open SSH error output: %w", err)
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := session.RequestPty("xterm-256color", 32, 120, modes); err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("request remote terminal: %w", err)
	}
	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("start remote shell: %w", err)
	}
	if entry.initialCommand != "" {
		if _, err := stdin.Write([]byte(entry.initialCommand + "\r")); err != nil {
			session.Close()
			client.Close()
			return fmt.Errorf("start plugin terminal command: %w", err)
		}
	}
	m.mu.Lock()
	if m.sessions[entry.ID] != entry || entry.State != "connecting" {
		m.mu.Unlock()
		session.Close()
		client.Close()
		return context.Canceled
	}
	entry.Title = alias
	if entry.pluginTitle != "" {
		entry.Title = entry.pluginTitle
	}
	entry.Alias = alias
	entry.sshClient = client
	entry.sshSession = session
	entry.stdin = stdin
	entry.State = "running"
	m.mu.Unlock()
	m.emitEvent(terminalEventName, OutputEvent{SessionID: entry.ID, State: "running"})
	go m.forwardSSHOutput(entry, stdout, stderr)
	go m.waitForSSHExit(entry)
	return nil
}

func (m *Manager) failSSHConnection(entry *Session, err error) {
	m.mu.Lock()
	if m.sessions[entry.ID] != entry || entry.State != "connecting" {
		m.mu.Unlock()
		return
	}
	exitCode := -1
	entry.State = "exited"
	entry.ExitCode = &exitCode
	message := fmt.Sprintf("Connection failed: %v\r\n", err)
	if entry.hostKeyRejected {
		message = "Host key verification cancelled.\r\n"
	}
	entry.output = append(entry.output, message...)
	entry.outputOffset += uint64(len(message))
	offset := entry.outputOffset
	m.mu.Unlock()
	m.emitEvent(terminalEventName, OutputEvent{SessionID: entry.ID, Data: base64.StdEncoding.EncodeToString([]byte(message)), Offset: offset, State: "exited", ExitCode: &exitCode})
}

func (m *Manager) OpenLocalShell() (Session, error) {
	return m.OpenLocalShellWithOptions(LaunchOptions{})
}

// OpenLocalProgram starts an already validated native executable in its own PTY.
func (m *Manager) OpenLocalProgram(command *exec.Cmd, title string) (Session, error) {
	return m.openWithOptions("local", title, "", command, LaunchOptions{Title: title})
}

func (m *Manager) OpenLocalShellWithOptions(options LaunchOptions) (Session, error) {
	shell, err := localpty.DefaultShell()
	if err != nil {
		return Session{}, fmt.Errorf("resolve login shell: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Session{}, fmt.Errorf("resolve home directory: %w", err)
	}
	args := []string{"-l"}
	if runtime.GOOS == "windows" {
		options.windowsDirectoryNonce = uuid.NewString()
		args = localpty.PowerShellIntegration(shell, options.windowsDirectoryNonce)
	}
	var environment []string
	if runtime.GOOS != "windows" {
		integration, err := openssh.New("/usr/bin/ssh")
		if err != nil {
			return Session{}, err
		}
		environment, args, err = integration.ShellEnvironment(shell, os.Environ())
		if err != nil {
			integration.Close()
			return Session{}, err
		}
		options.integration = integration
	}
	cmd := exec.Command(shell, args...)
	cmd.Dir = homeDir
	cmd.Env = environment
	session, err := m.openWithOptions("local", "Local Shell", "", cmd, options)
	if err != nil && options.integration != nil {
		options.integration.Close()
	}
	return session, err
}

func (m *Manager) open(kind, title, alias string, cmd *exec.Cmd) (Session, error) {
	return m.openWithOptions(kind, title, alias, cmd, LaunchOptions{})
}
func (m *Manager) openWithOptions(kind, title, alias string, cmd *exec.Cmd, options LaunchOptions) (Session, error) {
	if m == nil || m.ctx == nil {
		return Session{}, errors.New("terminal is not initialized")
	}
	if err := m.ctx.Err(); err != nil {
		return Session{}, err
	}
	cmd.Env = terminalEnvironment(cmd.Environ())
	file, err := localpty.Start(cmd, 120, 32)
	if err != nil {
		return Session{}, fmt.Errorf("start terminal: %w", err)
	}
	if options.InitialCommand != "" {
		if _, err := file.Write([]byte(options.InitialCommand + "\r")); err != nil {
			_ = file.Close()
			_ = file.Kill()
			_ = file.Wait()
			return Session{}, fmt.Errorf("start plugin terminal command: %w", err)
		}
	}
	session := &Session{
		ID:         uuid.NewString(),
		Title:      title,
		Alias:      alias,
		Kind:       kind,
		State:      "running",
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		pty:        file,
		outputDone: make(chan struct{}), closed: make(chan struct{}),
		PID: file.PID(), Cwd: cmd.Dir, Shell: cmd.Path,
	}
	if options.windowsDirectoryNonce != "" {
		session.directoryReports = localpty.NewDirectoryReports(options.windowsDirectoryNonce, func(path string) { m.updateReportedDirectory(session, path) })
	}
	session.pluginTitle = options.Title
	session.initialCommand = options.InitialCommand
	session.integration = options.integration
	session.HostID = options.HostID
	if options.Title != "" {
		session.Title = options.Title
	}
	m.mu.Lock()
	m.setupTransfer(session)
	m.sessions[session.ID] = session
	if session.Kind == "local" {
		refreshLocalTitle(session)
	}
	snapshot := session.snapshot()
	m.mu.Unlock()
	go m.forwardOutput(session)
	go m.waitForExit(session)
	m.watchOpenSSH(session)
	m.startDirectoryPolling()
	return snapshot, nil
}

func (m *Manager) forwardOutput(session *Session) {
	if session.outputDone != nil {
		defer close(session.outputDone)
	}
	if session.transfer != nil {
		defer session.transfer.Close()
	}
	buffer := make([]byte, 32*1024)
	for {
		count, err := session.pty.Read(buffer)
		if count > 0 {
			if session.directoryReports != nil {
				session.directoryReports.Consume(buffer[:count])
			}
			m.appendOutput(session, buffer[:count])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// Closing a PTY commonly returns an OS-specific read error. The exit
				// event below is the authoritative terminal state.
			}
			return
		}
	}
}

func (m *Manager) forwardReader(session *Session, reader io.Reader) {
	buffer := make([]byte, 32*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			m.appendOutput(session, buffer[:count])
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) appendOutput(session *Session, data []byte) {
	if session.transfer != nil {
		session.transfer.Consume(data)
		return
	}
	m.appendTerminalText(session, data)
}
func (m *Manager) appendTerminalText(session *Session, data []byte) {
	m.mu.Lock()
	for m.sessions[session.ID] == session && session.outputAttached && session.outputPaused {
		if session.outputWake == nil {
			session.outputWake = make(chan struct{})
		}
		wake := session.outputWake
		m.mu.Unlock()
		select {
		case <-wake:
		case <-m.ctx.Done():
			return
		}
		m.mu.Lock()
	}
	if m.sessions[session.ID] != session {
		m.mu.Unlock()
		return
	}
	session.output = append(session.output, data...)
	session.outputOffset += uint64(len(data))
	offset := session.outputOffset
	if session.outputAttached && offset-session.outputAcknowledged > 256*1024 {
		session.outputPaused = true
	}
	if len(session.output) > terminalReplayLimit {
		start := len(session.output) - terminalReplayLimit
		// Start replay at a UTF-8 code point boundary. The streaming decoder
		// retains any incomplete code point at the end until live output arrives.
		for start < len(session.output) && session.output[start]&0xc0 == 0x80 {
			start++
		}
		session.output = append([]byte(nil), session.output[start:]...)
	}
	attached := session.outputAttached
	m.mu.Unlock()
	if attached {
		m.emitEvent(terminalEventName, OutputEvent{SessionID: session.ID, Data: base64.StdEncoding.EncodeToString(data), Offset: offset})
	}
}

type OutputSnapshot struct {
	Data   string `json:"data"`
	Offset uint64 `json:"offset"`
}

func (m *Manager) OutputSnapshot(id string) (OutputSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[id]
	if session == nil {
		return OutputSnapshot{}, errors.New("terminal session was not found")
	}
	return OutputSnapshot{Data: base64.StdEncoding.EncodeToString(session.output), Offset: session.outputOffset}, nil
}

func (m *Manager) Scrollback(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[id]
	if session == nil {
		return "", errors.New("terminal session was not found")
	}
	return base64.StdEncoding.EncodeToString(session.output), nil
}

func (m *Manager) waitForExit(session *Session) {
	err := session.pty.Wait()
	m.drainTerminalExit(session, func() { _ = session.pty.Close() })
	_ = session.pty.Close()
	if session.integration != nil {
		session.integration.Close()
	}
	exitCode := 0
	if err != nil {
		var exited interface{ ExitCode() int }
		if errors.As(err, &exited) {
			exitCode = exited.ExitCode()
		} else {
			exitCode = -1
		}
	}
	m.finishTerminalExit(session, exitCode)
}

func (m *Manager) waitForSSHExit(session *Session) {
	err := session.sshSession.Wait()
	keepaliveCause := session.sshClient.Cause()
	exitCode := 0
	if err != nil {
		var exitError *ssh.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitStatus()
		} else {
			exitCode = -1
		}
	}
	if errors.Is(keepaliveCause, service.ErrSSHKeepaliveTimeout) {
		exitCode = -1
	}
	m.drainTerminalExit(session, func() { _ = session.sshSession.Close() })
	session.sshClient.Close()
	if message := terminalConnectionFailureMessage(keepaliveCause); message != "" {
		m.appendOutput(session, []byte(message))
	}
	m.finishTerminalExit(session, exitCode)
}

func terminalConnectionFailureMessage(cause error) string {
	if errors.Is(cause, service.ErrSSHKeepaliveTimeout) {
		return terminalKeepaliveFailureMessage
	}
	return ""
}

func (m *Manager) emitEvent(name string, payload any) {
	if m.emit != nil {
		m.emit(name, payload)
	}
}

func (m *Manager) Write(id, encoded string) error {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("terminal input is invalid")
	}
	session, err := m.session(id)
	if err != nil {
		return err
	}
	if session.transfer != nil && session.transfer.Busy() {
		if bytes.IndexByte(data, 3) >= 0 {
			return session.transfer.Cancel()
		}
		return nil
	}
	return m.writeBytes(id, data)
}
func (m *Manager) writeBytes(id string, data []byte) error {
	session, err := m.session(id)
	if err != nil {
		return err
	}
	session.writeMu.Lock()
	defer session.writeMu.Unlock()
	m.mu.RLock()
	state := session.State
	prompt := session.hostKeyPrompt
	stdin := session.stdin
	terminalPTY := session.pty
	m.mu.RUnlock()
	if state == "verifying-host-key" && prompt != nil {
		m.writeTerminalHostKeyPrompt(session, prompt, data)
		return nil
	}
	if state != "running" {
		return errors.New("terminal session is not ready for input")
	}
	if stdin != nil {
		return writeTerminalInput(stdin, data)
	}
	if terminalPTY != nil {
		return writeTerminalInput(terminalPTY, data)
	}
	return errors.New("terminal input is unavailable")
}

// A transport may accept only part of a protocol frame. Keep the terminal's
// write lock for the full frame so another input producer cannot interleave it.
func writeTerminalInput(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		count, err := writer.Write(data)
		if count < 0 || count > len(data) {
			return io.ErrShortWrite
		}
		data = data[count:]
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (m *Manager) writeTerminalHostKeyPrompt(session *Session, prompt *terminalHostKeyPrompt, data []byte) {
	if output := processTerminalHostKeyInput(prompt, data); output != "" {
		m.appendOutput(session, []byte(output))
	}
}

func processTerminalHostKeyInput(prompt *terminalHostKeyPrompt, data []byte) string {
	var output strings.Builder
inputLoop:
	for _, character := range data {
		if prompt.decided {
			break
		}
		if character != '\n' {
			prompt.lastWasCR = false
		}
		switch character {
		case 0x03:
			output.WriteString("^C\r\n")
			prompt.decided = true
			prompt.response <- terminalHostKeyReject
		case '\r', '\n':
			if character == '\n' && prompt.lastWasCR {
				prompt.lastWasCR = false
				continue
			}
			prompt.lastWasCR = character == '\r'
			output.WriteString("\r\n")
			answer := string(prompt.input)
			prompt.input = prompt.input[:0]
			prompt.overflowed = false
			switch parseTerminalHostKeyAnswer(answer, prompt.fingerprint) {
			case terminalHostKeyTrust:
				output.WriteString("Host key accepted. Continuing connection...\r\n")
				prompt.decided = true
				prompt.response <- terminalHostKeyTrust
			case terminalHostKeyReject:
				prompt.decided = true
				prompt.response <- terminalHostKeyReject
			default:
				if looksLikeTerminalFingerprint(answer) {
					output.WriteString("Fingerprint does not match.\r\n")
				}
				output.WriteString("Please type yes (y), no (n), or the full fingerprint: ")
				// Treat each frontend input event as one submission when it
				// contains a newline. Discard any pasted trailing lines so they
				// can never become remote shell input after the handshake.
				break inputLoop
			}
		case 0x08, 0x7f:
			if len(prompt.input) > 0 {
				prompt.input = prompt.input[:len(prompt.input)-1]
				output.WriteString("\b \b")
			}
		default:
			if character < 0x20 || character > 0x7e {
				continue
			}
			if len(prompt.input) >= terminalHostKeyInputLimit {
				if !prompt.overflowed {
					output.WriteByte('\a')
					prompt.overflowed = true
				}
				continue
			}
			prompt.input = append(prompt.input, character)
			output.WriteByte(character)
		}
	}
	return output.String()
}

func parseTerminalHostKeyAnswer(answer, fingerprint string) terminalHostKeyDecision {
	answer = strings.TrimSpace(answer)
	switch strings.ToLower(answer) {
	case "y", "ye", "yes":
		return terminalHostKeyTrust
	case "n", "no":
		return terminalHostKeyReject
	}
	if terminalFingerprintMatches(answer, fingerprint) {
		return terminalHostKeyTrust
	}
	return terminalHostKeyPending
}

func terminalFingerprintMatches(answer, fingerprint string) bool {
	answerParts := strings.SplitN(strings.TrimSpace(answer), ":", 2)
	fingerprintParts := strings.SplitN(strings.TrimSpace(fingerprint), ":", 2)
	return len(answerParts) == 2 &&
		len(fingerprintParts) == 2 &&
		strings.EqualFold(answerParts[0], fingerprintParts[0]) &&
		answerParts[1] == fingerprintParts[1]
}

func looksLikeTerminalFingerprint(answer string) bool {
	parts := strings.SplitN(strings.TrimSpace(answer), ":", 2)
	return len(parts) == 2 && strings.EqualFold(parts[0], "SHA256")
}

func (m *Manager) Resize(id string, columns, rows uint16) error {
	if columns == 0 || rows == 0 {
		return nil
	}
	session, err := m.session(id)
	if err != nil {
		return err
	}
	m.mu.RLock()
	state := session.State
	sshSession := session.sshSession
	terminalPTY := session.pty
	m.mu.RUnlock()
	if state != "running" {
		return nil
	}
	if sshSession != nil {
		return sshSession.WindowChange(int(rows), int(columns))
	}
	if terminalPTY == nil {
		return nil
	}
	return terminalPTY.Resize(columns, rows)
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	session := m.sessions[id]
	if session == nil {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, id)
	if session.closed != nil {
		close(session.closed)
	}
	wakeTerminalOutput(session)
	if session.cancel != nil {
		session.cancel()
	}
	if session.State == "connecting" || session.State == "verifying-host-key" {
		exitCode := -1
		session.State = "exited"
		session.ExitCode = &exitCode
		m.mu.Unlock()
		return nil
	}
	sshSession := session.sshSession
	sshClient := session.sshClient
	terminalPTY := session.pty
	state := session.State
	m.mu.Unlock()
	if session.transfer != nil {
		session.transfer.Close()
	}
	if session.integration != nil {
		session.integration.Close()
	}
	if sshSession != nil {
		_ = sshSession.Close()
		sshClient.Close()
		return nil
	}
	if terminalPTY != nil && state == "running" {
		_ = terminalPTY.Kill()
	}
	if terminalPTY == nil {
		return nil
	}
	_ = terminalPTY.Close()
	return nil
}

func (m *Manager) CloseAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Close(id)
	}
}

func (m *Manager) session(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[id]
	if session == nil {
		return nil, errors.New("terminal session was not found")
	}
	return session, nil
}

func (s *Session) snapshot() Session {
	// Snapshots must not let adapters mutate live metadata after the lock is released.
	var remote *openssh.RemoteContext
	if s.Remote != nil {
		copy := *s.Remote
		remote = &copy
	}
	var exitCode *int
	if s.ExitCode != nil {
		copy := *s.ExitCode
		exitCode = &copy
	}
	return Session{HostID: s.HostID, LocalNumber: s.LocalNumber, Columns: s.Columns, Rows: s.Rows, ID: s.ID, Title: s.Title, Alias: s.Alias, Kind: s.Kind, State: s.State, ExitCode: exitCode, StartedAt: s.StartedAt, Remote: remote, PID: s.PID, Cwd: s.Cwd, Shell: s.Shell}
}
