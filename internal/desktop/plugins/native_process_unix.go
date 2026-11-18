//go:build !windows

package plugins

import (
	"os/exec"
	"syscall"
)

func prepareNativeProcess(cmd *exec.Cmd) func() error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return func() error { return nil }
}
func terminateNativeTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
