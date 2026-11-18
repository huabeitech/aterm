package plugins

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin/runner"
	"io"
	"os"
	"os/exec"
)

// go-plugin normally replaces stdin with the application's stdin. The custom
// runner supplies a private lease pipe and attaches OS process ownership before
// plugin initialization; protocol negotiation remains owned by go-plugin.
type nativeRunner struct {
	cmd            *exec.Cmd
	stdout, stderr io.ReadCloser
	attach         func() error
}

func nativeProcessRunner(command *exec.Cmd, input *os.File, attach func() error) func(hclog.Logger, *exec.Cmd, string) (runner.Runner, error) {
	return func(_ hclog.Logger, configured *exec.Cmd, _ string) (runner.Runner, error) {
		cmd := command
		cmd.Env = append(cmd.Env, configured.Env...)
		cmd.Stdin = input
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		diagnostics, err := cmd.StderrPipe()
		if err != nil {
			return nil, err
		}
		return &nativeRunner{cmd, out, diagnostics, attach}, nil
	}
}
func (r *nativeRunner) Start(context.Context) error {
	if err := r.cmd.Start(); err != nil {
		return err
	}
	if err := r.attach(); err != nil {
		r.cmd.Process.Kill()
		r.cmd.Wait()
		return err
	}
	return nil
}
func (r *nativeRunner) Wait(context.Context) error { return r.cmd.Wait() }
func (r *nativeRunner) Kill(context.Context) error {
	if r.cmd.Process == nil {
		return nil
	}
	err := r.cmd.Process.Kill()
	terminateNativeTree(r.cmd)
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
func (r *nativeRunner) Stdout() io.ReadCloser { return r.stdout }
func (r *nativeRunner) Stderr() io.ReadCloser { return r.stderr }
func (r *nativeRunner) Name() string          { return r.cmd.Path }
func (r *nativeRunner) ID() string {
	if r.cmd.Process == nil {
		return ""
	}
	return fmt.Sprint(r.cmd.Process.Pid)
}
func (r *nativeRunner) Diagnose(context.Context) string { return "Native plugin handshake failed" }
func (r *nativeRunner) PluginToHost(network, address string) (string, string, error) {
	return network, address, nil
}
func (r *nativeRunner) HostToPlugin(network, address string) (string, string, error) {
	return network, address, nil
}
