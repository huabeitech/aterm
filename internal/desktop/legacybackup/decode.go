// Package legacybackup reads the authenticated Electron AES backup format.
// It has no filesystem, database, native UI or live-state dependencies.
package legacybackup

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/scrypt"
	"ssh-connect/internal/desktop"
	"ssh-connect/internal/domain"
)

const MaxBytes = 64 * 1024 * 1024

var legacyAlias = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

var ErrPassword = errors.New("incorrect backup password or damaged backup; no data was restored")

type sealed struct {
	Salt string `json:"salt"`
	IV   string `json:"iv"`
	Tag  string `json:"tag"`
	Data string `json:"data"`
}
type envelope struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	sealed
}
type library struct {
	Version     int                `json:"version"`
	Hosts       []domain.Host      `json:"hosts"`
	Directories []domain.Directory `json:"directories"`
	Vault       *sealed            `json:"vault"`
	SyncVault   *struct {
		Encoded      string `json:"encoded"`
		ProtectedKey sealed `json:"protectedKey"`
	} `json:"syncVault"`
}
type contents struct {
	Format      string              `json:"format"`
	Version     int                 `json:"version"`
	Library     library             `json:"library"`
	Preferences desktop.Preferences `json:"preferences"`
}
type Workspace struct {
	Hosts             []domain.Host
	Directories       []domain.Directory
	SortOrders        []domain.HostSortOrder
	Passwords         map[string]string
	Preferences       desktop.Preferences
	CompatibleEncoded []byte
	CompatibleKey     []byte
}

