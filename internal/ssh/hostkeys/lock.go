package hostkeys

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Lock a stable sibling file, not the trust document's inode: atomic rename
// replaces that inode. The operating system releases this lock on process exit.
func lockStore(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("invalid SSH host trust lock file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("invalid SSH host trust lock file")
	}
	if err = file.Chmod(0600); err != nil {
		return nil, err
	}
	for {
		locked, err := tryNativeLock(file)
		if err != nil {
			return nil, err
		}
		if locked {
			if err = ctx.Err(); err != nil {
				unlockNative(file)
				return nil, err
			}
			failed = false
			var once sync.Once
			return func() { once.Do(func() { unlockNative(file); file.Close() }) }, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
