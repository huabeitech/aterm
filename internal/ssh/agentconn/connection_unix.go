//go:build !windows

package agentconn

import (
	"context"
	"net"
)

func dialEndpoint(ctx context.Context, endpoint string, _ bool) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
