package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
)

type UploadConflict struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	LocalKind  string    `json:"localKind"`
	RemoteKind string    `json:"remoteKind"`
	LocalSize  int64     `json:"localSize"`
	RemoteSize int64     `json:"remoteSize"`
	LocalTime  time.Time `json:"localTime"`
	RemoteTime time.Time `json:"remoteTime"`
	Multiple   bool      `json:"multiple"`
}
type UploadChoice struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	All    bool   `json:"all"`
}
type UploadBatch struct {
	mu       sync.Mutex
	multiple bool
	policy   string
	decided  chan struct{}
}

func NewUploadBatch(multiple bool) *UploadBatch {
	return &UploadBatch{multiple: multiple, decided: make(chan struct{})}
}
func fileKind(info os.FileInfo) string {
	if info.IsDir() {
		return "folder"
	}
	if info.Mode().IsRegular() {
		return "file"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "link"
	}
	return "special"
}
func (m *Transfers) ResolveConflict(session, id string, choice UploadChoice) error {
	task, err := m.task(session, id)
	if err != nil {
		return err
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	c := task.info.Conflict
	if c == nil || c.ID != choice.ID || task.answer == nil {
		return nil
	}
	regular := c.LocalKind == "file" && c.RemoteKind == "file"
	if choice.Action != "skip" && !(choice.Action == "overwrite" && regular) {
		return errors.New("invalid upload conflict choice")
	}
	if choice.All && c.Multiple && regular {
		b := task.batch
		b.mu.Lock()
		if b.policy == "" {
			b.policy = choice.Action
			close(b.decided)
		}
		b.mu.Unlock()
	}
	task.answer <- choice.Action
	task.answer = nil
	return nil
}

func (m *Transfers) choose(ctx context.Context, task *transferTask, c UploadConflict) (string, error) {
	b := task.batch
	regular := c.LocalKind == "file" && c.RemoteKind == "file"
	var decided <-chan struct{}
	task.mu.Lock()
	if regular {
		b.mu.Lock()
		policy := b.policy
		decided = b.decided
		b.mu.Unlock()
		if policy != "" {
			task.mu.Unlock()
			return policy, nil
		}
	}
	if task.info.State == "paused" {
		task.mu.Unlock()
		if err := task.waitReady(ctx); err != nil {
			return "", err
		}
		return m.choose(ctx, task, c)
	}
	answer := make(chan string, 1)
	task.answer = answer
	c.ID = uuid.NewString()
	task.info.Conflict = &c
	task.info.State = "conflict"
	started := time.Now()
	m.send(task.info)
	task.mu.Unlock()
	<-m.slots
	task.slotHeld = false
	var action string
	select {
	case action = <-answer:
	case <-decided:
		b.mu.Lock()
		action = b.policy
		b.mu.Unlock()
		// Honor a decision submitted for this file before another task chose All.
		select {
		case action = <-answer:
		default:
		}
	case <-ctx.Done():
	}
	select {
	case m.slots <- struct{}{}:
		task.slotHeld = true
	case <-ctx.Done():
	}
	task.mu.Lock()
	task.answer = nil
	task.info.Conflict = nil
	task.info.State = "running"
	task.started = task.started.Add(time.Since(started))
	if ctx.Err() == nil {
		m.send(task.info)
	}
	task.mu.Unlock()
	return action, ctx.Err()
}

func (m *Transfers) uploadTree(ctx context.Context, task *transferTask, client *sftp.Client, info Transfer, entries []treeEntry) error {
	for i := 0; i < len(entries); {
		e := entries[i]
		local := filepath.Join(info.LocalPath, filepath.FromSlash(e.relative))
		remote := path.Join(info.RemotePath, e.relative)
		skipped, err := m.uploadEntry(ctx, task, client, local, remote, info.RemotePath, task.batch.multiple || entries[0].directory)
		if err != nil {
			return fmt.Errorf("%s: %w", remote, err)
		}
		i++
		task.mu.Lock()
		if skipped {
			before := task.info.Skipped
			if !e.directory {
				task.info.Skipped++
				task.info.Total -= e.size
			} else {
				for i < len(entries) && (e.relative == "" || strings.HasPrefix(entries[i].relative, e.relative+"/")) {
					if !entries[i].directory {
						task.info.Total -= entries[i].size
						task.info.Skipped++
					}
					i++
				}
			}
			if task.info.Skipped == before {
				task.info.Skipped++
			}
		} else if !e.directory {
			task.info.Uploaded++
		}
		m.send(task.info)
		task.mu.Unlock()
	}
	return nil
}

func (m *Transfers) uploadEntry(ctx context.Context, task *transferTask, client *sftp.Client, local, remote, root string, multiple bool) (bool, error) {
	src, err := os.Lstat(local)
	if err != nil {
		return false, err
	}
	if !src.IsDir() && !src.Mode().IsRegular() {
		return false, errors.New("unsupported upload source")
	}
	// A merged directory can change while a conflict prompt is open.
	checkParents := func() error {
		for parent := path.Dir(remote); remote != root && (parent == root || strings.HasPrefix(parent, root+"/")); parent = path.Dir(parent) {
			entry, err := client.Lstat(parent)
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return fmt.Errorf("upload parent is no longer a directory: %s", parent)
			}
		}
		return nil
	}
	temp := ""
	defer func() {
		if temp != "" && checkParents() == nil {
			client.Remove(temp)
		}
	}()
	for {
		if err := task.waitReady(ctx); err != nil {
			return false, err
		}
		if err := checkParents(); err != nil {
			return false, err
		}
		dst, err := client.Lstat(remote)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if dst != nil {
			if src.IsDir() && dst.IsDir() {
				return false, nil
			}
			choice, err := m.choose(ctx, task, UploadConflict{Path: remote, LocalKind: fileKind(src), RemoteKind: fileKind(dst), LocalSize: src.Size(), RemoteSize: dst.Size(), LocalTime: src.ModTime(), RemoteTime: dst.ModTime(), Multiple: multiple})
			if err != nil {
				return false, err
			}
			if choice == "skip" {
				if temp != "" {
					task.mu.Lock()
					task.info.Transferred -= src.Size()
					task.mu.Unlock()
				}
				return true, nil
			}
		}
		if src.IsDir() {
			err := client.Mkdir(remote)
			if err != nil {
				if _, statErr := client.Lstat(remote); statErr == nil {
					continue
				}
			}
			return false, err
		}
		if temp == "" {
			if err := checkParents(); err != nil {
				return false, err
			}
			source, err := os.Open(local)
			if err != nil {
				return false, err
			}
			name := path.Join(path.Dir(remote), ".aterm-"+uuid.NewString())
			dest, err := client.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
			if err != nil {
				source.Close()
				return false, err
			}
			temp = name
			if dst != nil {
				if err = dest.Chmod(dst.Mode().Perm()); err != nil {
					source.Close()
					dest.Close()
					return false, err
				}
			}
			if err = m.copy(ctx, task, dest, source); err != nil {
				return false, err
			}
		}
		if err := task.waitReady(ctx); err != nil {
			return false, err
		}
		if err := checkParents(); err != nil {
			return false, err
		}
		latest, err := client.Lstat(remote)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if latest == nil {
			err = client.Rename(temp, remote)
			if err != nil {
				if _, statErr := client.Lstat(remote); statErr == nil {
					continue
				}
			}
		} else if dst == nil || latest.Mode() != dst.Mode() || latest.Size() != dst.Size() || !latest.ModTime().Equal(dst.ModTime()) {
			continue
		} else {
			if err = client.Chmod(temp, latest.Mode().Perm()); err != nil {
				return false, err
			}
			err = client.PosixRename(temp, remote)
		}
		if err == nil {
			temp = ""
		}
		return false, err
	}
}
