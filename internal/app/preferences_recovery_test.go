package app

import (
	"encoding/json"
	"reflect"
	"ssh-connect/internal/desktop"
	"sync"
	"testing"
)

func TestCorruptDesktopPreferencesRecoverAndNotifyBootstrap(t *testing.T) {
	invalid := desktop.DefaultPreferences()
	invalid["theme"] = "nonexistent"
	data, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"{broken", "null", "[]", string(data)} {
		t.Run(value, func(t *testing.T) {
			app := bridgeFixture(t)
			if err := app.store.SetAppMetadata(desktopPreferencesKey, value); err != nil {
				t.Fatal(err)
			}
			if err := app.store.SetAppMetadata("unrelated", "keep"); err != nil {
				t.Fatal(err)
			}
			// Menu/startup can load preferences before renderer bootstrap.
			got, err := app.desktopPreferences()
			if err != nil || !reflect.DeepEqual(got, desktop.DefaultPreferences()) {
				t.Fatal(got, err)
			}
			bootstrap, err := app.desktopBootstrap()
			if err != nil {
				t.Fatal(err)
			}
			notice := bootstrap.(map[string]any)["recoveryNotice"]
			if notice == nil || notice == "" {
				t.Fatal("recovery not reported to renderer")
			}
			if got, err := app.store.GetAppMetadata("unrelated"); err != nil || got != "keep" {
				t.Fatal(got, err)
			}
			// A new application instance sees the recovered record, not another error.
			next := &App{store: app.store}
			bootstrap, err = next.desktopBootstrap()
			if err != nil {
				t.Fatal(err)
			}
			if bootstrap.(map[string]any)["recoveryNotice"] != nil {
				t.Fatal("valid recovered preferences triggered another recovery")
			}
		})
	}
}
func TestPreferenceStorageFailureIsNotHiddenAsRecovery(t *testing.T) {
	app := bridgeFixture(t)
	if err := app.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.desktopBootstrap(); err == nil {
		t.Fatal("closed storage failure hidden by defaults")
	}
	if app.desktopPreferencesRecovered.Load() {
		t.Fatal("storage failure marked as preference recovery")
	}
}

func TestConcurrentPreferenceReadsRecoverOnce(t *testing.T) {
	app := bridgeFixture(t)
	if err := app.store.SetAppMetadata(desktopPreferencesKey, "broken"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for index := 0; index < 16; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			got, err := app.desktopPreferences()
			if err != nil || !reflect.DeepEqual(got, desktop.DefaultPreferences()) {
				t.Errorf("unexpected preferences: %v %v", got, err)
			}
		}()
	}
	workers.Wait()
	if !app.desktopPreferencesRecovered.Load() {
		t.Fatal("recovery notice missing")
	}
}
