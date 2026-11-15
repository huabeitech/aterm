package files

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
)

const MaxTextBytes = 1024 * 1024
const MaxImageBytes = 20 * 1024 * 1024

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager() *Manager { return &Manager{sessions: map[string]*Session{}} }
func (m *Manager) Add(client *sftp.Client, owner io.Closer, info SessionInfo) (SessionInfo, error) {
	home, err := client.Getwd()
	if err != nil {
		client.Close()
		if owner != nil {
			owner.Close()
		}
		return SessionInfo{}, err
	}
	info.ID = uuid.NewString()
	info.State = "ready"
	info.Home = home
	m.mu.Lock()
	m.sessions[info.ID] = &Session{Info: info, Client: client, owner: owner}
	m.mu.Unlock()
	return info, nil
}
func (m *Manager) Session(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value := m.sessions[id]
	if value == nil {
		return nil, errors.New("file session is no longer connected")
	}
	return value, nil
}
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	session := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if session == nil {
		return nil
	}
	err := session.Client.Close()
	if session.owner != nil {
		session.owner.Close()
	}
	// An owning OpenSSH master may have already closed the pipe. Closing its
	// file workspace is still successful and must not surface a cleanup toast.
	if errors.Is(err, os.ErrClosed) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
func (m *Manager) CloseAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		m.Close(id)
	}
}
func absolute(value string) (string, error) {
	if !strings.HasPrefix(value, "/") || strings.IndexByte(value, 0) >= 0 {
		return "", errors.New("enter an absolute remote path")
	}
	return path.Clean(value), nil
}
func child(parent, name string) (string, error) {
	directory, err := absolute(parent)
	if err != nil {
		return "", err
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return "", errors.New("enter a valid file name")
	}
	return path.Join(directory, name), nil
}
func (m *Manager) List(id, requested string) (Listing, error) {
	session, err := m.Session(id)
	if err != nil {
		return Listing{}, err
	}
	if requested == "" {
		requested = session.Info.Home
	}
	directory, err := absolute(requested)
	if err != nil {
		return Listing{}, err
	}
	directory, err = session.Client.RealPath(directory)
	if err != nil {
		return Listing{}, err
	}
	entries, err := session.Client.ReadDir(directory)
	if err != nil {
		return Listing{}, err
	}
	result := Listing{Path: directory, Entries: []Entry{}}
	for _, info := range entries {
		kind := "other"
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "link"
		} else if info.IsDir() {
			kind = "directory"
		} else if info.Mode().IsRegular() {
			kind = "file"
		}
		entry := Entry{Name: info.Name(), Path: path.Join(directory, info.Name()), Kind: kind, Size: info.Size(), Modified: info.ModTime().UnixMilli(), Mode: uint32(info.Mode().Perm())}
		if stat, ok := info.Sys().(*sftp.FileStat); ok {
			entry.UID = stat.UID
			entry.GID = stat.GID
			entry.Mode = stat.Mode
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}
func readBounded(client *sftp.Client, name string, limit int64) ([]byte, error) {
	original, err := client.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !original.Mode().IsRegular() || original.Size() > limit {
		return nil, errors.New("file is not a regular file or exceeds the size limit")
	}
	file, err := client.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("file is not a regular file or exceeds the size limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds the size limit")
	}
	return data, nil
}
func revision(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func (m *Manager) ReadText(id, name string) (Text, error) {
	session, err := m.Session(id)
	if err != nil {
		return Text{}, err
	}
	name, err = absolute(name)
	if err != nil {
		return Text{}, err
	}
	data, err := readBounded(session.Client, name, MaxTextBytes)
	if err != nil {
		return Text{}, err
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return Text{}, errors.New("this file is not UTF-8 text")
	}
	return Text{Path: name, Text: string(data), Revision: revision(data)}, nil
}
func (m *Manager) SaveText(id string, document Text) (Text, error) {
	session, err := m.Session(id)
	if err != nil {
		return Text{}, err
	}
	session.saveMu.Lock()
	defer session.saveMu.Unlock()
	name, err := absolute(document.Path)
	if err != nil {
		return Text{}, err
	}
	if len(document.Text) > MaxTextBytes || !utf8.ValidString(document.Text) || strings.IndexByte(document.Text, 0) >= 0 {
		return Text{}, errors.New("text must be UTF-8 and no larger than 1 MB")
	}
	current, err := readBounded(session.Client, name, MaxTextBytes)
	if err != nil {
		return Text{}, err
	}
	if revision(current) != document.Revision {
		return Text{}, errors.New("the remote file changed; reload it before saving")
	}
	info, err := session.Client.Stat(name)
	if err != nil {
		return Text{}, err
	}
	temp := path.Join(path.Dir(name), "."+path.Base(name)+".aterm-"+uuid.NewString())
	file, err := session.Client.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return Text{}, err
	}
	defer session.Client.Remove(temp)
	if err = file.Chmod(info.Mode().Perm()); err == nil {
		_, err = file.Write([]byte(document.Text))
	}
	closeErr := file.Close()
	if err != nil {
		return Text{}, err
	}
	if closeErr != nil {
		return Text{}, closeErr
	}
	latest, err := readBounded(session.Client, name, MaxTextBytes)
	if err != nil {
		return Text{}, err
	}
	if revision(latest) != document.Revision {
		return Text{}, errors.New("the remote file changed while saving; the original file was preserved")
	}
	if err = session.Client.PosixRename(temp, name); err != nil {
		return Text{}, fmt.Errorf("atomic save failed: %w", err)
	}
	return Text{Path: name, Text: document.Text, Revision: revision([]byte(document.Text))}, nil
}
func (m *Manager) Image(id, name string) (map[string]any, error) {
	session, err := m.Session(id)
	if err != nil {
		return nil, err
	}
	name, err = absolute(name)
	if err != nil {
		return nil, err
	}
	data, err := readBounded(session.Client, name, MaxImageBytes)
	if err != nil {
		return nil, err
	}
	media := http.DetectContentType(data)
	if media != "image/png" && media != "image/jpeg" && media != "image/gif" && media != "image/webp" && media != "image/bmp" {
		return nil, errors.New("unsupported image format")
	}
	return map[string]any{"dataURL": "data:" + media + ";base64," + base64.StdEncoding.EncodeToString(data), "size": len(data)}, nil
}
func (m *Manager) Mutate(id string, input Mutation) error {
	session, err := m.Session(id)
	if err != nil {
		return err
	}
	client := session.Client
	switch input.Kind {
	case "mkdir", "touch":
		name, err := child(input.Parent, input.Name)
		if err != nil {
			return err
		}
		if input.Kind == "mkdir" {
			return client.Mkdir(name)
		}
		file, err := client.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		if err != nil {
			return err
		}
		return file.Close()
	case "rename":
		from, err := absolute(input.Path)
		if err != nil {
			return err
		}
		to, err := absolute(input.Destination)
		if err != nil {
			return err
		}
		if from == "/" || to == "/" {
			return errors.New("cannot rename the root directory")
		}
		if _, err := client.Lstat(to); err == nil {
			return errors.New("destination already exists")
		}
		return client.Rename(from, to)
	case "chmod":
		name, err := absolute(input.Path)
		if err != nil {
			return err
		}
		mode, err := strconv.ParseUint(input.Mode, 8, 12)
		if err != nil {
			return errors.New("enter valid octal permissions")
		}
		return client.Chmod(name, os.FileMode(mode))
	case "delete":
		remaining := 10000
		for _, name := range input.Paths {
			name, err = absolute(name)
			if err != nil {
				return err
			}
			if name == "/" {
				return errors.New("cannot delete the root directory")
			}
			if err := removeTree(client, name, 0, &remaining); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("unknown file operation")
	}
}
func removeTree(client *sftp.Client, name string, depth int, remaining *int) error {
	*remaining--
	if *remaining < 0 || depth > 64 {
		return errors.New("recursive delete exceeds 10000 entries or 64 levels")
	}
	info, err := client.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return client.Remove(name)
	}
	entries, err := client.ReadDir(name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := removeTree(client, path.Join(name, entry.Name()), depth+1, remaining); err != nil {
			return err
		}
	}
	return client.RemoveDirectory(name)
}
