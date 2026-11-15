package terminalmeta

import (
	"context"
	"io"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	sshutil "ssh-connect/internal/ssh"
)

const directoryScript = `
parent=$(ps -o ppid= -p $$) || exit 1
pids=$(ps -axo pid=,ppid=,tty= | awk -v parent="$parent" '$2 == parent+0 && $3 != "?" && $3 != "??" && $3 != "-" { print $1 }')
set -- $pids
[ "$#" -eq 1 ] || exit 1
if [ -d /proc/"$1"/cwd ]; then
  readlink /proc/"$1"/cwd
else
  lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n "s/^n//p"
fi
`

var RemoteDirectoryCommand = "exec sh -c '" + strings.ReplaceAll(directoryScript, "'", "'\\''") + "'"

func ParseRemoteDirectory(output string) string {
	path := strings.TrimSuffix(output, "\n")
	if len(path) != len(output) {
		path = strings.TrimSuffix(path, "\r")
	}
	if !strings.HasPrefix(path, "/") || !utf8.ValidString(path) || len(utf16.Encode([]rune(path))) > 8192 || strings.ContainsAny(path, "\x00\r\n") {
		return ""
	}
	return path
}
func ReadRemoteDirectory(ctx context.Context, client sshutil.SessionFactory) string {
	bounded, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	session, err := sshutil.OpenSession(bounded, client)
	if err != nil {
		return ""
	}
	defer session.Close()
	stop := context.AfterFunc(bounded, func() { session.Close() })
	defer stop()
	output := &boundedBuffer{limit: 64 * 1024, cancel: cancel}
	session.Stdout = output
	session.Stderr = io.Discard
	if err = session.Run(RemoteDirectoryCommand); err != nil || bounded.Err() != nil {
		return ""
	}
	return ParseRemoteDirectory(output.String())
}
