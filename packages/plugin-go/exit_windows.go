//go:build windows

package plugin

import "os"

// The host's kill-on-close Job Object owns this process and its descendants.
func exitWithChildren() { os.Exit(0) }
