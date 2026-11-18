package plugins

import (
	"os"
	"path/filepath"
	"ssh-connect/internal/fsutil"
	"sync"
)

const nativeLogLimit = 64 * 1024

// Native diagnostics are local, bounded and private to the application profile.
type nativeLog struct {
	mu   sync.Mutex
	path string
	tail []byte
}

func newNativeLog(root, id string) (*nativeLog, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	l := &nativeLog{path: filepath.Join(root, id+".log")}
	tail, err := regularRead(l.path, nativeLogLimit)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l.tail = tail
	return l, nil
}
func (l *nativeLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(p)
	if len(p) > nativeLogLimit {
		p = p[len(p)-nativeLogLimit:]
	}
	next := append(append([]byte{}, l.tail...), p...)
	if len(next) > nativeLogLimit {
		next = next[len(next)-nativeLogLimit:]
	}
	if err := fsutil.AtomicWriteFile(l.path, next, 0600); err != nil {
		return 0, err
	}
	l.tail = next
	return n, nil
}