// Recognizes never falls back from a failed modern-vault password attempt.
func Recognizes(data []byte) bool {
	var header struct {
		Format string `json:"format"`
	}
	return json.Unmarshal(data, &header) == nil && header.Format == "aterm-backup"
}
func decodeField(value string) ([]byte, error) {
	return base64.StdEncoding.Strict().DecodeString(value)
}
func open(value sealed, key []byte, aad string) ([]byte, error) {
	salt, err := decodeField(value.Salt)
	if err != nil || len(salt) != 16 {
		return nil, errors.New("invalid legacy backup salt")
	}
	iv, err := decodeField(value.IV)
	if err != nil || len(iv) != 12 {
		return nil, errors.New("invalid legacy backup nonce")
	}
	tag, err := decodeField(value.Tag)
	if err != nil || len(tag) != 16 {
		return nil, errors.New("invalid legacy backup authentication tag")
	}
	data, err := decodeField(value.Data)
	if err != nil || len(data) > MaxBytes {
		return nil, errors.New("invalid legacy backup ciphertext")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	result, err := gcm.Open(nil, iv, append(data, tag...), []byte(aad))
	if err != nil {
		return nil, ErrPassword
	}
	return result, nil
}
func Decode(data []byte, password string) (Workspace, error) {
	if len(data) > MaxBytes {
		return Workspace{}, errors.New("backup exceeds the supported size")
	}
	var encoded envelope
	if err := json.Unmarshal(data, &encoded); err != nil {
		return Workspace{}, err
	}
	if encoded.Format != "aterm-backup" || encoded.Version != 1 {
		return Workspace{}, errors.New("unsupported legacy backup format")
	}
	salt, err := decodeField(encoded.Salt)
	if err != nil || len(salt) != 16 {
		return Workspace{}, errors.New("invalid legacy backup salt")
	}
	key, err := scrypt.Key([]byte(password), salt, 16384, 8, 1, 32)
	if err != nil {
		return Workspace{}, err
	}
	defer clear(key)
	plaintext, err := open(encoded.sealed, key, "aterm-backup-v1")
	if err != nil {
		return Workspace{}, err
	}
	defer clear(plaintext)
	if err = validateShape(plaintext); err != nil {
		return Workspace{}, err
	}
	var value contents
	if err = json.Unmarshal(plaintext, &value); err != nil {
		return Workspace{}, errors.New("invalid legacy backup contents")
	}
	if value.Format != "aterm-library" || value.Version != 1 || value.Library.Version != 1 || value.Library.Hosts == nil || value.Library.Directories == nil {
		return Workspace{}, errors.New("unsupported legacy library contents")
	}
	if value.Preferences == nil {
		return Workspace{}, errors.New("missing legacy backup preferences")
	}
	preferences, err := desktop.ValidatePreferences(value.Preferences)
	if err != nil {
		return Workspace{}, err
	}
	result := Workspace{Hosts: value.Library.Hosts, Directories: value.Library.Directories, Preferences: preferences, Passwords: map[string]string{}}
	if err = validate(&result); err != nil {
		return Workspace{}, err
	}
	if value.Library.Vault != nil {
		secrets, err := open(*value.Library.Vault, key, "aterm-electron-vault-v1")
		if err != nil {
			return Workspace{}, err
		}
		err = json.Unmarshal(secrets, &result.Passwords)
		clear(secrets)
		if err != nil || result.Passwords == nil {
			return Workspace{}, errors.New("invalid legacy password catalog")
		}
	} else {
		for _, host := range result.Hosts {
			if host.PasswordAuth {
				return Workspace{}, errors.New("missing legacy encrypted password catalog")
			}
		}
	}
	if compatible := value.Library.SyncVault; compatible != nil {
		protected, err := open(compatible.ProtectedKey, key, "aterm-electron-vault-v1")
		if err != nil {
			return Workspace{}, err
		}
		var value struct {
			Key string `json:"key"`
		}
		err = json.Unmarshal(protected, &value)
		clear(protected)
		if err != nil {
			return Workspace{}, errors.New("invalid protected backup key")
		}
		backupKey, err := decodeField(value.Key)
		if err != nil || len(backupKey) != 32 {
			clear(backupKey)
			return Workspace{}, errors.New("invalid protected backup key")
		}
		if len(compatible.Encoded) > MaxBytes {
			clear(backupKey)
			return Workspace{}, errors.New("embedded vault exceeds the supported size")
		}
		result.CompatibleEncoded = []byte(compatible.Encoded)
		result.CompatibleKey = backupKey
	}
	return result, nil
}
func validate(w *Workspace) error {
	if len(w.Hosts) > 100000 || len(w.Directories) > 100000 {
		return errors.New("legacy backup has too many records")
	}
	dirs := map[string]string{}
	hosts := map[string]string{}
	aliases := map[string]bool{}
	for _, d := range w.Directories {
		if d.ID == "" || len(d.ID) > 200 {
			return errors.New("invalid legacy folder identifier")
		}
		if _, exists := dirs[d.ID]; exists {
			return errors.New("duplicate legacy folder identifier")
		}
		if err := domain.ValidateDirectoryInput(domain.DirectoryInput{Name: d.Name, ParentID: d.ParentID}); err != nil {
			return err
		}
		dirs[d.ID] = d.ParentID
	}
	if err := validateGraph(dirs, "folder"); err != nil {
		return err
	}
	for i := range w.Hosts {
		h := &w.Hosts[i]
		if h.ID == "" || len(h.ID) > 200 {
			return errors.New("invalid legacy host identifier")
		}
		if _, exists := hosts[h.ID]; exists {
			return errors.New("duplicate legacy host identifier")
		}
		if aliases[h.Alias] {
			return errors.New("duplicate legacy host alias")
		}
		aliases[h.Alias] = true
		if !legacyAlias.MatchString(h.Alias) {
			return errors.New("invalid legacy host alias")
		}
		h.Name = h.Alias
		if err := domain.ValidateHostInput(domain.HostInput{Name: h.Name, Alias: h.Alias, HostName: h.HostName, Port: h.Port}); err != nil {
			return err
		}
		if h.ServerAliveInterval < 0 || h.ServerAliveCountMax < 0 || h.ConnectTimeout < 0 {
			return errors.New("invalid legacy connection reliability values")
		}
		if h.DirectoryID != "" {
			if _, exists := dirs[h.DirectoryID]; !exists {
				return errors.New("legacy host folder is missing")
			}
		}
		if h.SortOrders == nil {
			return errors.New("missing legacy host sort orders")
		}
		for scope, order := range h.SortOrders {
			kind, id, ok := strings.Cut(scope, ":")
			if !ok {
				return errors.New("invalid legacy sort scope")
			}
			switch kind {
			case "favorites":
				delete(h.SortOrders, scope)
				continue
			case "all", "uncategorized":
				if id != "" {
					return errors.New("invalid legacy sort scope")
				}
			case "directory":
				if _, exists := dirs[id]; !exists {
					return errors.New("legacy sort folder is missing")
				}
			default:
				return errors.New("invalid legacy sort scope")
			}
			if len(w.SortOrders) >= 400000 {
				return errors.New("too many legacy sort entries")
			}
			w.SortOrders = append(w.SortOrders, domain.HostSortOrder{ScopeType: kind, ScopeID: id, HostID: h.ID, SortOrder: order, CreatedAt: h.CreatedAt, UpdatedAt: h.UpdatedAt})
		}
		hosts[h.ID] = h.JumpHostID
	}
	if err := validateGraph(hosts, "jump host"); err != nil {
		return err
	}
	aliasesByID := map[string]string{}
	for _, host := range w.Hosts {
		aliasesByID[host.ID] = host.Alias
	}
	for i := range w.Hosts {
		if jump := w.Hosts[i].JumpHostID; jump != "" {
			w.Hosts[i].ProxyJump = aliasesByID[jump]
		}
	}
	return nil
}
func validateGraph(parents map[string]string, kind string) error {
	colors := map[string]uint8{}
	for id := range parents {
		path := []string{}
		for next := id; next != "" && colors[next] != 2; next = parents[next] {
			if _, exists := parents[next]; !exists {
				return fmt.Errorf("legacy %s reference is missing", kind)
			}
			if colors[next] == 1 {
				return fmt.Errorf("legacy %s graph contains a cycle", kind)
			}
			colors[next] = 1
			path = append(path, next)
		}
		for _, id := range path {
			colors[id] = 2
		}
	}
	return nil
}
