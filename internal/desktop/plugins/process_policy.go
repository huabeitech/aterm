package plugins

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

const ProcessOutputLimit = 131072
const StreamOutputLimit = 24576

type ProcessCheck struct {
	Args   []string `json:"args"`
	Stdout string   `json:"stdout"`
}
type ProcessRequest struct {
	Executable     string             `json:"executable"`
	Args           []string           `json:"args"`
	Env            map[string]*string `json:"env,omitempty"`
	TimeoutMS      int                `json:"timeoutMs,omitempty"`
	Output         string             `json:"output,omitempty"`
	MaxOutputBytes int                `json:"maxOutputBytes,omitempty"`
	Title          string             `json:"title,omitempty"`
	Checks         []ProcessCheck     `json:"checks,omitempty"`
}
type ProcessResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  *int   `json:"exitCode"`
	Truncated *bool  `json:"truncated,omitempty"`
}

func ValidateProcessRequest(data json.RawMessage, terminal, stream bool) (ProcessRequest, error) {
	var r ProcessRequest
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return r, errors.New("invalid capability request")
	}
	allowed := map[string]bool{"executable": true, "args": true, "env": true}
	if !stream {
		allowed["timeoutMs"] = true
	}
	if terminal {
		allowed["title"] = true
		allowed["checks"] = true
	} else {
		allowed["maxOutputBytes"] = true
		if !stream {
			allowed["output"] = true
		}
	}
	for key := range fields {
		if !allowed[key] {
			return r, errors.New("unknown execution request field")
		}
	}
	if json.Unmarshal(data, &r) != nil || !programName.MatchString(r.Executable) {
		return r, errors.New("invalid executable request")
	}
	if err := validateArguments(r.Args); err != nil {
		return r, err
	}
	if raw, exists := fields["timeoutMs"]; !exists || string(raw) == "null" {
		r.TimeoutMS = 30000
	}
	if !stream && (r.TimeoutMS < 100 || r.TimeoutMS > 30000) {
		return r, errors.New("invalid execution timeout")
	}
	if raw, exists := fields["env"]; exists {
		if string(raw) == "null" || r.Env == nil || len(r.Env) > 16 {
			return r, errors.New("invalid execution environment")
		}
		for name, value := range r.Env {
			if !envName.MatchString(name) || (value != nil && (stringLength(*value) > 4096 || hasControls(*value, false))) {
				return r, errors.New("invalid execution environment")
			}
		}
	}
	if !terminal {
		if _, exists := fields["output"]; exists && r.Output != "error" && r.Output != "tail" {
			return r, errors.New("invalid output policy")
		}
		if _, exists := fields["maxOutputBytes"]; exists && (r.MaxOutputBytes < 1024 || r.MaxOutputBytes > ProcessOutputLimit) {
			return r, errors.New("invalid output budget")
		}
		if stream {
			if r.MaxOutputBytes == 0 {
				r.MaxOutputBytes = StreamOutputLimit
			}
			if r.MaxOutputBytes > StreamOutputLimit {
				return r, errors.New("invalid stream output budget")
			}
			r.Output = "tail"
			r.TimeoutMS = 0
		} else if r.MaxOutputBytes == 0 {
			r.MaxOutputBytes = ProcessOutputLimit
		}
	}
	if _, exists := fields["title"]; exists && (!validTitle(r.Title, 100) || hasControls(r.Title, false)) {
		return r, errors.New("invalid terminal title")
	}
	if raw, exists := fields["checks"]; exists {
		if string(raw) == "null" || r.Checks == nil || len(r.Checks) > 4 {
			return r, errors.New("invalid terminal checks")
		}
		var checks []map[string]json.RawMessage
		json.Unmarshal(raw, &checks)
		for i, check := range checks {
			for key := range check {
				if key != "args" && key != "stdout" {
					return r, errors.New("invalid terminal check")
				}
			}
			if _, ok := check["stdout"]; !ok || string(check["stdout"]) == "null" || stringLength(r.Checks[i].Stdout) > 4096 || strings.ContainsAny(r.Checks[i].Stdout, "\x00\r\n") {
				return r, errors.New("invalid terminal check")
			}
			if err := validateArguments(r.Checks[i].Args); err != nil {
				return r, err
			}
		}
	}
	if terminal {
		for _, arg := range r.Args {
			if hasControls(arg, true) {
				return r, errors.New("interactive arguments cannot contain terminal control characters")
			}
		}
		for _, check := range r.Checks {
			for _, arg := range append(append([]string{}, check.Args...), check.Stdout) {
				if hasControls(arg, true) {
					return r, errors.New("interactive arguments cannot contain terminal control characters")
				}
			}
		}
	}
	return r, nil
}
func validateArguments(args []string) error {
	if args == nil || len(args) > 128 {
		return errors.New("invalid executable arguments")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return errors.New("invalid executable arguments")
		}
	}
	data, _ := marshalJSON(args)
	if len(data) > 65536 {
		return errors.New("invalid executable arguments")
	}
	return nil
}
func hasControls(value string, extended bool) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f || (extended && r >= 0x80 && r <= 0x9f) {
			return true
		}
	}
	return false
}
func AuthorizeExecution(manifest Manifest, capability, target string, request ProcessRequest) error {
	policy := manifest.Execution
	if policy == nil || !slices.Contains(manifest.Permissions, capability) || !slices.Contains(policy.Programs, request.Executable) || !slices.Contains(policy.Targets, target) {
		return errors.New("execution capability not granted")
	}
	for name := range request.Env {
		if !slices.Contains(policy.Env, name) {
			return errors.New("execution capability not granted")
		}
	}
	return nil
}
func QuoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func ProgramCommand(request ProcessRequest, policy ExecutionPolicy, executable string) string {
	if executable == "" {
		executable = request.Executable
	}
	parts := []string{"env"}
	unset := append([]string{}, policy.UnsetEnv...)
	keys := make([]string, 0, len(request.Env))
	for name := range request.Env {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	for _, name := range keys {
		if request.Env[name] == nil && !slices.Contains(unset, name) {
			unset = append(unset, name)
		}
	}
	for _, name := range unset {
		parts = append(parts, "-u", name)
	}
	for _, name := range keys {
		if request.Env[name] != nil {
			parts = append(parts, name+"="+*request.Env[name])
		}
	}
	parts = append(parts, executable)
	parts = append(parts, request.Args...)
	for i, value := range parts {
		parts[i] = QuoteShell(value)
	}
	return strings.Join(parts, " ")
}
func TerminalCommand(request ProcessRequest, policy ExecutionPolicy, executable string) (string, error) {
	command := ProgramCommand(request, policy, executable)
	if len(request.Checks) > 0 {
		checks := []string{}
		for _, check := range request.Checks {
			run := ProgramCommand(ProcessRequest{Executable: request.Executable, Args: check.Args, Env: request.Env}, policy, executable)
			checks = append(checks, "aterm_check=$("+run+") && [ \"$aterm_check\" = "+QuoteShell(check.Stdout)+" ]")
		}
		command = "if " + strings.Join(checks, " && ") + "; then " + command + "; else printf '%s\\n' 'Plugin terminal precondition failed. Reconnect the tool before retrying.'; fi"
	}
	if len(command) > 3000 {
		return "", errors.New("interactive command limit exceeded")
	}
	return command, nil
}
