package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Transfer struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"sessionID"`
	Direction      string  `json:"direction"`
	Name           string  `json:"name"`
	RemotePath     string  `json:"remotePath"`
	LocalPath      string  `json:"localPath"`
	State          string  `json:"state"`
	Transferred    int64   `json:"transferred"`
	Total          int64   `json:"total"`
	BytesPerSecond float64 `json:"bytesPerSecond"`
	Error          string  `json:"error,omitempty"`
	Directory      bool    `json:"directory,omitempty"`
}
type transferTask struct {
	mu        sync.Mutex
	info      Transfer
	ctx       context.Context
	cancel    context.CancelFunc
	executing bool
	attempt   uint64
	closers   []io.Closer
	started   time.Time
	notified  time.Time
}
type transferJob struct {
	task    *transferTask
	attempt uint64
}

// Transfers owns a bounded FIFO queue independently of the view. A worker owns
// each task's I/O; cancellation closes only its streams, never the shared SFTP client.
type Transfers struct {
	mu      sync.Mutex
	tasks   map[string]*transferTask
	files   *Manager
	emit    func(any)
	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan transferJob
	workers sync.WaitGroup
}

func NewTransfers(files *Manager, emit func(any)) *Transfers {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Transfers{tasks: map[string]*transferTask{}, files: files, emit: emit, ctx: ctx, cancel: cancel, queue: make(chan transferJob, 1000)}
	for i := 0; i < 2; i++ {
		manager.workers.Add(1)
		go manager.worker()
	}
	return manager
}
func (m *Transfers) send(info Transfer) {
	if m.emit != nil {
		m.emit(map[string]any{"type": "transfer", "transfer": info})
	}
}
func (m *Transfers) removed(ids []string) {
	if len(ids) > 0 && m.emit != nil {
		m.emit(map[string]any{"type": "transfers-removed", "ids": ids})
	}
}
func portableName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("unsupported transfer filename %q", name)
	}
	for _, r := range name {
		if r < 32 {
			return errors.New("transfer filename contains a control character")
		}
	}
	return nil
}
func (m *Transfers) Queue(sessionID, direction, local, remote string) (Transfer, error) {
	if _, err := m.files.Session(sessionID); err != nil {
		return Transfer{}, err
	}
	if direction != "upload" && direction != "download" {
		return Transfer{}, errors.New("invalid transfer direction")
	}
	remote, err := absolute(remote)
	if err != nil {
		return Transfer{}, err
	}
	if !filepath.IsAbs(local) {
		return Transfer{}, errors.New("local transfer path must be absolute")
	}
	name := path.Base(remote)
	if err := portableName(name); err != nil {
		return Transfer{}, err
	}
	ctx, cancel := context.WithCancel(m.ctx)
	task := &transferTask{info: Transfer{ID: uuid.NewString(), SessionID: sessionID, Direction: direction, Name: name, RemotePath: remote, LocalPath: filepath.Clean(local), State: "queued"}, ctx: ctx, cancel: cancel, attempt: 1}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		cancel()
		return Transfer{}, errors.New("transfer manager is closed")
	}
	if len(m.tasks) >= 1000 {
		cancel()
		return Transfer{}, errors.New("remove completed transfers before adding more")
	}
	m.tasks[task.info.ID] = task
	// Emit queued before a worker can emit running.
	task.mu.Lock()
	m.send(task.info)
	select {
	case m.queue <- transferJob{task: task, attempt: 1}:
		result := task.info
		task.mu.Unlock()
		return result, nil
	default:
		task.mu.Unlock()
		delete(m.tasks, task.info.ID)
		cancel()
		return Transfer{}, errors.New("transfer queue is full")
	}
}
func (m *Transfers) task(sessionID, id string) (*transferTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task := m.tasks[id]
	if task == nil || task.info.SessionID != sessionID {
		return nil, errors.New("transfer was not found in this session")
	}
	return task, nil
}
func (m *Transfers) Cancel(sessionID, id string) error {
	task, err := m.task(sessionID, id)
	if err != nil {
		return err
	}
	task.mu.Lock()
	task.cancel()
	closers := append([]io.Closer(nil), task.closers...)
	if task.info.State == "queued" {
		task.info.State = "cancelled"
		m.send(task.info)
	}
	task.mu.Unlock()
	for _, closer := range closers {
		_ = closer.Close()
	}
	return nil
}
func (m *Transfers) Retry(sessionID, id string) error {
	if _, err := m.files.Session(sessionID); err != nil {
		return err
	}
	task, err := m.task(sessionID, id)
	if err != nil {
		return err
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.executing || (task.info.State != "failed" && task.info.State != "cancelled") {
		return errors.New("only stopped failed or cancelled transfers can be retried")
	}
	task.ctx, task.cancel = context.WithCancel(m.ctx)
	task.attempt++
	task.info.State = "queued"
	task.info.Transferred = 0
	task.info.Total = 0
	task.info.BytesPerSecond = 0
	task.info.Error = ""
	m.send(task.info)
	select {
	case m.queue <- transferJob{task: task, attempt: task.attempt}:
		return nil
	default:
		task.cancel()
		task.info.State = "failed"
		task.info.Error = "transfer queue is full"
		m.send(task.info)
		return errors.New(task.info.Error)
	}
}
func (m *Transfers) ClearCompleted(sessionID string) {
	m.mu.Lock()
	ids := []string{}
	for id, task := range m.tasks {
		task.mu.Lock()
		if task.info.SessionID == sessionID && task.info.State == "completed" && !task.executing {
			delete(m.tasks, id)
			ids = append(ids, id)
		}
		task.mu.Unlock()
	}
	m.mu.Unlock()
	m.removed(ids)
}
func (m *Transfers) Remove(sessionID, id string) error {
	m.mu.Lock()
	task := m.tasks[id]
	if task == nil || task.info.SessionID != sessionID {
		m.mu.Unlock()
		return errors.New("transfer was not found")
	}
	task.mu.Lock()
	if task.executing || task.info.State == "queued" || task.info.State == "running" {
		task.mu.Unlock()
		m.mu.Unlock()
		return errors.New("cancel the transfer before removing it")
	}
	delete(m.tasks, id)
	task.mu.Unlock()
	m.mu.Unlock()
	m.removed([]string{id})
	return nil
}
func (m *Transfers) CompletedDownload(sessionID, id string) (string, error) {
	task, err := m.task(sessionID, id)
	if err != nil {
		return "", err
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.info.State != "completed" || task.info.Direction != "download" {
		return "", errors.New("download is not complete")
	}
	return task.info.LocalPath, nil
}
func (m *Transfers) CloseSession(sessionID string) {
	m.mu.Lock()
	ids := []string{}
	for id, task := range m.tasks {
		if task.info.SessionID == sessionID {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Cancel(sessionID, id)
	}
}
func (m *Transfers) Close() {
	m.cancel()
	m.mu.Lock()
	tasks := make([]*transferTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	m.mu.Unlock()
	for _, task := range tasks {
		_ = m.Cancel(task.info.SessionID, task.info.ID)
	}
}
func (m *Transfers) worker() {
	defer m.workers.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case job := <-m.queue:
			task := job.task
			task.mu.Lock()
			if task.attempt != job.attempt || task.info.State != "queued" || task.ctx.Err() != nil {
				task.mu.Unlock()
				continue
			}
			task.executing = true
			task.info.State = "running"
			task.started = time.Now()
			task.notified = task.started
			info := task.info
			ctx := task.ctx
			m.send(info)
			task.mu.Unlock()
			actual, err := m.run(ctx, task, info)
			task.mu.Lock()
			task.executing = false
			task.closers = nil
			if ctx.Err() != nil {
				task.info.State = "cancelled"
				task.info.Error = ""
			} else if err != nil {
				task.info.State = "failed"
				task.info.Error = err.Error()
			} else {
				task.info.State = "completed"
				task.info.Total = task.info.Transferred
				task.info.LocalPath = actual
			}
			task.info.BytesPerSecond = 0
			m.send(task.info)
			task.mu.Unlock()
		}
	}
}
func (m *Transfers) progress(task *transferTask, bytes, total int64, directory bool) {
	task.mu.Lock()
	defer task.mu.Unlock()
	task.info.Transferred += bytes
	if total >= 0 {
		task.info.Total = total
		task.info.Directory = directory
	}
	elapsed := time.Since(task.started).Seconds()
	if elapsed > 0 {
		task.info.BytesPerSecond = float64(task.info.Transferred) / elapsed
	}
	if total >= 0 || time.Since(task.notified) >= 100*time.Millisecond {
		task.notified = time.Now()
		m.send(task.info)
	}
}
func (m *Transfers) copy(ctx context.Context, task *transferTask, destination io.WriteCloser, source io.ReadCloser) error {
	task.mu.Lock()
	task.closers = []io.Closer{source, destination}
	task.mu.Unlock()
	defer func() { source.Close(); destination.Close(); task.mu.Lock(); task.closers = nil; task.mu.Unlock() }()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, err := destination.Write(buffer[:count])
			m.progress(task, int64(written), -1, false)
			if err != nil {
				return err
			}
			if written != count {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return destination.Close()
		}
		if readErr != nil {
			return readErr
		}
	}
}

type treeEntry struct {
	relative  string
	directory bool
	size      int64
	mode      os.FileMode
}

func (m *Transfers) run(ctx context.Context, task *transferTask, info Transfer) (string, error) {
	session, err := m.files.Session(info.SessionID)
	if err != nil {
		return "", err
	}
	client := session.Client
	upload := info.Direction == "upload"
	entries := []treeEntry{}
	var total int64
	var walk func(string, int) error
	walk = func(relative string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(entries) >= 10000 || depth > 64 {
			return errors.New("folder transfer exceeds 10000 items or 64 levels")
		}
		var stat os.FileInfo
		var err error
		if upload {
			stat, err = os.Lstat(filepath.Join(info.LocalPath, filepath.FromSlash(relative)))
		} else {
			stat, err = client.Lstat(path.Join(info.RemotePath, relative))
		}
		if err != nil {
			return err
		}
		if stat.Mode()&os.ModeSymlink != 0 || (!stat.IsDir() && !stat.Mode().IsRegular()) {
			return errors.New("symbolic links and special files cannot be transferred")
		}
		entries = append(entries, treeEntry{relative: relative, directory: stat.IsDir(), size: stat.Size(), mode: stat.Mode().Perm()})
		if !stat.IsDir() {
			total += stat.Size()
			return nil
		}
		names := []string{}
		if upload {
			children, err := os.ReadDir(filepath.Join(info.LocalPath, filepath.FromSlash(relative)))
			if err != nil {
				return err
			}
			for _, child := range children {
				names = append(names, child.Name())
			}
		} else {
			children, err := client.ReadDir(path.Join(info.RemotePath, relative))
			if err != nil {
				return err
			}
			for _, child := range children {
				names = append(names, child.Name())
			}
		}
		for _, name := range names {
			if err := portableName(name); err != nil {
				return err
			}
			if err := walk(path.Join(relative, name), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("", 0); err != nil {
		return "", err
	}
	directory := entries[0].directory
	m.progress(task, 0, total, directory)
	// Stage beside the destination. Existing content is never opened for truncation.
	suffix := ".aterm-" + uuid.NewString()
	remoteTemp := path.Join(path.Dir(info.RemotePath), suffix)
	localTemp := filepath.Join(filepath.Dir(info.LocalPath), suffix)
	if upload {
		if _, err := client.Lstat(info.RemotePath); err == nil {
			return "", errors.New("remote destination already exists")
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if directory {
			if err := client.Mkdir(remoteTemp); err != nil {
				return "", err
			}
			defer func() { remaining := 10001; _ = removeTree(client, remoteTemp, 0, &remaining) }()
		} else {
			defer client.Remove(remoteTemp)
		}
	} else {
		if directory {
			if err := os.Mkdir(localTemp, 0o700); err != nil {
				return "", err
			}
			defer os.RemoveAll(localTemp)
		} else {
			defer os.Remove(localTemp)
		}
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		remote := info.RemotePath
		local := info.LocalPath
		if upload {
			remote = remoteTemp
		} else {
			local = localTemp
		}
		if directory {
			remote = path.Join(remote, entry.relative)
			local = filepath.Join(local, filepath.FromSlash(entry.relative))
		}
		if entry.directory {
			if entry.relative != "" {
				if upload {
					err = client.Mkdir(remote)
				} else {
					err = os.Mkdir(local, 0o700)
				}
				if err != nil {
					return "", err
				}
			}
			continue
		}
		var source io.ReadCloser
		var destination io.WriteCloser
		if upload {
			source, err = os.Open(local)
			if err == nil {
				destination, err = client.OpenFile(remote, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
			}
		} else {
			source, err = client.Open(remote)
			if err == nil {
				destination, err = os.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			}
		}
		if err != nil {
			if source != nil {
				source.Close()
			}
			return "", err
		}
		if err := m.copy(ctx, task, destination, source); err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if upload {
		if _, err := client.Lstat(info.RemotePath); err == nil {
			return "", errors.New("remote destination already exists")
		}
		if err := client.Rename(remoteTemp, info.RemotePath); err != nil {
			return "", err
		}
		return info.LocalPath, nil
	}

	destination := info.LocalPath
	if directory {
		if _, err := os.Lstat(destination); err == nil {
			destination = filepath.Join(filepath.Dir(destination), filepath.Base(destination)+" ("+info.ID[:8]+")")
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if _, err := os.Lstat(destination); err == nil {
			return "", errors.New("download destination already exists")
		}
		if err := os.Rename(localTemp, destination); err != nil {
			return "", err
		}
		return destination, nil
	}
	for attempt := 0; attempt <= 1000; attempt++ {
		destination = info.LocalPath
		if attempt > 0 {
			destination = fmt.Sprintf("%s (%d)", info.LocalPath, attempt)
		}
		if err := os.Link(localTemp, destination); err == nil {
			return destination, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", errors.New("too many files with this name in the download folder")
}
