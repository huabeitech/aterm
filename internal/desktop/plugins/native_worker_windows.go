//go:build windows

package plugins

import (
	"os"
	"os/exec"
	"syscall"
)

func configureWorkerCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func workerEnvironment() []string {
	return []string{"SystemRoot=" + os.Getenv("SystemRoot")}
}
