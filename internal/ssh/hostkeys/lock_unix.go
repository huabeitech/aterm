//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package hostkeys

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func tryNativeLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
		return false, nil
	}
	return err == nil, err
}
func unlockNative(file *os.File) { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }
