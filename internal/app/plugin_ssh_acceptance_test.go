//go:build pluginacceptance

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/service"
	"ssh-connect/internal/terminal"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == plugins.WorkerArgument {
		if err := plugins.ServeWorker(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--run-plugin-acceptance" {
		if path := os.Getenv("ATERM_FIXTURE_LOG"); path != "" {
			log, err := os.Create(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Stdout, os.Stderr = log, log
			defer log.Close()
		}
		if err := checkAcceptanceEnvironment(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

type acceptanceNotice struct {
	Type            string
	State           string
	Metric          string
	Title           string
	Value           string
	OpaqueOrigin    bool
	ParentBlocked   bool
	RuntimeAbsent   bool
	Items           int
	History         int
	CPUValid        bool
	Network         string
	Selected        string
	Sections        int
	Trends          int
	DockerAvailable bool
	DockerLog       bool
}

// Run with an isolated ATERM_DATA_DIR, a fixture SSH identity and the
// two packages produced by scripts/plugin-prototype/build.mjs --ssh.
func checkAcceptanceEnvironment() error {
	if os.Getenv("ATERM_FIXTURE_DOCKER_PACKAGE") != "" && os.Getenv("ATERM_FIXTURE_HOST") == "" {
		if !filepath.IsAbs(os.Getenv("ATERM_DATA_DIR")) || !filepath.IsAbs(os.Getenv("ATERM_FIXTURE_ASSETS")) {
			return fmt.Errorf("set absolute fixture profile and assets paths")
		}
		return runDesktopPluginAcceptance(0)
	}
	for _, name := range []string{"ATERM_DATA_DIR", "ATERM_FIXTURE_KEY", "ATERM_FIXTURE_PACKAGES", "ATERM_FIXTURE_ASSETS"} {
		if !filepath.IsAbs(os.Getenv(name)) {
			return fmt.Errorf("set %s to an absolute path", name)
		}
	}
	if os.Getenv("ATERM_FIXTURE_HOST") == "" || os.Getenv("ATERM_FIXTURE_FINGERPRINT") == "" {
		return fmt.Errorf("set ATERM_FIXTURE_HOST and ATERM_FIXTURE_FINGERPRINT")
	}
	port, err := strconv.Atoi(os.Getenv("ATERM_FIXTURE_PORT"))
	if err != nil {
		return err
	}
	return runDesktopPluginAcceptance(port)
}

func runDesktopPluginAcceptance(port int) error {
	a := NewApp()
	notices := make(chan acceptanceNotice, 64)
	var verifyFullView atomic.Bool
	assets := application.AssetFileServerFS(os.DirFS(os.Getenv("ATERM_FIXTURE_ASSETS")))
	app := application.New(application.Options{
		Name:     "aTerm Plugin SSH Acceptance",
		Services: []application.Service{application.NewService(a)},
		Assets: application.AssetOptions{Handler: assets, Middleware: func(next http.Handler) http.Handler {
			desktop := a.desktopAssetMiddleware(next)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dockerView := os.Getenv("ATERM_FIXTURE_DOCKER_PACKAGE") != ""
				if (verifyFullView.Load() || dockerView) && strings.HasPrefix(r.URL.Path, plugins.ViewPath) {
					record := httptest.NewRecorder()
					desktop.ServeHTTP(record, r)
					for key, values := range record.Header() {
						w.Header()[key] = values
					}
					w.WriteHeader(record.Code)
					_, _ = w.Write(record.Body.Bytes())
					if record.Code == http.StatusOK {
						observer := fullResourceViewObserver
						if dockerView {
							observer = dockerViewObserver
						}
						_, _ = fmt.Fprint(w, observer)
					}
					return
				}
				if r.URL.Path != "/__acceptance" || r.Method != "POST" {
					desktop.ServeHTTP(w, r)
					return
				}
				var notice acceptanceNotice
				if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&notice); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				select {
				case notices <- notice:
				default:
				}
				w.WriteHeader(204)
			})
		}},
	})
	a.setWailsApp(app)
	window, err := a.createMainWindow()
	if err != nil {
		return err
	}
	result := make(chan error, 1)
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		go func() {
			err := verifyDesktopPluginAcceptance(a, window, notices, port, &verifyFullView)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if os.Getenv("ATERM_FIXTURE_DOCKER_PACKAGE") != "" {
				fmt.Println("PASS Docker desktop acceptance")
			} else {
				fmt.Println("PASS same desktop host, Linux SSH, revoke and two package versions")
			}
			result <- nil
			app.Quit()
		}()
	})
	if err := app.Run(); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	default:
		return fmt.Errorf("app closed before acceptance")
	}
}

