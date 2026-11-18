package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	sdk "ssh-connect/packages/plugin-go"
	"testing"
)

type monitorHost struct {
	store  json.RawMessage
	writes int
	fail   bool
}

func (h *monitorHost) Open(context.Context, string, any) (net.Conn, error) {
	return nil, fmt.Errorf("unused")
}
func (h *monitorHost) Call(_ context.Context, method string, args any, out any) error {
	var value any
	switch method {
	case "process.execute":
		code := 0
		value = sdk.ProcessResult{Stdout: probe(100, 10, 90, 1000, 2000), ExitCode: &code}
	case "storage.get":
		if h.store == nil {
			h.store = sdk.JSON(nil)
		}
		value = h.store
	case "storage.set":
		if h.fail {
			return fmt.Errorf("write failed")
		}
		var p struct{ Value json.RawMessage }
		json.Unmarshal(sdk.JSON(args), &p)
		h.store = p.Value
		h.writes++
	case "status.publish":
	default:
		return fmt.Errorf("unexpected %s", method)
	}
	if out != nil {
		return json.Unmarshal(sdk.JSON(value), out)
	}
	return nil
}
func TestPreferencePersistenceAcrossSessionScopesAndFailedSave(t *testing.T) {
	host := &monitorHost{}
	b := newBackend(&sdk.SessionRuntime{Host: host})
	ctx := context.Background()
	scope := sdk.SessionInfo{Token: "one", SessionID: "session", HostKey: "host", Interval: 5}
	if e := b.Refresh(ctx, scope); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Command(ctx, scope, "monitor.select", sdk.JSON(map[string]string{"network": "eth0"})); e != nil {
		t.Fatal(e)
	}
	host.fail = true
	if _, e := b.Command(ctx, scope, "monitor.select", sdk.JSON(map[string]any{"network": nil})); e == nil {
		t.Fatal("failed preference write was hidden")
	}
	if b.tokens["one"].preference.Network != "eth0" {
		t.Fatal("failed save changed selection")
	}
	host.fail = false
	b.history["session"].history[0].At -= 1000
	b.Release("one")
	scope.Token = "two"
	if e := b.Refresh(ctx, scope); e != nil {
		t.Fatal(e)
	}
	sample := b.tokens["two"]
	if sample.preference.Network != "eth0" || host.writes != 1 {
		t.Fatal("reopen lost saved preference")
	}
	if len(sample.history) != 3 || sample.history[1].CPU != nil || sample.resources.Network[0].ReceiveRate != nil {
		t.Fatal("reopen joined disconnected sample baselines")
	}
}
