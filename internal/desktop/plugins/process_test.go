package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessPolicyBoundsAndQuoting(t *testing.T) {
	value := `literal ' $(printf injected) ; 中文`
	input, _ := json.Marshal(map[string]any{"executable": "printf", "args": []string{"%s", value}, "output": "tail", "maxOutputBytes": 1024})
	request, err := ValidateProcessRequest(input, false, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunLocalProcess(context.Background(), request, ExecutionPolicy{}, nil)
	if err != nil || result.Stdout != value || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatal(result, err)
	}
	for _, input := range []string{`{"executable":"/bin/sh","args":[]}`, `{"executable":"printf","args":null}`, `{"executable":"printf","args":["\u0000"]}`, `{"executable":"printf","args":[],"env":null}`, `{"executable":"printf","args":[],"output":null}`, `{"executable":"printf","args":[],"timeoutMs":99}`, `{"executable":"printf","args":[],"maxOutputBytes":131073}`} {
		if _, err := ValidateProcessRequest(json.RawMessage(input), false, false); err == nil {
			t.Fatal("accepted invalid request", input)
		}
	}
	manifest := fixturePackage(t).Manifest
	manifest.Permissions = []string{"process.execute"}
	manifest.Execution = &ExecutionPolicy{Programs: []string{"printf"}, Targets: []string{"local"}, Env: []string{"LANG"}}
	if err = AuthorizeExecution(manifest, "process.execute", "local", request); err != nil {
		t.Fatal(err)
	}
	if err = AuthorizeExecution(manifest, "process.execute", "ssh", request); err == nil {
		t.Fatal("granted undeclared target")
	}
	if err = AuthorizeExecution(manifest, "terminal.create", "local", request); err == nil {
		t.Fatal("granted undeclared capability")
	}
}
func TestProcessOutputSharedBudgetTailAndUTF8(t *testing.T) {
	output := NewProcessOutput(6, true)
	output.Push([]byte("中文"), false)
	output.Push([]byte("ab"), true)
	result := output.Finish()
	if result.Stdout != "文" || result.Stderr != "ab" || result.Truncated == nil || !*result.Truncated {
		t.Fatal(result)
	}
	strict := NewProcessOutput(4, false)
	strict.Push([]byte("abc"), false)
	if err := strict.Push([]byte("de"), true); err == nil {
		t.Fatal("stdout and stderr did not share one budget")
	}
}
func TestLocalExecutionCancelsAndReportsOutputOverflow(t *testing.T) {
	request, _ := ValidateProcessRequest(json.RawMessage(`{"executable":"sh","args":["-c","sleep 5 & wait"],"timeoutMs":100}`), false, false)
	start := time.Now()
	if _, err := RunLocalProcess(context.Background(), request, ExecutionPolicy{}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("process group did not stop promptly")
	}
	request, _ = ValidateProcessRequest(json.RawMessage(`{"executable":"sh","args":["-c","yes x"],"maxOutputBytes":1024}`), false, false)
	if _, err := RunLocalProcess(context.Background(), request, ExecutionPolicy{}, nil); err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatal("overflow did not stop execution with its cause", err)
	}
}
func TestLocalExecutableSearchAndEnvironment(t *testing.T) {
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, "fixture-unsafe"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", ".")
	original, _ := os.Getwd()
	os.Chdir(directory)
	t.Cleanup(func() { os.Chdir(original) })
	if _, err := ResolveExecutable("fixture-unsafe"); err == nil {
		t.Fatal("resolved the current working directory")
	}
	t.Setenv("NODE_OPTIONS", "do-not-leak")
	t.Setenv("ATERM_DATA_DIR", "private-profile")
	keep := "fixture"
	env := ExecutionEnvironment(ExecutionPolicy{UnsetEnv: []string{"HOME"}}, map[string]*string{"LANG": &keep})
	for _, entry := range env {
		if strings.HasPrefix(entry, "NODE_OPTIONS=") || strings.HasPrefix(entry, "ATERM_DATA_DIR=") || strings.HasPrefix(entry, "HOME=") {
			t.Fatal("inherited removed internal environment", entry)
		}
	}
}
func TestExecutorAuthorizationAuditAndCancellation(t *testing.T) {
	audit := NewAudit(filepath.Join(t.TempDir(), "audit.json"))
	var runs atomic.Int32
	executor := NewExecutor(ExecutionOptions{Audit: audit, Run: func(ctx context.Context, session Session, request ProcessRequest, policy ExecutionPolicy, onData ProcessData) (ProcessResult, error) {
		runs.Add(1)
		return RunLocalProcess(ctx, request, policy, onData)
	}})
	manifest := fixturePackage(t).Manifest
	manifest.Permissions = []string{"process.execute"}
	manifest.Execution = &ExecutionPolicy{Programs: []string{"printf", "sh"}, Targets: []string{"local"}}
	scope := CapabilityScope{PluginID: manifest.ID, Manifest: manifest, Owner: 1, Session: Session{ID: "fixture", Kind: "local"}, GrantID: strings.Repeat("a", 64), Context: context.Background(), Authorized: func() bool { return true }}
	secret := "fixture-not-in-audit"
	input, _ := json.Marshal(map[string]any{"request": map[string]any{"executable": "printf", "args": []string{"%s", secret}}})
	result, err := executor.Request(context.Background(), "process.execute", input, scope)
	if err != nil || result.(ProcessResult).Stdout != secret {
		t.Fatal(result, err)
	}
	records := audit.List(manifest.ID)
	data, _ := json.Marshal(records)
	if strings.Contains(string(data), secret) || len(records) != 2 || records[0].Outcome != "succeeded" {
		t.Fatal("audit omitted results or retained argument contents")
	}
	denied := scope
	denied.Authorized = func() bool { return false }
	if _, err = executor.Request(context.Background(), "process.execute", input, denied); err == nil || runs.Load() != 1 {
		t.Fatal("denied operation reached transport")
	}
	lifetime, cancel := context.WithCancel(context.Background())
	scope.Context = lifetime
	done := make(chan error, 1)
	go func() {
		_, err := executor.Request(context.Background(), "process.execute", json.RawMessage(`{"request":{"executable":"sh","args":["-c","sleep 5"]}}`), scope)
		done <- err
	}()
	deadline := time.After(time.Second)
	for runs.Load() != 2 {
		select {
		case <-deadline:
			t.Fatal("execution did not start")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expired grant execution completed")
		}
	case <-time.After(time.Second):
		t.Fatal("expired grant execution did not stop")
	}
}
func TestExecutorFailsClosedWhenAuditCannotBeWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	os.WriteFile(path, []byte("{"), 0600)
	audit := NewAudit(path)
	called := false
	executor := NewExecutor(ExecutionOptions{Audit: audit, Run: func(context.Context, Session, ProcessRequest, ExecutionPolicy, ProcessData) (ProcessResult, error) {
		called = true
		return ProcessResult{}, nil
	}})
	manifest := fixturePackage(t).Manifest
	manifest.Permissions = []string{"process.execute"}
	manifest.Execution = &ExecutionPolicy{Programs: []string{"printf"}, Targets: []string{"local"}}
	scope := CapabilityScope{PluginID: manifest.ID, Manifest: manifest, Session: Session{Kind: "local"}, GrantID: "fixture", Context: context.Background(), Authorized: func() bool { return true }}
	if _, err := executor.Request(context.Background(), "process.execute", json.RawMessage(`{"request":{"executable":"printf","args":[]}}`), scope); err == nil || called {
		t.Fatal("unreadable audit did not prevent execution")
	}
}