func verifyDesktopPluginAcceptance(a *App, window application.Window, notices <-chan acceptanceNotice, port int, verifyFullView *atomic.Bool) error {
	const id = "huabei.resource-monitor"
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	approvals := make(chan plugins.PermissionRequest)
	a.desktopPluginManager.Close()
	a.desktopPluginViews = plugins.NewViewHost()
	if !a.securePluginFrames(window) {
		return fmt.Errorf("native plugin frame isolation unavailable")
	}
	a.desktopPluginManager = a.newDesktopPluginManager(a.ctx, filepath.Join(os.Getenv("ATERM_DATA_DIR"), "plugins"), func(ctx context.Context, request plugins.PermissionRequest) bool {
		select {
		case approvals <- request:
			return true
		case <-ctx.Done():
			return false
		}
	})
	entry, err := openAcceptanceSession(ctx, a, port)
	if err != nil {
		return err
	}
	defer a.CloseTerminal(entry.ID)
	for {
		session, err := a.terminal.Snapshot(entry.ID)
		if err != nil {
			return err
		}
		if session.State == "running" {
			break
		}
		if session.State == "exited" {
			output, _ := a.TerminalScrollback(entry.ID)
			return fmt.Errorf("SSH exited before opening: %s", output)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("SSH did not connect: %s", session.State)
		case <-time.After(50 * time.Millisecond):
		}
	}
	window.ExecJS(`
		(async () => {
		  while (!window.desktop) await new Promise(resolve => setTimeout(resolve, 20))
		  const emit = value => fetch('/__acceptance', {method: 'POST', body: JSON.stringify(value)})
		  window.acceptance = {emit}
		  window.desktop.plugins.onStatus(async status => {
		    window.acceptance.status = status
		    const {resources, history, preference} = status.data ?? {}
		    await emit({
		      type: 'status', state: status.state, value: status.items?.[0]?.value, title: status.items?.[0]?.label,
		      items: status.items?.length, history: history?.length, cpuValid: typeof resources?.cpu?.usage === 'number',
		      network: resources?.network?.[0]?.name, selected: preference?.network,
		      dockerAvailable: status.views?.['docker.containers']?.available,
		      dockerLog: status.data?.dockerLogs?.text.includes("aterm-docker-migration-log"),
		    })
		  })
		  await emit({type: 'listening'})
		})()
	`)
	await := func(match func(acceptanceNotice) bool) (acceptanceNotice, error) {
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case notice := <-notices:
				if match(notice) {
					return notice, nil
				}
			case <-timer.C:
				return acceptanceNotice{}, fmt.Errorf("timed out waiting for WebView report")
			case <-ctx.Done():
				return acceptanceNotice{}, ctx.Err()
			}
		}
	}
	if _, err := await(func(n acceptanceNotice) bool { return n.Type == "listening" }); err != nil {
		return err
	}

	if os.Getenv("ATERM_FIXTURE_DOCKER_PACKAGE") != "" {
		return verifyDockerAcceptance(ctx, a, window, entry.ID, approvals, await)
	}
	consumer := "acceptance"
	for i, version := range []string{"0.0.1", "0.0.2"} {
		pkg, err := plugins.InspectArchive(filepath.Join(os.Getenv("ATERM_FIXTURE_PACKAGES"), "resource-monitor-"+version+".aterm-plugin"))
		if err != nil {
			return err
		}
		if err := a.desktopPluginRegistry.Install(pkg, a.desktopPluginRegistry.Revision()); err != nil {
			return err
		}
		a.desktopPluginManager.Changed(id)
		_, config, _, err := a.desktopPluginRegistry.Package(id)
		if err != nil {
			return err
		}
		if len(config.Grants) != 0 {
			return fmt.Errorf("%s inherited execution permissions", version)
		}
		if err := a.desktopPluginManager.Visibility(window.ID(), id, entry.ID, consumer, true, false); err != nil {
			return err
		}
		select {
		case request := <-approvals:
			if request.Session.ID != entry.ID || request.Plugin.Manifest.Version != version {
				return fmt.Errorf("wrong authorization scope")
			}
		case <-time.After(10 * time.Second):
			return fmt.Errorf("authorization not requested")
		}
		label := []string{"Total memory", "Available memory"}[i]
		title := []string{"Memory overview", "Memory availability"}[i]
		notice, err := await(func(n acceptanceNotice) bool { return n.Type == "status" && n.State == "ready" && n.Title == label })
		if err != nil {
			return fmt.Errorf("%s sample: %w", version, err)
		}
		if !strings.HasSuffix(notice.Value, " MiB") || notice.Value == "0 MiB" {
			return fmt.Errorf("invalid SSH sample %q", notice.Value)
		}
		openAcceptanceView(window, id, entry.ID)
		ui, err := await(func(n acceptanceNotice) bool { return n.Type == "rendered" || n.Type == "view-error" })
		if err != nil {
			return fmt.Errorf("%s view: %w", version, err)
		}
		if ui.Title != title || ui.Metric != notice.Value || !ui.OpaqueOrigin || !ui.ParentBlocked || !ui.RuntimeAbsent {
			return fmt.Errorf("unexpected %s view: %+v", version, ui)
		}
		fmt.Printf("PASS %s: authorized Linux SSH returned %s, isolated detail rendered %q\n", version, notice.Value, title)

		if i == 0 {
			continue
		} // Upgrade while the first version still has a grant.
		_, cfg, _, err := a.desktopPluginRegistry.Package(id)
		if err != nil || len(cfg.Grants) != 1 {
			return fmt.Errorf("missing active permission")
		}
		raw, _ := json.Marshal([]string{"revoke", id, cfg.Grants[0].ID})
		var call []json.RawMessage
		_ = json.Unmarshal(raw, &call)
		if _, err := a.desktopPlugins(context.WithValue(ctx, application.WindowKey, window), call); err != nil {
			return err
		}
		if _, err := await(func(n acceptanceNotice) bool { return n.Type == "status" && n.State == "permission-required" }); err != nil {
			// Re-subscribe just as a newly mounted view would, so the denial
			// state must still be visible after revoking.
			if err := a.desktopPluginManager.Visibility(window.ID(), id, entry.ID, consumer, true, false); err != nil {
				return err
			}
			if _, err := await(func(n acceptanceNotice) bool { return n.State == "permission-required" }); err != nil {
				return err
			}
		}
		silence := time.NewTimer(6 * time.Second)
		defer silence.Stop()
		for {
			select {
			case <-silence.C:
				if os.Getenv("ATERM_FIXTURE_FULL_PACKAGE") == "" {
					return nil
				}
				verifyFullView.Store(true)
				return verifyFullResourceMonitor(ctx, a, window, entry.ID, approvals, await)
			case notice := <-notices:
				if notice.State == "ready" {
					return fmt.Errorf("sampling continued after revocation")
				}
			case <-approvals:
				return fmt.Errorf("revocation created an unsolicited authorization")
			}
		}
	}
	return nil
}

