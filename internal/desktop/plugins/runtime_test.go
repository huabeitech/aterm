package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRuntimeBindsIdentityAndCompletesCommand(t *testing.T) {
	var host *RuntimeHost
	called := false
	host = NewRuntimeHost(func(control RuntimeControl) {
		switch control.Kind {
		case "start":
			if _, err := host.Handle(context.Background(), control.InstanceID, "runtime.ready", json.RawMessage(`{}`)); err != nil {
				t.Error(err)
			}
		case "command":
			data, _ := json.Marshal(control.Context)
			var commandContext map[string]json.RawMessage
			json.Unmarshal(data, &commandContext)
			response, _ := json.Marshal(map[string]any{"callId": json.RawMessage(commandContext["callId"]), "value": map[string]string{"answer": "fixture"}})
			host.Handle(context.Background(), control.InstanceID, "runtime.commandResult", response)
		}
	}, func(ctx context.Context, id, method string, params json.RawMessage) (any, error) {
		called = id == "example.monitor" && method == "storage.get"
		return "fixture", nil
	})
	id, err := host.Start(context.Background(), "example.monitor", fixturePackage(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = host.Handle(context.Background(), "invented", "storage.get", json.RawMessage(`{}`)); err == nil {
		t.Fatal("accepted unbound runtime")
	}
	if _, err = host.Handle(context.Background(), id, "storage.get", json.RawMessage(`{"pluginId":"another.plugin"}`)); err != nil || !called {
		t.Fatal("request was not bound to runtime identity", err)
	}
	value, err := host.Command(context.Background(), id, "session-token", "fixture", json.RawMessage(`{"hello":1}`))
	if err != nil || string(value) != `{"answer":"fixture"}` {
		t.Fatal(string(value), err)
	}
	if _, err = host.Handle(context.Background(), id, "runtime.commandResult", json.RawMessage(`{"callId":"stale","value":null}`)); err == nil {
		t.Fatal("accepted stale response")
	}
	host.Stop(id)
	if _, err = host.Handle(context.Background(), id, "runtime.ready", json.RawMessage(`{}`)); err == nil {
		t.Fatal("stopped runtime still accepts requests")
	}
	host.Close()
	if _, err = host.Start(context.Background(), "closed", fixturePackage(t), nil); err == nil {
		t.Fatal("closed host started runtime")
	}
}
func TestRuntimeActivationCancellationAndLimits(t *testing.T) {
	controls := make(chan RuntimeControl, 20)
	host := NewRuntimeHost(func(c RuntimeControl) { controls <- c }, nil)
	host.activationTimeout = 10 * time.Millisecond
	if _, err := host.Start(context.Background(), "timeout", fixturePackage(t), nil); err == nil {
		t.Fatal("activation did not time out")
	}
	start := <-controls
	<-controls
	if _, err := host.Handle(context.Background(), start.InstanceID, "runtime.ready", json.RawMessage(`{}`)); err == nil {
		t.Fatal("late ready reactivated runtime")
	}
	host.activationTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := host.Start(ctx, "cancel", fixturePackage(t), nil); done <- err }()
	start = <-controls
	if _, err := host.Handle(context.Background(), start.InstanceID, "storage.get", json.RawMessage(`{}`)); err == nil {
		t.Fatal("capability accessible before ready")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-controls
}
func TestRuntimeStopRejectsPendingCommandsAndFaults(t *testing.T) {
	controls := make(chan RuntimeControl, 20)
	host := NewRuntimeHost(func(c RuntimeControl) { controls <- c }, nil)
	started := make(chan string, 1)
	faulted := make(chan string, 1)
	go func() {
		id, err := host.Start(context.Background(), "example.monitor", fixturePackage(t), func(reason string) { faulted <- reason })
		if err != nil {
			t.Error(err)
		}
		started <- id
	}()
	control := <-controls
	host.Handle(context.Background(), control.InstanceID, "runtime.ready", json.RawMessage(`{}`))
	id := <-started
	done := make(chan error, 1)
	go func() {
		_, err := host.Command(context.Background(), id, "token", "test", json.RawMessage(`null`))
		done <- err
	}()
	<-controls
	host.Handle(context.Background(), id, "runtime.fault", json.RawMessage(`{}`))
	if err := <-done; err == nil {
		t.Fatal("pending command survived stop")
	}
	if reason := <-faulted; reason == "" {
		t.Fatal("missing fault notification")
	}
}
func TestRuntimeRejectsOversizedAndExcessRequests(t *testing.T) {
	var host *RuntimeHost
	host = NewRuntimeHost(func(c RuntimeControl) {
		if c.Kind == "start" {
			host.Handle(context.Background(), c.InstanceID, "runtime.ready", json.RawMessage(`{}`))
		}
	}, nil)
	id, err := host.Start(context.Background(), "fixture", fixturePackage(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = host.Handle(context.Background(), id, "storage.get", json.RawMessage(strings.Repeat(" ", 262145))); err == nil {
		t.Fatal("accepted oversized request")
	}
	rejected := false
	for i := 0; i < 60; i++ {
		if _, err = host.Handle(context.Background(), id, "runtime.heartbeat", json.RawMessage(`{}`)); err != nil {
			rejected = true
		}
	}
	if !rejected {
		t.Fatal("runtime request rate unbounded")
	}
	host.Close()
}
