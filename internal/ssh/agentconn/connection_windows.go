package agentconn

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func dialEndpoint(ctx context.Context, endpoint string, automatic bool) (net.Conn, error) {
	if strings.EqualFold(endpoint, "pageant") {
		return connectPageant()
	}
	if strings.HasPrefix(strings.ToLower(endpoint), `\\.\pipe\`) || strings.HasPrefix(strings.ToLower(endpoint), `\\?\pipe\`) {
		connection, err := winio.DialPipeContext(ctx, endpoint)
		if automatic && (errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)) {
			return connectPageant()
		}
		return connection, err
	}
	// Honor an explicit AF_UNIX endpoint, including Windows installations using
	// a custom agent rather than the built-in OpenSSH named pipe.
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
