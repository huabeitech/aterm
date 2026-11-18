// A second, deliberately unrelated extension validates the generic protocol.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	sdk "ssh-connect/packages/plugin-go"
)

type tool struct{ host sdk.Host }

func (t *tool) Call(ctx context.Context, method string, args json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "runtime.status":
		return sdk.JSON(map[string]int{"tasks": 0}), nil
	case "runtime.shutdown":
		return sdk.JSON(nil), nil
	case "tool.inspect":
		var hosts json.RawMessage
		err := t.host.Call(ctx, "hosts.list", nil, &hosts)
		return sdk.JSON(map[string]any{"extension": "connection-inspector", "hosts": hosts}), err
	}
	return nil, errors.New("unsupported command")
}
func (t *tool) Open(context.Context, string, json.RawMessage) (io.ReadWriteCloser, error) {
	return nil, errors.New("unsupported stream")
}
func main() { sdk.Serve(func(host sdk.Host) sdk.Handler { return &tool{host} }) }
