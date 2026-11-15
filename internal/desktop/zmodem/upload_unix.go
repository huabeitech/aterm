//go:build !windows

package zmodem

import (
	"os"
	"syscall"
)

// O_NONBLOCK prevents a file-to-FIFO replacement race from blocking selection.
// It has no effect on regular files; the caller verifies the opened descriptor.
func openUpload(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
