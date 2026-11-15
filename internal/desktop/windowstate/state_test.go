package windowstate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestPlacementRecoversRemovedDisplayAndPreservesSecondaryDisplay(t *testing.T) {
	primary := Rect{X: 0, Y: 25, Width: 1440, Height: 875}
	secondary := Rect{X: -1920, Y: 0, Width: 1920, Height: 1080}
	saved := &State{Rect: Rect{X: -1800, Y: 100, Width: 1200, Height: 800}}
	if got := Fit(saved, []Rect{primary, secondary}); got != saved.Rect {
		t.Fatal("secondary placement changed", got)
	}
	got := Fit(saved, []Rect{primary})
	if got.X != 0 || got.Y != 100 || got.Width != 1200 || got.Height != 800 {
		t.Fatal("removed display left window offscreen", got)
	}
	got = Fit(nil, []Rect{primary})
	if got != (Rect{X: 130, Y: 73, Width: 1180, Height: 780}) {
		t.Fatal("default placement differs from Electron", got)
	}
	for _, raw := range []string{`null`, `{}`, `{"x":0,"y":0,"width":1200,"height":800}`, `{"x":0,"y":0,"width":1200,"height":800,"maximized":null}`, `{"x":0,"y":0,"width":-1,"height":800,"maximized":false}`} {
		if Decode(raw) != nil {
			t.Fatal("corrupt geometry accepted", raw)
		}
	}
}
func TestTrackerRetainsNormalBoundsAndIgnoresTransientStates(t *testing.T) {
	initial := State{Rect: Rect{X: 40, Y: 60, Width: 1180, Height: 780}}
	var saved []State
	tracker := New(initial, func(state State) error { saved = append(saved, state); return nil })
	normal := Rect{X: 100, Y: 120, Width: 1250, Height: 820}
	if err := tracker.Observe(normal, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(Rect{Width: 1920, Height: 1080}, true, false, false); err != nil {
		t.Fatal(err)
	}
	if last := saved[len(saved)-1]; last.Rect != normal || !last.Maximized {
		t.Fatal("maximized frame replaced normal geometry", last)
	}
	_ = tracker.Observe(Rect{}, false, true, false)
	_ = tracker.Observe(Rect{Width: 1920, Height: 1080}, false, false, true)
	if len(saved) != 2 {
		t.Fatal("minimized/fullscreen frame was persisted")
	}
	if err := tracker.Close(); err != nil {
		t.Fatal(err)
	}
	count := len(saved)
	_ = tracker.Observe(initial.Rect, false, false, false)
	_ = tracker.Close()
	if len(saved) != count {
		t.Fatal("closed tracker wrote again")
	}
}
func TestTrackerSerializesSaveAndClose(t *testing.T) {
	tracker := New(State{Rect: Rect{Width: 1180, Height: 780}}, func(state State) error {
		if !Valid(state) {
			t.Error("invalid state persisted")
		}
		return nil
	})
	var work sync.WaitGroup
	for i := 0; i < 30; i++ {
		work.Add(1)
		go func(i int) {
			defer work.Done()
			_ = tracker.Observe(Rect{X: i, Y: i, Width: 1180, Height: 780}, false, false, false)
		}(i)
	}
	work.Add(1)
	go func() { defer work.Done(); _ = tracker.Close() }()
	work.Wait()
}
func TestElectronWindowPlacementInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron placement oracle")
	}
	type vector struct {
		Saved    *State `json:"saved"`
		Displays []Rect `json:"displays"`
	}
	vectors := []vector{{Displays: []Rect{}}, {Displays: []Rect{{Width: 1440, Height: 900}}}, {Displays: []Rect{{X: 20, Y: 30, Width: 800, Height: 600}}}}
	for i := 0; i < 100; i++ {
		vectors = append(vectors, vector{Saved: &State{Rect: Rect{X: i*77 - 3000, Y: i*17 - 1000, Width: 760 + i*10, Height: 540 + i*7}}, Displays: []Rect{{Y: 25, Width: 1440, Height: 875}, {X: -1920, Width: 1920, Height: 1080}, {X: 1440, Y: 200, Width: 2560, Height: 1440}}})
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "vectors.json")
	data, _ := json.Marshal(vectors)
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "placement.mts")
	code := `import {readFileSync} from 'node:fs';import {pathToFileURL} from 'node:url';const {fitWindow}=await import(pathToFileURL(process.argv[2]).href);const vectors=JSON.parse(readFileSync(process.argv[3],'utf8'));process.stdout.write(JSON.stringify(vectors.map(v=>fitWindow(v.saved,v.displays))));`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, filepath.Join(reference, "src/main/window/bounds.ts"), input).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron bounds oracle: %v: %s", err, output)
	}
	var expected []Rect
	if err = json.Unmarshal(output, &expected); err != nil || len(expected) != len(vectors) {
		t.Fatal("invalid oracle output", err)
	}
	for i, vector := range vectors {
		if got := Fit(vector.Saved, vector.Displays); got != expected[i] {
			t.Fatalf("placement differs at %d: Go=%+v Electron=%+v", i, got, expected[i])
		}
	}
}
