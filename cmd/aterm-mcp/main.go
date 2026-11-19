package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ssh-connect/internal/agentbridge"
)

func main() {
	path := flag.String("socket", "", "aTerm Unix socket")
	flag.Parse()
	if *path == "" {
		log.Fatal("missing --socket")
	}
	client := http.Client{Timeout: 2 * time.Minute, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", *path)
	}}}
	call := func(ctx context.Context, input agentbridge.Request) (*mcp.CallToolResult, agentbridge.Result, error) {
		var out agentbridge.Result
		data, err := json.Marshal(input)
		if err != nil {
			return nil, out, err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/request", bytes.NewReader(data))
		if err != nil {
			return nil, out, err
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, out, errors.New("APP_UNAVAILABLE")
		}
		defer res.Body.Close()
		err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&out)
		if out.Error != "" {
			err = errors.New(out.Error)
		}
		return nil, out, err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "aterm", Version: "1"}, nil)
	type Search struct {
		Query string `json:"query,omitempty"`
	}
	type Open struct {
		HostID string `json:"hostId"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "list_hosts", Description: "List saved aTerm SSH hosts."}, func(ctx context.Context, _ *mcp.CallToolRequest, in Search) (*mcp.CallToolResult, agentbridge.Result, error) {
		return call(ctx, agentbridge.Request{Method: "list", Query: in.Query})
	})
	mcp.AddTool(server, &mcp.Tool{Name: "prepare_ssh", Description: "Prepare a local OpenSSH invocation for a saved host. Execute the returned program with its argument array and environment. For password authentication use a user-controlled interactive terminal. Never request or relay a password in chat."}, func(ctx context.Context, _ *mcp.CallToolRequest, in Open) (*mcp.CallToolResult, agentbridge.Result, error) {
		return call(ctx, agentbridge.Request{Method: "prepare", HostID: in.HostID})
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
