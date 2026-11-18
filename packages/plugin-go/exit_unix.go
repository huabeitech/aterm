//go:build !windows

package plugin

import (
	"os"
	"syscall"
)

func exitWithChildren() {
	pid := os.Getpid()
	if syscall.Getpgrp() == pid {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	os.Exit(0)
}