func verifyFullResourceMonitor(
	ctx context.Context, a *App, window application.Window, sessionID string,
	approvals <-chan plugins.PermissionRequest, await func(func(acceptanceNotice) bool) (acceptanceNotice, error),
) error {
	pkg, err := plugins.InspectArchive(os.Getenv("ATERM_FIXTURE_FULL_PACKAGE"))
	if err != nil {
		return err
	}
	id := pkg.Manifest.ID
	if err := a.desktopPluginRegistry.Install(pkg, a.desktopPluginRegistry.Revision()); err != nil {
		return err
	}
	a.desktopPluginManager.Changed(id)
	select {
	case request := <-approvals:
		if request.Plugin.Manifest.Version != pkg.Manifest.Version || request.Session.ID != sessionID {
			return fmt.Errorf("unexpected full plugin approval scope")
		}
	case <-time.After(10 * time.Second):
		return fmt.Errorf("full plugin did not request authorization")
	}
	status, err := await(func(n acceptanceNotice) bool {
		return n.Type == "status" && n.State == "ready" && n.Items == 5 && n.CPUValid && n.History >= 2
	})
	if err != nil {
		return fmt.Errorf("full resource monitor: %w", err)
	}
	if status.Network == "" {
		return fmt.Errorf("full resource monitor reported no network interfaces")
	}
	args, _ := json.Marshal(map[string]string{"network": status.Network})
	if _, err := a.desktopPluginManager.ExecuteCommand(ctx, window.ID(), id, sessionID, "monitor.select", args); err != nil {
		return err
	}
	if _, err := await(func(n acceptanceNotice) bool {
		return n.Type == "status" && n.State == "ready" && n.Selected == status.Network
	}); err != nil {
		return err
	}
	openAcceptanceView(window, id, sessionID)
	view, err := await(func(n acceptanceNotice) bool { return n.Type == "full-rendered" || n.Type == "view-error" })
	if err != nil {
		return fmt.Errorf("full resource view: %w", err)
	}
	if view.Sections != 5 || view.Trends != 4 || !view.OpaqueOrigin || !view.ParentBlocked || !view.RuntimeAbsent {
		return fmt.Errorf("unexpected full resource view: %+v", view)
	}
	fmt.Printf("PASS %s: full resource probe published five items, second CPU sample and history, and accepted the network selection command, and rendered five detail sections and four charts\n", pkg.Manifest.Version)
	return nil
}

