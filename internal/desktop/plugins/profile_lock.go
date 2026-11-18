package plugins

import (
	"errors"
	"os"
	"path/filepath"
)

type ProfileLock struct{ file *os.File }

func LockProfile(root string) (*ProfileLock, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, "runtime.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	ok, err := lockProfileFile(f)
	if err != nil || !ok {
		f.Close()
		if err == nil {
			err = errors.New("plugin profile is already in use")
		}
		return nil, err
	}
	return &ProfileLock{f}, nil
}
func (l *ProfileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockProfileFile(l.file)
	err := l.file.Close()
	l.file = nil
	return err
}
