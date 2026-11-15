//go:build windows

package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func configureProcess(command *exec.Cmd) func() {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.WaitDelay = 250 * time.Millisecond
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		kill := exec.Command(filepath.Join(root, "System32", "taskkill.exe"), "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return kill.Run()
	}

	return func() {}
}
