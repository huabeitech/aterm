// The standalone prototype uses the production package parser and registry.
// It registers no Wails services and never loads production user data.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"time"

	"github.com/dop251/goja"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
	"ssh-connect/internal/desktop/plugins"
)

const (
	pluginID        = "huabei.resource-monitor"
	snapshotCommand = "monitor.snapshot"
)

type sampleRequest struct {
	Source string
	Raw    string
}

type sampleResult struct {
	PID   int `json:"pid"`
	Value int `json:"value"`
}

func sampleWorker() error {
	var input sampleRequest
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 2<<20)).Decode(&input); err != nil {
		return err
	}
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	if _, err := vm.RunString(input.Source); err != nil {
		return err
	}
	sample, ok := goja.AssertFunction(vm.Get("sample"))
	if !ok {
		return errors.New("missing sample function")
	}
	value, err := sample(goja.Undefined(), vm.ToValue(input.Raw))
	if err != nil {
		return err
	}
	var result sampleResult
	if err := vm.ExportTo(value, &result); err != nil {
		return err
	}
	result.PID = os.Getpid()
	return json.NewEncoder(os.Stdout).Encode(result)
}

func sample(source, raw string) (sampleResult, error) {
	var result sampleResult
	executable, err := os.Executable()
	if err != nil {
		return result, err
	}
	input, err := json.Marshal(sampleRequest{Source: source, Raw: raw})
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "worker")
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return result, fmt.Errorf("worker exited: %s", exit.Stderr)
		}
		return result, err
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, err
	}
	if result.PID <= 0 || result.PID == os.Getpid() {
		return result, errors.New("sample did not come from an independent process")
	}
	return result, nil
}

type result struct {
	Title         string `json:"title"`
	Metric        string `json:"metric"`
	ParentBlocked bool   `json:"parentBlocked"`
	OpaqueOrigin  bool   `json:"opaqueOrigin"`
	RuntimeAbsent bool   `json:"runtimeAbsent"`
}

type page struct {
	Version  string       `json:"version"`
	Snapshot sampleResult `json:"snapshot"`
	URL      string       `json:"url"`
	Command  string       `json:"command"`
}

type fixture struct {
	version string
	title   string
	value   int
}

var versions = []fixture{
	{version: "0.0.1", title: "Memory overview", value: 8192},
	{version: "0.0.2", title: "Memory availability", value: 3072},
}

// Both versions receive the same Linux input, so a changed result exercises
// the installed package's parser rather than a change in host data.
const meminfo = "MemTotal:        8388608 kB\nMemAvailable:    3145728 kB\n"

//go:embed host.html
var hostHTML string

func run(paths []string) error {
	root, err := os.MkdirTemp("", "aterm-plugin-prototype-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	registry, err := plugins.NewRegistry(plugins.RegistryOptions{Root: root})
	if err != nil {
		return err
	}
	token := uuid.NewString()
	mux := http.NewServeMux()
	reports := make(chan result, 1)
	current := make(chan page)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		plugins.ServePluginDocument(w, r, hostHTML, "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-src 'self'; base-uri 'none'; form-action 'none'")
	})
	mux.HandleFunc("GET /next", func(w http.ResponseWriter, r *http.Request) {
		select {
		case next := <-current:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(next)
		case <-r.Context().Done():
		case <-time.After(15 * time.Second):
			http.Error(w, "No sample", http.StatusRequestTimeout)
		}
	})
	mux.HandleFunc("GET /detail/{token}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("token") != token {
			http.NotFound(w, r)
			return
		}
		pkg, _, _, err := registry.Package(pluginID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		plugins.ServePluginDocument(w, r, pkg.Views["ui/details.html"], plugins.ViewCSP)
	})
	mux.HandleFunc("POST /result", func(w http.ResponseWriter, r *http.Request) {
		var report result
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&report); err != nil {
			http.Error(w, "Invalid report", http.StatusBadRequest)
			return
		}
		select {
		case reports <- report:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "Unexpected report", http.StatusConflict)
		}
	})

	app := application.New(application.Options{
		Name: "aTerm Plugin Prototype",
		Assets: application.AssetOptions{
			Middleware:     func(_ http.Handler) http.Handler { return mux },
			DisableLogging: true,
		},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "aTerm Plugin Prototype",
		Width:  760,
		Height: 460,
	})
	done := make(chan error, 1)
	go func() {
		err := verify(registry, paths, current, reports, token)
		// Wails may exit before run's deferred cleanup on macOS.
		if cleanupErr := os.RemoveAll(root); err == nil {
			err = cleanupErr
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("PASS: both plugin versions ran inside the same host process")
		done <- nil
		app.Quit()
	}()
	if err := app.Run(); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	default:
		return errors.New("prototype closed before verification completed")
	}
}

func verify(registry *plugins.Registry, paths []string, current chan<- page, reports <-chan result, token string) error {
	for i, expected := range versions {
		pkg, err := plugins.InspectArchive(paths[i])
		if err != nil {
			return err
		}
		if pkg.Manifest.ID != pluginID || pkg.Manifest.Version != expected.version {
			return fmt.Errorf("expected %s %s", pluginID, expected.version)
		}
		if err := registry.Install(pkg, registry.Revision()); err != nil {
			return err
		}
		installed, _, _, err := registry.Package(pluginID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(installed.Manifest.Contributes.Commands, func(command plugins.Command) bool {
			return command.ID == snapshotCommand
		}) {
			return errors.New("missing snapshot command")
		}
		snapshot, err := sample(installed.Source, meminfo)
		if err != nil {
			return err
		}
		if snapshot.Value != expected.value {
			return fmt.Errorf("version %s worker returned %d MiB, expected %d", expected.version, snapshot.Value, expected.value)
		}
		select {
		case current <- page{
			Version:  expected.version,
			Snapshot: snapshot,
			URL:      "/detail/" + token + "?version=" + expected.version,
			Command:  snapshotCommand,
		}:
		case <-time.After(15 * time.Second):
			return errors.New("WebView did not request a sample")
		}
		select {
		case report := <-reports:
			if report.Title != expected.title || report.Metric != strconv.Itoa(expected.value)+" MiB" ||
				!report.ParentBlocked || !report.OpaqueOrigin || !report.RuntimeAbsent {
				return fmt.Errorf("version %s unexpected WebView result: %+v", expected.version, report)
			}
			fmt.Printf("PASS %s: host PID %d, worker PID %d, detail %q, %s, opaque origin, parent DOM blocked, Wails runtime absent, snapshot command verified\n",
				expected.version, os.Getpid(), snapshot.PID, report.Title, report.Metric)
		case <-time.After(15 * time.Second):
			return fmt.Errorf("version %s WebView did not report a render", expected.version)
		}
	}
	return nil
}

func main() {
	var err error
	switch {
	case len(os.Args) == 2 && os.Args[1] == "worker":
		err = sampleWorker()
	case len(os.Args) == 4 && os.Args[1] == "verify":
		err = run(os.Args[2:])
	default:
		err = errors.New("usage: plugin-prototype verify <v1.aterm-plugin> <v2.aterm-plugin>")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
