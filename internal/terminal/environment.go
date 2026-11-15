package terminal

import "strings"

func terminalEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "ELECTRON_RUN_AS_NODE", "NODE_OPTIONS", "NODE_CHANNEL_FD", "ATERM_USER_DATA_DIR", "ATERM_DATA_DIR", "TERM", "COLORTERM", "TERM_PROGRAM":
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=aTerm")
}
