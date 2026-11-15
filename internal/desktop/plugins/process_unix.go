//go:build !windows

package plugins

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func configureProcess(command *exec.Cmd) func() {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = 250 * time.Millisecond
	var mu sync.Mutex
	var timer *time.Timer
	finished, cancelled := false, false
	command.Cancel = func() error {
		mu.Lock()
		defer mu.Unlock()
		if finished || command.Process == nil {
			return os.ErrProcessDone
		}
		cancelled = true
		pid := command.Process.Pid
		err := syscall.Kill(-pid, syscall.SIGTERM)
		timer = time.AfterFunc(200*time.Millisecond, func() {
			mu.Lock()
			defer mu.Unlock()
			if !finished {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		})
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return func() {
		mu.Lock()
		defer mu.Unlock()
		finished = true
		if timer != nil {
			timer.Stop()
		}
		if cancelled && command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
	}
}
