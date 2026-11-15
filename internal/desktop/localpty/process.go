// Package localpty owns native local terminal processes and their PTY handles.
// Session orchestration, output replay and renderer events remain outside it.
package localpty

import (
	"errors"
	"io"
	"os/exec"
)

type Process interface {
	io.ReadWriteCloser
	PID() int
	Resize(columns, rows uint16) error
	Wait() error
	Kill() error
}

func Start(command *exec.Cmd, columns, rows uint16) (Process, error) {
	if command == nil || command.Path == "" {
		return nil, errors.New("terminal program is required")
	}
	if columns == 0 || rows == 0 {
		return nil, errors.New("invalid terminal size")
	}
	return start(command, columns, rows)
}
