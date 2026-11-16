//go:build !windows

package plugins

import "os/exec"

func configureWorkerCommand(_ *exec.Cmd) {}

func workerEnvironment() []string { return []string{} }
