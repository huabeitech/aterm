//go:build pluginacceptance

package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"ssh-connect/internal/desktop/plugins"
)

// The fixture container must be disposable and emit aterm-docker-migration-log.
func verifyDockerAcceptance(ctx context.Context, a *App, window application.Window, sessionID string,
	approvals <-chan plugins.PermissionRequest, await func(func(acceptanceNotice) bool) (acceptanceNotice, error)) error {
	const id = "huabei.docker"
	pkg, err := plugins.InspectArchive(os.Getenv("ATERM_FIXTURE_DOCKER_PACKAGE"))
	if err != nil {
		return err
	}
	if pkg.Manifest.ID != id {
		return fmt.Errorf("expected Docker plugin")
	}
	container, image := os.Getenv("ATERM_FIXTURE_CONTAINER"), os.Getenv("ATERM_FIXTURE_IMAGE")
	if container == "" || image == "" {
		return fmt.Errorf("set disposable fixture container and image IDs")
	}
	if err := a.desktopPluginRegistry.Install(pkg, a.desktopPluginRegistry.Revision()); err != nil {
		return err
	}
	a.desktopPluginManager.Changed(id)
	owner := window.ID()
	if err := a.desktopPluginManager.Visibility(owner, id, sessionID, "docker-passive", true, true); err != nil {
		return err
	}
	if _, err := await(func(n acceptanceNotice) bool { return n.State == "permission-required" }); err != nil {
		return fmt.Errorf("passive availability: %w", err)
	}
	if err := a.desktopPluginManager.Visibility(owner, id, sessionID, "docker-workspace", true, false); err != nil {
		return err
	}
	select {
	case request := <-approvals:
		if request.Plugin.Manifest.ID != id || request.Session.ID != sessionID {
			return fmt.Errorf("authorization scope mismatch")
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if _, err := await(func(n acceptanceNotice) bool { return n.DockerAvailable }); err != nil {
		return fmt.Errorf("Docker availability: %w", err)
	}
	fmt.Println("PASS Docker passive tab availability and per-session authorization")

	invoke := func(command string, args map[string]any) (json.RawMessage, error) {
		input, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		value, err := a.desktopPluginManager.ExecuteCommand(ctx, owner, id, sessionID, command, input)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data struct {
				Error             string
				Data              json.RawMessage
				TerminalSessionID string
			}
		}
		if err := json.Unmarshal(encoded, &result); err != nil {
			return nil, err
		}
		if result.Data.Error != "" {
			return nil, fmt.Errorf("%s: %s", args["operation"], result.Data.Error)
		}
		if result.Data.TerminalSessionID != "" {
			return json.Marshal(result.Data.TerminalSessionID)
		}
		return result.Data.Data, nil
	}
	raw, err := invoke("docker.request", map[string]any{"operation": "connect"})
	if err != nil {
		return err
	}
	var engine struct{ Context, Engine string }
	if err := json.Unmarshal(raw, &engine); err != nil {
		return err
	}
	run := func(operation, target string) (json.RawMessage, error) {
		args := map[string]any{"operation": operation, "context": engine.Context, "engine": engine.Engine}
		if target != "" {
			args["id"] = target
		}
		return invoke("docker.request", args)
	}
	for _, step := range []struct{ operation, target, expect string }{
		{"list", "", container}, {"inspect", container, "RestartPolicy"},
		{"images.list", "", strings.TrimPrefix(image, "sha256:")}, {"images.inspect", image, `"Layers"`},
	} {
		data, err := run(step.operation, step.target)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), step.expect) {
			return fmt.Errorf("%s did not contain fixture data", step.operation)
		}
		fmt.Printf("PASS Docker %s\n", step.operation)
	}
	windowCtx := context.WithValue(ctx, application.WindowKey, window)
	openDockerAcceptanceView(window, id, sessionID)
	view, err := await(func(n acceptanceNotice) bool { return n.Type == "docker-rendered" || n.Type == "view-error" })
	if err != nil {
		return fmt.Errorf("Docker workspace: %w", err)
	}
	if !view.OpaqueOrigin || !view.ParentBlocked || !view.RuntimeAbsent {
		return fmt.Errorf("Docker workspace did not render in its isolated frame: %s", view.Title)
	}
	window.ExecJS(`document.getElementById('docker-acceptance-view')?.remove()`)
	fmt.Println("PASS Docker isolated workspace rendered fixture container")

	stream := map[string]any{"action": "start", "subscription": "docker-acceptance", "id": container, "context": engine.Context, "engine": engine.Engine}
	if _, err := invoke("docker.logs", stream); err != nil {
		return err
	}
	if _, err := await(func(n acceptanceNotice) bool { return n.DockerLog }); err != nil {
		return fmt.Errorf("Docker follow: %w", err)
	}
	stream["action"] = "stop"
	if _, err := invoke("docker.logs", stream); err != nil {
		return err
	}
	fmt.Println("PASS Docker follow and stop")

	raw, err = run("exec", container)
	if err != nil {
		return err
	}
	var terminalID string
	if err := json.Unmarshal(raw, &terminalID); err != nil {
		return err
	}
	if terminalID == "" {
		return fmt.Errorf("Docker exec did not return a terminal session")
	}
	for {
		session, err := a.terminal.Snapshot(terminalID)
		if err != nil {
			return err
		}
		if session.State == "running" {
			break
		}
		if session.State == "exited" {
			return fmt.Errorf("Docker terminal exited while starting")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	input := base64.StdEncoding.EncodeToString([]byte("printf 'aterm-container-%s\\n' ready\r"))
	if err := a.terminal.Write(terminalID, input); err != nil {
		return err
	}
	for {
		output, err := a.TerminalScrollback(terminalID)
		if err != nil {
			return err
		}
		decoded, err := base64.StdEncoding.DecodeString(output)
		if err != nil {
			return err
		}
		if strings.Contains(string(decoded), "aterm-container-ready") {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("container terminal did not respond")
		case <-time.After(50 * time.Millisecond):
		}
	}
	_ = a.CloseTerminal(terminalID)
	fmt.Println("PASS Docker interactive Shell")

	for _, operation := range []string{"stop", "start", "restart", "stop", "remove"} {
		if _, err := run(operation, container); err != nil {
			return err
		}
	}
	if _, err := run("inspect", container); err == nil {
		return fmt.Errorf("disposable container still present after removal")
	}
	fmt.Println("PASS Docker container lifecycle")

	_, config, _, err := a.desktopPluginRegistry.Package(id)
	if err != nil || len(config.Grants) != 1 {
		return fmt.Errorf("expected one Docker grant")
	}
	args, _ := json.Marshal([]string{"revoke", id, config.Grants[0].ID})
	var call []json.RawMessage
	_ = json.Unmarshal(args, &call)
	if _, err := a.desktopPlugins(windowCtx, call); err != nil {
		return err
	}
	if _, err := run("list", ""); err == nil {
		return fmt.Errorf("Docker command succeeded after revocation")
	}
	fmt.Println("PASS Docker revocation")
	if runtime.GOOS == "windows" {
		return verifyWindowsLocalProgram(ctx, a, owner)
	}
	return nil
}

func verifyWindowsLocalProgram(ctx context.Context, a *App, owner uint) error {
	source, err := a.OpenLocalTerminal()
	if err != nil {
		return err
	}
	defer a.CloseTerminal(source.ID)
	session, err := a.resolvePluginSession(owner, source.ID)
	if err != nil {
		return err
	}
	policy := plugins.ExecutionPolicy{Programs: []string{"cmd"}, Targets: []string{"local"}}
	request := plugins.ProcessRequest{Executable: "cmd", Args: []string{"/d", "/c", "echo aterm-local-program"}, Checks: []plugins.ProcessCheck{{Args: []string{"/d", "/c", "echo aterm-local-check"}, Stdout: "wrong"}}}
	if child, err := a.openPluginTerminal(ctx, owner, session, request, policy); err == nil {
		_ = a.CloseTerminal(child)
		return fmt.Errorf("Windows local program accepted a failed precondition")
	}
	request.Checks[0].Stdout = "aterm-local-check"
	child, err := a.openPluginTerminal(ctx, owner, session, request, policy)
	if err != nil {
		return err
	}
	defer a.CloseTerminal(child)
	for {
		output, err := a.TerminalScrollback(child)
		if err != nil {
			return err
		}
		decoded, err := base64.StdEncoding.DecodeString(output)
		if err != nil {
			return err
		}
		if strings.Contains(string(decoded), "aterm-local-program") {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Windows program did not emit ConPTY output")
		case <-time.After(50 * time.Millisecond):
		}
	}
	fmt.Println("PASS Windows local native program and precondition in ConPTY")
	return nil
}

func openDockerAcceptanceView(window application.Window, id, sessionID string) {
	args, _ := json.Marshal([]string{id, sessionID})
	window.ExecJS(fmt.Sprintf(`
	  (async ([id, session]) => {
	    const view = await desktop.plugins.openView(id, session, 'docker.containers')
	    const frame = document.createElement('iframe')
	    frame.id = 'docker-acceptance-view'
	    frame.sandbox = 'allow-scripts'
	    frame.src = view.url
	    addEventListener('message', async function receive(event) {
	      if (event.source !== frame.contentWindow || event.origin !== 'null') return
	      const data = event.data
	      if (data?.type === 'docker-rendered') {
	        removeEventListener('message', receive)
	        acceptance.emit(data)
	        return
	      }
	      if (data?.channel !== 'aterm-view' || data.instanceId !== view.instanceId || data.method !== 'command') return
	      const response = {channel: 'aterm-view', instanceId: view.instanceId, requestId: data.requestId, type: 'response'}
	      try { response.result = (await desktop.plugins.executeCommand(id, session, data.args.command, data.args.args)).data }
	      catch (error) { response.error = String(error) }
	      frame.contentWindow?.postMessage(response, '*')
	    })
	    frame.onload = () => frame.contentWindow.postMessage({
	      channel: 'aterm-view', instanceId: view.instanceId, type: 'state',
	      state: {status: acceptance.status, context: {active: true, invocationId: 'test', sessionId: session, sessionLabel: 'Docker fixture', view: null}, environment: {tokens: {}, locale: 'en', appearance: 'light'}}
	    }, '*')
	    document.body.append(frame)
	  })(%s).catch(error => acceptance.emit({type: 'view-error', title: String(error)}))
	`, args))
}

const dockerViewObserver = `<script>
{
  const observer = new MutationObserver(() => {
    const rows = [...document.querySelectorAll('.container-name')]
    if (!rows.some(row => row.textContent.includes('aterm-docker-migration-'))) return
    observer.disconnect()
    let parentBlocked = false
    try { void parent.document } catch { parentBlocked = true }
    parent.postMessage({
      type: 'docker-rendered', parentBlocked, opaqueOrigin: self.origin === 'null',
      runtimeAbsent: typeof window._wails === 'undefined',
    }, '*')
  })
  observer.observe(document.documentElement, {childList: true, subtree: true})
}
</script>`
