//go:build windows

package localpty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ConPTY uses synchronous pipes serviced independently by the session's reader
// and input writer. No cursor inheritance query or ordinary console is created.
// https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
type process struct {
	input, output *os.File
	console       windows.Handle
	handle        windows.Handle
	pid           int
	mu            sync.Mutex
	closeOnce     sync.Once
	waitOnce      sync.Once
	waitErr       error
}

func start(command *exec.Cmd, columns, rows uint16) (Process, error) {
	if command == nil || command.Path == "" {
		return nil, errors.New("terminal program is required")
	}
	if command.Err != nil {
		return nil, command.Err
	}
	if columns == 0 || rows == 0 || columns > 32767 || rows > 32767 {
		return nil, errors.New("invalid terminal size")
	}
	// Report unsupported systems without panicking in a lazy syscall lookup.
	if err := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find(); err != nil {
		return nil, fmt.Errorf("Windows ConPTY is unavailable: %w", err)
	}
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer inRead.Close()
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inWrite.Close()
		return nil, err
	}
	defer outWrite.Close()
	p := &process{input: inWrite, output: outRead}
	success := false
	defer func() {
		if !success {
			p.Close()
		}
	}()
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(columns), Y: int16(rows)}, windows.Handle(inRead.Fd()), windows.Handle(outWrite.Fd()), 0, &p.console); err != nil {
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	// Unlike most attributes, PSEUDOCONSOLE takes the HPCON value itself,
	// not a pointer to a variable containing that value (Microsoft's contract).
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(p.console), unsafe.Sizeof(p.console)); err != nil {
		return nil, err
	}
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attributes.List()}
	program, err := windows.UTF16PtrFromString(command.Path)
	if err != nil {
		return nil, err
	}
	args := command.Args
	if len(args) == 0 {
		args = []string{command.Path}
	}
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = syscall.EscapeArg(arg)
	}
	line, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return nil, err
	}
	var directory *uint16
	if command.Dir != "" {
		directory, err = windows.UTF16PtrFromString(command.Dir)
		if err != nil {
			return nil, err
		}
	}
	environment, err := environmentBlock(command.Environ())
	if err != nil {
		return nil, err
	}
	var info windows.ProcessInformation
	err = windows.CreateProcess(program, line, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, &environment[0], directory, &startup.StartupInfo, &info)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(environment)
	if err != nil {
		return nil, err
	}
	windows.CloseHandle(info.Thread)
	p.handle, p.pid = info.Process, int(info.ProcessId)
	success = true
	return p, nil
}
func environmentBlock(environment []string) ([]uint16, error) {
	entries := append([]string(nil), environment...)
	for _, value := range entries {
		if strings.ContainsRune(value, 0) {
			return nil, errors.New("environment contains NUL")
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return strings.ToUpper(entries[i]) < strings.ToUpper(entries[j]) })
	block := utf16.Encode([]rune(strings.Join(entries, "\x00") + "\x00\x00"))
	return block, nil
}
func (p *process) Read(data []byte) (int, error)  { return p.output.Read(data) }
func (p *process) Write(data []byte) (int, error) { return p.input.Write(data) }
func (p *process) PID() int                       { return p.pid }
func (p *process) Resize(columns, rows uint16) error {
	if columns == 0 || rows == 0 || columns > 32767 || rows > 32767 {
		return errors.New("invalid terminal size")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.console == 0 {
		return os.ErrClosed
	}
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(columns), Y: int16(rows)})
}
func (p *process) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return os.ErrProcessDone
	}
	return windows.TerminateProcess(p.handle, 1)
}

type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e *exitError) ExitCode() int { return e.code }
func (p *process) Wait() error {
	p.waitOnce.Do(func() {
		defer func() {
			p.mu.Lock()
			if p.handle != 0 {
				windows.CloseHandle(p.handle)
				p.handle = 0
			}
			p.mu.Unlock()
			p.closeConsole() // Final frames drain on the independent PTY reader before EOF.
		}()
		// Wait owns the process handle until it has observed and read the exit code.
		status, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
		if err != nil {
			p.waitErr = err
			return
		}
		if status != windows.WAIT_OBJECT_0 {
			p.waitErr = errors.New("unexpected process wait result")
			return
		}
		p.mu.Lock()
		var code uint32
		err = windows.GetExitCodeProcess(p.handle, &code)
		windows.CloseHandle(p.handle)
		p.handle = 0
		p.mu.Unlock()
		if err != nil {
			p.waitErr = err
		} else if code != 0 {
			p.waitErr = &exitError{int(code)}
		}
	})
	return p.waitErr
}
func (p *process) Close() error {
	p.closeOnce.Do(func() {
		p.Kill()
		// Closing the pipes unblocks pending I/O and prevents ClosePseudoConsole's
		// final frame from deadlocking against an unconsumed output buffer.
		p.input.Close()
		p.output.Close()
		p.closeConsole()
	})
	return nil
}

// Detach under the lock, but do not hold it during ClosePseudoConsole: that
// call can wait for the output reader, while concurrent Close must close pipes
// to release a reader blocked by application shutdown/backpressure.
func (p *process) closeConsole() {
	p.mu.Lock()
	console := p.console
	p.console = 0
	p.mu.Unlock()
	if console != 0 {
		windows.ClosePseudoConsole(console)
	}
}

// DefaultShell follows Electron's Windows discovery order.
func DefaultShell() (string, error) {
	for _, candidate := range []struct{ root, path string }{
		{os.Getenv("ProgramFiles"), filepath.Join("PowerShell", "7", "pwsh.exe")},
		{os.Getenv("SystemRoot"), filepath.Join("System32", "WindowsPowerShell", "v1.0", "powershell.exe")},
	} {
		if candidate.root != "" {
			path := filepath.Join(candidate.root, candidate.path)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}
	if shell := os.Getenv("COMSPEC"); shell != "" {
		return shell, nil
	}
	return "cmd.exe", nil
}
