//go:build darwin && agentacceptance && !mas

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run only against the disposable loopback parity fixture.
func TestMain(m *testing.M) {
	if len(os.Args) != 2 || os.Args[1] != "--agent-acceptance" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("ATERM_DATA_DIR")
	rel, e := filepath.Rel(os.TempDir(), profile)
	if e != nil || strings.Contains(rel, "/") || !strings.HasPrefix(rel, "at-agent.") {
		panic("isolated fixture profile required")
	}
	a := NewApp()
	app := application.New(application.Options{Name: "aTerm Agent Acceptance", Services: []application.Service{application.NewServiceWithOptions(a, application.ServiceOptions{Name: "App"})}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(os.DirFS(os.Getenv("ATERM_FIXTURE_ASSETS"))), Middleware: a.desktopAssetMiddleware}})
	a.setWailsApp(app)
	w, e := a.createMainWindow()
	if e != nil {
		panic(e)
	}
	w.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		go func() {
			deadline := time.Now().Add(3 * time.Minute)
			hosts, e := a.ListHosts()
			if e != nil {
				panic(e)
			}
			config := agentConfig{Enabled: true, Hosts: map[string]bool{}}
			for _, h := range hosts {
				if h.HostName != "127.0.0.1" {
					panic("loopback required")
				}
				if h.Alias == "parity-jump" {
					config.Hosts[h.ID] = true
				}
			}
			if len(config.Hosts) != 1 {
				panic("fixture host missing")
			}
			payload, _ := json.Marshal(config)
			if _, e = a.agentDesktop("save", []json.RawMessage{payload}); e != nil {
				panic(e)
			}
			for time.Now().Before(deadline) {
				w.ExecJS(`(()=>{const buttons=[...document.querySelectorAll('button')];const allow=buttons.find(b=>['Allow','允许','允許'].includes(b.textContent.trim()));if(allow&&document.body.innerText.includes('127.0.0.1'))allow.click();const input=document.querySelector('.sync-password-dialog input[type=password]');if(input&&!input.value){Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,'fixture-vault-password');input.dispatchEvent(new Event('input',{bubbles:true}));const unlock=buttons.find(b=>['Unlock','解锁','解鎖'].includes(b.textContent.trim()));unlock?.click()}})()`)
				for _, id := range a.terminal.IDs() {
					ss, _ := a.terminal.Snapshot(id)
					if ss.State == "running" {
						status, e := a.svc.GetEncryptedSyncStatus(context.Background())
						if e != nil || !status.Unlocked {
							panic("vault still locked")
						}
						fmt.Println("PASS SSH running with unlocked fixture vault")
						return
					}
				}
				time.Sleep(250 * time.Millisecond)
			}
			panic("timed out waiting for SSH")
		}()
	})
	if e = app.Run(); e != nil {
		panic(e)
	}
}
