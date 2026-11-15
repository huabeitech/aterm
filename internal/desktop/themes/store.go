package themes

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Store struct {
	mu        sync.Mutex
	directory string
}

func NewStore(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	return &Store{directory: directory}, nil
}

// Read only bounded regular files. SameFile detects replacements between Lstat
// and Open, and the reader limit also covers files enlarged while being read.
func read(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return nil, errors.New("theme file must be a regular file smaller than 1 MB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("theme file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err == nil && len(data) > MaxBytes {
		err = errors.New("theme package exceeds 1 MB")
	}
	return data, err
}
func (s *Store) List() (Catalog, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.list() }
func (s *Store) list() (Catalog, error) {
	result := Catalog{Packages: []Package{}, Warnings: []string{}}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := read(filepath.Join(s.directory, entry.Name()))
		var pkg Package
		if err == nil {
			pkg, err = Parse(data, false)
		}
		if err == nil && entry.Name() != pkg.ID+".json" {
			err = errors.New("package filename does not match its ID")
		}
		if err != nil {
			result.Warnings = append(result.Warnings, entry.Name()+": "+err.Error())
		} else {
			result.Packages = append(result.Packages, pkg)
		}
	}
	return result, nil
}
func Inspect(path string) (Package, error) {
	data, err := read(path)
	if err != nil {
		return Package{}, err
	}
	if strings.HasSuffix(path, "manifest.json") {
		var manifest map[string]any
		if err = json.Unmarshal(data, &manifest); err != nil {
			return Package{}, err
		}
		if entries, ok := manifest["themes"].([]any); ok {
			if len(entries) < 1 || len(entries) > 64 {
				return Package{}, invalid("themes")
			}
			root, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				return Package{}, err
			}
			for i, v := range entries {
				entry, ok := v.(map[string]any)
				if !ok {
					continue
				}
				relative, ok := entry["path"].(string)
				if !ok {
					continue
				}
				target := filepath.Join(root, relative)
				rel, err := filepath.Rel(root, target)
				if err != nil || filepath.IsAbs(relative) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return Package{}, errors.New("theme path escapes its package")
				}
				current := root
				for _, part := range strings.Split(rel, string(filepath.Separator)) {
					current = filepath.Join(current, part)
					info, err := os.Lstat(current)
					if err != nil {
						return Package{}, err
					}
					if info.Mode()&os.ModeSymlink != 0 {
						return Package{}, errors.New("theme paths cannot contain symbolic links")
					}
				}
				bytes, err := read(target)
				if err != nil {
					return Package{}, err
				}
				var def map[string]any
				if err = json.Unmarshal(bytes, &def); err != nil || def == nil {
					return Package{}, errors.New("invalid theme definition")
				}
				for _, k := range []string{"id", "name", "appearance"} {
					def[k] = entry[k]
				}
				entries[i] = def
			}
			data, err = json.Marshal(manifest)
			if err != nil {
				return Package{}, err
			}
		}
	}
	return Parse(data, false)
}
func (s *Store) Install(path string) (Catalog, error) {
	pkg, err := Inspect(path)
	if err != nil {
		return Catalog{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.install(pkg)
}

// InstallPackage installs the exact inspected and approved package without rereading
// a caller-owned import file after the confirmation dialog closes.
func (s *Store) InstallPackage(pkg Package) (Catalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.install(pkg)
}
func (s *Store) install(pkg Package) (Catalog, error) {
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return Catalog{}, err
	}
	pkg, err = Parse(data, false)
	if err != nil {
		return Catalog{}, err
	}
	file, err := os.CreateTemp(s.directory, ".theme-*.tmp")
	if err != nil {
		return Catalog{}, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return Catalog{}, err
	}
	if err = file.Sync(); err != nil {
		return Catalog{}, err
	}
	if err = file.Close(); err != nil {
		return Catalog{}, err
	}
	if err = os.Rename(file.Name(), filepath.Join(s.directory, pkg.ID+".json")); err != nil {
		return Catalog{}, err
	}
	return s.list()
}
func (s *Store) Duplicate(reference string) (Catalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	catalog, err := s.list()
	if err != nil {
		return Catalog{}, err
	}
	builtin := Builtin()
	parts := strings.Split(reference, "/")
	pid, tid := builtin.ID, parts[0]
	if len(parts) > 1 {
		pid, tid = parts[0], parts[1]
	}
	for _, pkg := range append([]Package{builtin}, catalog.Packages...) {
		if pkg.ID != pid {
			continue
		}
		for _, theme := range pkg.Themes {
			if theme.ID != tid {
				continue
			}
			name := theme.Name + " copy"
			theme.ID = "default"
			return s.install(Package{SchemaVersion: 1, ThemeAPIVersion: 1, ID: "user.theme-" + uuid.NewString(), Name: name, Version: "1.0.0", Author: "Local", Themes: []Definition{theme}})
		}
	}
	return Catalog{}, errors.New("theme unavailable")
}
func (s *Store) Remove(reference string) (Catalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.HasPrefix(reference, "aterm.") {
		return Catalog{}, errors.New("built-in themes cannot be deleted")
	}
	for _, t := range Builtin().Themes {
		if t.ID == reference {
			return Catalog{}, errors.New("built-in themes cannot be deleted")
		}
	}
	parts := strings.Split(reference, "/")
	catalog, err := s.list()
	if err != nil {
		return Catalog{}, err
	}
	for _, pkg := range catalog.Packages {
		if pkg.ID != parts[0] {
			continue
		}
		if len(parts) > 1 && parts[1] != "" {
			found := false
			remaining := []Definition{}
			for _, t := range pkg.Themes {
				if t.ID == parts[1] {
					found = true
				} else {
					remaining = append(remaining, t)
				}
			}
			if !found {
				return Catalog{}, errors.New("unknown theme package")
			}
			if len(remaining) > 0 {
				pkg.Themes = remaining
				return s.install(pkg)
			}
		}
		if err = os.Remove(filepath.Join(s.directory, pkg.ID+".json")); err != nil {
			return Catalog{}, err
		}
		return s.list()
	}
	return Catalog{}, errors.New("unknown theme package")
}
func (s *Store) Export(id, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	catalog, err := s.list()
	if err != nil {
		return err
	}
	for _, pkg := range catalog.Packages {
		if pkg.ID != id {
			continue
		}
		data, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return err
		}
		return writeExport(path, data)
	}
	return fmt.Errorf("unknown theme package")
}

// Never follow a replaced export destination into an unrelated file.
func writeExport(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !info.Mode().IsRegular() {
		return errors.New("theme export destination must be a regular file")
	}
	flags := os.O_WRONLY
	if os.IsNotExist(err) {
		flags |= os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if info != nil && !os.SameFile(info, opened) {
		return errors.New("theme export destination changed")
	}
	if err = f.Truncate(0); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
