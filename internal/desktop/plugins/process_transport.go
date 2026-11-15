package plugins

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	sshutil "ssh-connect/internal/ssh"
	"strings"
	"sync"
	"time"
)

type ProcessData func([]byte, bool)

func ExecutionEnvironment(policy ExecutionPolicy, overrides map[string]*string) []string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			env[name] = value
		}
	}
	for _, name := range append([]string{"ELECTRON_RUN_AS_NODE", "NODE_OPTIONS", "NODE_CHANNEL_FD", "ATERM_USER_DATA_DIR", "ATERM_DATA_DIR"}, policy.UnsetEnv...) {
		delete(env, name)
	}
	for name, value := range overrides {
		if value == nil {
			delete(env, name)
		} else {
			env[name] = *value
		}
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	slices.Sort(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+env[name])
	}
	return result
}
func ResolveExecutable(program string) (string, error) {
	if !programName.MatchString(program) {
		return "", errors.New("invalid executable name")
	}
	names := []string{program}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(program), ".exe") {
		names = []string{program + ".exe"}
	}
	paths := append(filepath.SplitList(os.Getenv("PATH")), "/usr/local/bin", "/opt/homebrew/bin")
	denied := false
	for _, directory := range paths {
		if !filepath.IsAbs(directory) {
			continue
		}
		for _, name := range names {
			candidate := filepath.Join(directory, name)
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				if os.IsPermission(err) {
					denied = true
				}
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil {
				continue
			}
			if info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0111 != 0) {
				return resolved, nil
			}
			if info.Mode().IsRegular() {
				denied = true
			}
		}
	}
	if denied {
		return "", fmt.Errorf("executable access denied: %s", program)
	}
	return "", fmt.Errorf("executable not found: %s", program)
}

type collectWriter struct {
	output *ProcessOutput
	stderr bool
	cancel context.CancelFunc
	onData ProcessData
	fault  *error
	mu     *sync.Mutex
}

func (w collectWriter) Write(data []byte) (int, error) {
	// Serialize stdout/stderr callbacks as a merged arrival stream.
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.output.Push(data, w.stderr); err != nil {
		if w.fault != nil {
			*w.fault = err
		}
		w.cancel()
		return 0, err
	}
	if w.onData != nil {
		w.onData(data, w.stderr)
	}
	return len(data), nil
}
func processContext(ctx context.Context, request ProcessRequest, stream bool) (context.Context, context.CancelFunc) {
	if !stream {
		timeout := request.TimeoutMS
		if timeout == 0 {
			timeout = 30000
		}
		return context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	}
	return context.WithCancel(ctx)
}
func RunLocalProcess(ctx context.Context, request ProcessRequest, policy ExecutionPolicy, onData ProcessData) (ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return ProcessResult{}, err
	}
	executable, err := ResolveExecutable(request.Executable)
	if err != nil {
		return ProcessResult{}, err
	}
	childCtx, cancel := processContext(ctx, request, onData != nil)
	defer cancel()
	command := exec.CommandContext(childCtx, executable, request.Args...)
	command.Env = ExecutionEnvironment(policy, request.Env)
	command.Dir = "/"
	if runtime.GOOS == "windows" {
		command.Dir, _ = os.UserHomeDir()
	}
	cleanupProcess := configureProcess(command)
	defer cleanupProcess()
	limit := request.MaxOutputBytes
	if limit == 0 {
		limit = ProcessOutputLimit
	}
	output := NewProcessOutput(limit, request.Output == "tail")
	var mu sync.Mutex
	var outputErr error
	command.Stdout = collectWriter{output: output, cancel: cancel, onData: onData, mu: &mu, fault: &outputErr}
	command.Stderr = collectWriter{output: output, stderr: true, cancel: cancel, onData: onData, mu: &mu, fault: &outputErr}
	err = command.Run()
	if outputErr != nil {
		return ProcessResult{}, outputErr
	}
	if childCtx.Err() != nil {
		return ProcessResult{}, childCtx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return ProcessResult{}, err
		}
	}
	result := output.Finish()
	code := command.ProcessState.ExitCode()
	if code >= 0 {
		result.ExitCode = &code
	}
	return result, nil
}

type SSHSessionFactory = sshutil.SessionFactory

func RunSSHProcess(ctx context.Context, client SSHSessionFactory, request ProcessRequest, policy ExecutionPolicy, onData ProcessData) (ProcessResult, error) {
	childCtx, cancel := processContext(ctx, request, onData != nil)
	defer cancel()
	if err := childCtx.Err(); err != nil {
		return ProcessResult{}, err
	}
	session, err := sshutil.OpenSession(childCtx, client)
	if err != nil {
		return ProcessResult{}, err
	}
	defer session.Close()
	stop := context.AfterFunc(childCtx, func() { session.Close() })
	defer stop()
	limit := request.MaxOutputBytes
	if limit == 0 {
		limit = ProcessOutputLimit
	}
	output := NewProcessOutput(limit, request.Output == "tail")
	var mu sync.Mutex
	var outputErr error
	session.Stdout = collectWriter{output: output, cancel: cancel, onData: onData, mu: &mu, fault: &outputErr}
	session.Stderr = collectWriter{output: output, stderr: true, cancel: cancel, onData: onData, mu: &mu, fault: &outputErr}
	command := ProgramCommand(request, policy, "")
	if onData != nil {
		command = "exec " + command
	}
	err = session.Run(command)
	if outputErr != nil {
		return ProcessResult{}, outputErr
	}
	if childCtx.Err() != nil {
		return ProcessResult{}, childCtx.Err()
	}
	result := output.Finish()
	code := 0
	if err != nil {
		var exit *ssh.ExitError
		var missing *ssh.ExitMissingError
		if errors.As(err, &exit) {
			code = exit.ExitStatus()
		} else if errors.As(err, &missing) {
			return result, nil
		} else if err != io.EOF {
			return ProcessResult{}, err
		}
	}
	result.ExitCode = &code
	return result, nil
}
