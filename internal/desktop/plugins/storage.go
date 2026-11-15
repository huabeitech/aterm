package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"ssh-connect/internal/fsutil"
	"strings"
	"sync"
)

var storageKey = regexp.MustCompile(`^[a-zA-Z0-9.-]{1,100}$`)

type Storage struct {
	mu   sync.Mutex
	root string
}

func NewStorage(root string) *Storage { return &Storage{root: root} }
func (s *Storage) file(id, key string) (string, string, error) {
	if !pluginID.MatchString(id) || !storageKey.MatchString(key) {
		return "", "", errors.New("invalid storage key")
	}
	directory := filepath.Join(s.root, id)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", "", err
	}
	if err := checkedDirectory(directory); err != nil {
		return "", "", err
	}
	path := filepath.Join(directory, key+".json")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() > 32768 {
			return "", "", errors.New("invalid storage file")
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	return path, directory, nil
}
func (s *Storage) Get(id, key string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, _, err := s.file(id, key)
	if err != nil {
		return nil, err
	}
	data, err := regularRead(path, 32768)
	if os.IsNotExist(err) {
		return json.RawMessage("null"), nil
	}
	if err != nil {
		return nil, err
	}
	return JSONSnapshot(data, 32768)
}
func (s *Storage) Set(id, key string, value json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, directory, err := s.file(id, key)
	if err != nil {
		return err
	}
	data, err := JSONSnapshot(value, 30000)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	count, size := 0, int64(len(data))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		count++
		other := filepath.Join(directory, entry.Name())
		if other == path {
			continue
		}
		info, err := os.Lstat(other)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("invalid storage file")
		}
		size += info.Size()
	}
	if _, err = os.Lstat(path); os.IsNotExist(err) && count >= 64 {
		return errors.New("plugin storage key limit")
	}
	if size > 256*1024 {
		return errors.New("plugin storage size limit")
	}
	return fsutil.AtomicWriteFile(path, data, 0600)
}
