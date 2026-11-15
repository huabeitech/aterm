package app

import (
	"context"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type referenceTestWindow struct {
	application.Window
	id uint
}

func (w *referenceTestWindow) ID() uint { return w.id }

func TestMainWindowReplacementRejectsStaleOwnerAndClose(t *testing.T) {
	app := &App{}
	old := &referenceTestWindow{id: 1}
	next := &referenceTestWindow{id: 2}
	app.setWindow(old)
	oldRequest := context.WithValue(context.Background(), application.WindowKey, application.Window(old))
	captured, err := app.requirePluginOwner(oldRequest)
	if err != nil || captured != old {
		t.Fatal("initial owner rejected", err)
	}
	app.setWindow(next)
	if captured.ID() != 1 {
		t.Fatal("in-flight snapshot was retargeted")
	}
	if app.clearMainWindow(old.ID()) {
		t.Fatal("stale close cleared a replacement")
	}
	if _, err := app.requirePluginOwner(oldRequest); err == nil {
		t.Fatal("old renderer retained plugin access")
	}
	nextRequest := context.WithValue(context.Background(), application.WindowKey, application.Window(next))
	if window, err := app.requirePluginOwner(nextRequest); err != nil || window != next {
		t.Fatal("new renderer rejected", err)
	}
	if !app.clearMainWindow(next.ID()) || app.mainWindow() != nil {
		t.Fatal("current close did not clear reference")
	}
}
func TestMainWindowReferenceConcurrentPublicationAndSnapshots(t *testing.T) {
	app := &App{}
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			for iteration := 0; iteration < 500; iteration++ {
				window := &referenceTestWindow{id: uint(index*500 + iteration + 1)}
				app.setWindow(window)
				snapshot := app.mainWindow()
				if snapshot != nil && snapshot.ID() == 0 {
					t.Error("invalid window snapshot")
				}
				app.clearMainWindow(window.ID())
			}
		}(worker)
	}
	group.Wait()
}