func openAcceptanceView(window application.Window, id, sessionID string) {
	args, _ := json.Marshal([]string{id, sessionID})
	window.ExecJS(fmt.Sprintf(`
			(async ([id, session]) => {
			  const view = await desktop.plugins.openView(id, session, 'monitor.details')
			  document.getElementById('plugin-acceptance-view')?.remove()
			  const frame = document.createElement('iframe')
			  frame.id = 'plugin-acceptance-view'
			  frame.sandbox = 'allow-scripts'
			  frame.src = view.url
			  addEventListener('message', function report(event) {
			    if (event.source !== frame.contentWindow || event.origin !== 'null' || event.data?.channel !== 'resource-monitor-acceptance') return
			    removeEventListener('message', report)
			    acceptance.emit(event.data)
			  })
			  frame.onload = () => frame.contentWindow.postMessage({
			    channel: 'aterm-view', instanceId: view.instanceId, type: 'state',
			    state: {status: acceptance.status, context: {invocationId: 'test', sessionId: session, sessionLabel: 'Linux fixture', view: {section: 'cpu'}}, environment: {tokens: {}, locale: 'en', appearance: 'light'}}
			  }, '*')
			  document.body.append(frame)
			})(%s).catch(error => acceptance.emit({type: 'view-error', title: String(error)}))
		`, args))
}

const fullResourceViewObserver = `<script>
{
  const observer = new MutationObserver(() => {
    const sections = document.querySelectorAll('[data-resource-section]').length
    const trends = document.querySelectorAll('.resource-trend').length
    if (sections !== 5 || trends !== 4) return
    observer.disconnect()
    let parentBlocked = false
    try { void parent.document } catch { parentBlocked = true }
    parent.postMessage({
      channel: 'resource-monitor-acceptance', type: 'full-rendered', sections, trends,
      parentBlocked, opaqueOrigin: self.origin === 'null', runtimeAbsent: typeof window._wails === 'undefined',
    }, '*')
  })
  observer.observe(document.documentElement, {childList: true, subtree: true})
}
</script>`

func openAcceptanceSession(ctx context.Context, a *App, port int) (terminal.Session, error) {
	if port == 0 {
		return a.OpenLocalTerminal()
	}
	alias := fmt.Sprintf("linux-plugin-fixture-%d", time.Now().UnixNano())
	host, err := a.CreateHost(service.HostInput{Alias: alias, HostName: os.Getenv("ATERM_FIXTURE_HOST"), User: "probe", Port: port, IdentityFile: os.Getenv("ATERM_FIXTURE_KEY")})
	if err != nil {
		return terminal.Session{}, err
	}
	return a.terminal.OpenSSH(alias, terminal.LaunchOptions{}, func(ctx context.Context, _ string) (*service.NativeSSHClient, service.Host, error) {
		return a.svc.OpenNativeSSHWithHostKeyConfirmer(ctx, host.Alias, func(_ context.Context, key service.HostKeyInfo) (bool, error) {
			return slices.Contains(strings.Split(os.Getenv("ATERM_FIXTURE_FINGERPRINT"), ","), key.Fingerprint), nil
		})
	})
}
