//go:build !windows

package localpty

import (
	"github.com/creack/pty"
	"os"
	"os/exec"
)

type process struct {
	*os.File
	command *exec.Cmd
}

func start(command *exec.Cmd, columns, rows uint16) (Process, error) {
	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: columns, Rows: rows})
	if err != nil {
		return nil, err
	}
	return &process{File: file, command: command}, nil
}
func (p *process) PID() int { return p.command.Process.Pid }
func (p *process) Resize(columns, rows uint16) error {
	return pty.Setsize(p.File, &pty.Winsize{Cols: columns, Rows: rows})
}
func (p *process) Wait() error { return p.command.Wait() }
func (p *process) Kill() error { return p.command.Process.Kill() }

// DefaultShell follows os.userInfo().shell || SHELL || /bin/sh in Electron.
func DefaultShell() (string, error) {
	account, err := accountShell()
	if err != nil {
		return "", err
	}
	return selectUnixShell(account, os.Getenv("SHELL")), nil
}
func selectUnixShell(account, environment string) string {
	if account != "" {
		return account
	}
	if environment != "" {
		return environment
	}
	return "/bin/sh"
}