func TestInteractiveTerminalLeasesFollowGrantAndSource(t *testing.T) {
	audit := NewAudit(filepath.Join(t.TempDir(), "audit.json"))
	opened, closed := make(chan string, 10), make(chan string, 10)
	var count atomic.Int32
	executor := NewExecutor(ExecutionOptions{Audit: audit, OpenTerminal: func(context.Context, uint, Session, ProcessRequest, ExecutionPolicy) (string, error) {
		return "child-" + string(rune('a'+count.Add(1))), nil
	}, CloseTerminal: func(_ uint, id string) { closed <- id }, TerminalOpened: func(_ uint, id string) { opened <- id }})
	manifest := fixturePackage(t).Manifest
	manifest.Permissions = []string{"terminal.create"}
	manifest.Execution = &ExecutionPolicy{Programs: []string{"printf"}, Targets: []string{"local"}}
	lifetime, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := CapabilityScope{PluginID: manifest.ID, Manifest: manifest, Owner: 1, Session: Session{ID: "source", Kind: "local"}, GrantID: strings.Repeat("a", 64), Context: lifetime, Authorized: func() bool { return lifetime.Err() == nil }}
	result, err := executor.Request(context.Background(), "terminal.create", json.RawMessage(`{"request":{"executable":"printf","args":["fixture"],"title":"Fixture Tool"}}`), scope)
	if err != nil {
		t.Fatal(err)
	}
	id := result.(map[string]string)["terminalSessionId"]
	if id == "" || <-opened != id {
		t.Fatal("interactive child was not adopted")
	}
	// Hiding a surface ends its job but does not discard an already-created tool.
	cancel()
	select {
	case <-closed:
		t.Fatal("hiding the source surface closed the user-facing tool")
	default:
	}
	executor.ReleaseGrant(scope.PluginID, scope.GrantID)
	if <-closed != id {
		t.Fatal("revocation did not close the child")
	}
	if _, err = ValidateProcessRequest(json.RawMessage(`{"executable":"printf","args":["\u001b[31m"]}`), true, false); err == nil {
		t.Fatal("interactive arguments accepted terminal control injection")
	}
	request, err := ValidateProcessRequest(json.RawMessage(`{"executable":"printf","args":["safe"],"checks":[{"args":["identity"],"stdout":"expected"}]}`), true, false)
	if err != nil {
		t.Fatal(err)
	}
	command, err := TerminalCommand(request, ExecutionPolicy{}, "/usr/bin/printf")
	if err != nil || !strings.Contains(command, `[ "$aterm_check" = 'expected' ]`) {
		t.Fatal(command, err)
	}
}
