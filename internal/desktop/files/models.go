package files

import (
	"github.com/pkg/sftp"
	"io"
	"sync"
)

type SessionInfo struct {
	ID          string `json:"id"`
	HostID      string `json:"hostID,omitempty"`
	Target      any    `json:"target"`
	Title       string `json:"title"`
	State       string `json:"state"`
	Home        string `json:"home"`
	InitialPath string `json:"initialPath,omitempty"`
	Error       string `json:"error,omitempty"`
}
type Session struct {
	Info   SessionInfo
	Client *sftp.Client
	owner  io.Closer
	saveMu sync.Mutex
}
type Entry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Mode     uint32 `json:"mode"`
	UID      uint32 `json:"uid"`
	GID      uint32 `json:"gid"`
}
type Listing struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}
type Text struct {
	Path     string `json:"path"`
	Text     string `json:"text"`
	Revision string `json:"revision"`
}
type Mutation struct {
	Kind        string   `json:"kind"`
	Parent      string   `json:"parent"`
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Destination string   `json:"destination"`
	Paths       []string `json:"paths"`
	Mode        string   `json:"mode"`
}
