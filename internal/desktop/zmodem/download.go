package zmodem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type diskDownload struct {
	mu                     sync.Mutex
	ctx                    context.Context
	file                   *os.File
	temporary, destination string
	finished               bool
}

// NewDownload stages beside the chosen destination. Link publishes without
// replacement, including when another process creates the target after selection.
func NewDownload(ctx context.Context, destination string) (Download, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(destination) {
		return nil, errors.New("choose an absolute ZMODEM download path")
	}
	if _, err := os.Lstat(destination); err == nil {
		return nil, errors.New("a local file already exists; choose a new name for the ZMODEM download")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".aterm-*.zmodem")
	if err != nil {
		return nil, err
	}
	if err = file.Chmod(0600); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, err
	}
	return &diskDownload{ctx: ctx, file: file, temporary: file.Name(), destination: destination}, nil
}
func (d *diskDownload) Write(data []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return 0, os.ErrClosed
	}
	if err := d.ctx.Err(); err != nil {
		return 0, err
	}
	return d.file.Write(data)
}
func (d *diskDownload) Commit() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return os.ErrClosed
	}
	if err := d.ctx.Err(); err != nil {
		return err
	}
	if err := d.file.Sync(); err != nil {
		return err
	}
	if err := d.file.Close(); err != nil {
		return err
	}
	if err := d.ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(d.temporary, d.destination); err != nil {
		return err
	}
	d.finished = true
	return os.Remove(d.temporary)
}
func (d *diskDownload) Abort() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return nil
	}
	d.finished = true
	_ = d.file.Close()
	err := os.Remove(d.temporary)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
