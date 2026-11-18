package plugins

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const ArchiveLimit = 64 * 1024 * 1024
const archiveFileLimit = 2 * 1024 * 1024

var archiveName = regexp.MustCompile(`^(manifest\.json|plugin\.js|ui/[A-Za-z0-9._-]+\.html|locales/[A-Za-z0-9._-]+\.json|backend/(darwin|linux|windows)-(amd64|arm64)/plugin(\.exe)?)$`)
var translationKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,120}$`)
var packageReference = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9.-]*/[0-9]+\.[0-9]+\.[0-9]+-[a-f0-9]{64}$`)

func regularRead(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return nil, errors.New("invalid plugin file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("plugin file changed while opening")
	}
	return boundedRead(f, limit)
}
func boundedRead(reader io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err == nil && len(data) > limit {
		err = errors.New("plugin file size limit")
	}
	return data, err
}
func checkedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("plugin directory must not be a symlink")
	}
	return nil
}
func InspectDirectory(directory string) (Package, error) {
	if err := checkedDirectory(directory); err != nil {
		return Package{}, err
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return Package{}, err
	}
	return inspectFiles(func(name string, limit int) ([]byte, error) {
		for dir := filepath.Dir(name); dir != "."; dir = filepath.Dir(dir) {
			if err := checkedDirectory(filepath.Join(root, dir)); err != nil {
				return nil, err
			}
		}
		return regularRead(filepath.Join(root, name), limit)
	})
}
func InspectArchive(path string) (Package, error) {
	data, err := regularRead(path, ArchiveLimit)
	if err != nil {
		return Package{}, err
	}
	return InspectArchiveBytes(data)
}
func InspectArchiveBytes(data []byte) (Package, error) {
	if len(data) > ArchiveLimit {
		return Package{}, errors.New("plugin archive size limit")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Package{}, err
	}
	if len(archive.File) > 128 {
		return Package{}, errors.New("plugin archive entry limit")
	}
	files := map[string][]byte{}
	var size uint64
	for _, entry := range archive.File {
		_, duplicate := files[entry.Name]
		mode := (entry.ExternalAttrs >> 16) & 0xf000
		if duplicate || !archiveName.MatchString(entry.Name) || (mode != 0 && mode != 0x8000) || entry.Flags&1 != 0 || entry.UncompressedSize64 > uint64(packageFileLimit(entry.Name)) {
			return Package{}, errors.New("invalid plugin archive entry")
		}
		size += entry.UncompressedSize64
		if size > ArchiveLimit {
			return Package{}, errors.New("plugin archive expansion limit")
		}
		stream, err := entry.Open()
		if err != nil {
			return Package{}, err
		}
		bytes, err := boundedRead(stream, packageFileLimit(entry.Name))
		closeErr := stream.Close()
		if err != nil {
			return Package{}, err
		}
		if closeErr != nil {
			return Package{}, closeErr
		}
		if uint64(len(bytes)) != entry.UncompressedSize64 {
			return Package{}, errors.New("plugin archive checksum or size mismatch")
		}
		files[entry.Name] = bytes
	}
	plugin, err := inspectFiles(func(name string, limit int) ([]byte, error) {
		data, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("plugin file missing: %s", name)
		}
		if len(data) > limit {
			return nil, errors.New("plugin file size limit")
		}
		return data, nil
	})
	if err != nil {
		return Package{}, err
	}
	expected, err := packageFiles(plugin)
	if err != nil {
		return Package{}, err
	}
	if len(expected) != len(files) {
		return Package{}, errors.New("undeclared plugin archive files")
	}
	for name := range expected {
		if _, ok := files[name]; !ok {
			return Package{}, errors.New("undeclared plugin archive files")
		}
	}
	return plugin, nil
}
func inspectFiles(read func(string, int) ([]byte, error)) (Package, error) {
	manifestData, err := read("manifest.json", 32768)
	if err != nil {
		return Package{}, err
	}
	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return Package{}, err
	}
	if strings.HasPrefix(manifest.ID, "aterm.") {
		return Package{}, errors.New("reserved plugin identifier")
	}
	plugin := Package{Manifest: manifest, BuiltIn: false}
	if manifest.Runtime == "native" {
		plugin.Binary, err = read(manifest.Entry, packageFileLimit(manifest.Entry))
		if err != nil {
			return Package{}, err
		}
		hash := sha256.Sum256(plugin.Binary)
		if fmt.Sprintf("%x", hash) != manifest.Backend.SHA256 {
			return Package{}, errors.New("native backend checksum mismatch")
		}
	} else {
		source, err := read("plugin.js", 256*1024)
		if err != nil {
			return Package{}, err
		}
		plugin.Source = strings.ToValidUTF8(string(source), "\uFFFD")
	}
	if manifest.I18n != nil {
		config := manifest.I18n
		bundle := &Localizations{DefaultLocale: config.DefaultLocale, Messages: map[string]map[string]string{}}
		size := 0
		locales := make([]string, 0, len(config.Locales))
		for locale := range config.Locales {
			locales = append(locales, locale)
		}
		sort.Strings(locales)
		for _, locale := range locales {
			data, err := read(config.Locales[locale], 64*1024)
			if err != nil {
				return Package{}, err
			}
			size += len(data)
			if size > 512*1024 {
				return Package{}, errors.New("plugin localization size limit")
			}
			var values map[string]any
			if err = json.Unmarshal(data, &values); err != nil || values == nil || len(values) > 512 {
				return Package{}, errors.New("invalid plugin locale dictionary")
			}
			translations := map[string]string{}
			for key, v := range values {
				value, ok := v.(string)
				if !ok || !translationKey.MatchString(key) || key == "__proto__" || key == "constructor" || key == "prototype" || !validTitle(value, 4096) {
					return Package{}, errors.New("invalid plugin translation")
				}
				translations[key] = value
			}
			bundle.Messages[locale] = translations
		}
		plugin.Localizations = bundle
	}
	if len(manifest.Contributes.Views) > 0 {
		plugin.Views = map[string]string{}
		size := 0
		for _, view := range manifest.Contributes.Views {
			if _, ok := plugin.Views[view.Entry]; ok {
				continue
			}
			data, err := read(view.Entry, archiveFileLimit)
			if err != nil {
				return Package{}, err
			}
			size += len(data)
			if size > 8*1024*1024 {
				return Package{}, errors.New("plugin UI size limit")
			}
			plugin.Views[view.Entry] = strings.ToValidUTF8(string(data), "\uFFFD")
		}
	}
	return plugin, nil
}
func packageFiles(plugin Package) (map[string][]byte, error) {
	data, err := marshalJSON(plugin.Manifest)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{"manifest.json": data}
	if plugin.Manifest.Runtime == "native" {
		files[plugin.Manifest.Entry] = plugin.Binary
	} else {
		files["plugin.js"] = []byte(plugin.Source)
	}
	for name, html := range plugin.Views {
		files[name] = []byte(html)
	}
	if config := plugin.Manifest.I18n; config != nil {
		if plugin.Localizations == nil {
			return nil, errors.New("plugin localizations missing")
		}
		for locale, path := range config.Locales {
			data, err := marshalJSON(plugin.Localizations.Messages[locale])
			if err != nil {
				return nil, err
			}
			files[path] = data
		}
	}
	return files, nil
}
func packageDigest(plugin Package) (string, error) {
	value := struct {
		Manifest      Manifest          `json:"manifest"`
		Source        string            `json:"source"`
		Localizations *Localizations    `json:"localizations"`
		Views         map[string]string `json:"views"`
		Binary        []byte            `json:"binary,omitempty"`
	}{plugin.Manifest, plugin.Source, plugin.Localizations, plugin.Views, plugin.Binary}
	data, err := marshalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

type PackageStore struct{ root string }

func NewPackageStore(root string) (*PackageStore, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &PackageStore{root}, nil
}

// Install revalidates the complete normalized package before writing any path.
func (s *PackageStore) Install(plugin Package) (string, error) {
	files, err := packageFiles(plugin)
	if err != nil {
		return "", err
	}
	normalized, err := inspectFiles(func(name string, limit int) ([]byte, error) {
		data, ok := files[name]
		if !ok || len(data) > limit {
			return nil, errors.New("invalid plugin file")
		}
		return data, nil
	})
	if err != nil {
		return "", err
	}
	// Do not silently drop undeclared files from a caller-assembled package.
	expected, _ := packageFiles(normalized)
	if len(expected) != len(files) {
		return "", errors.New("undeclared plugin package files")
	}
	digest, err := packageDigest(normalized)
	if err != nil {
		return "", err
	}
	key := normalized.Manifest.ID + "/" + normalized.Manifest.Version + "-" + digest
	target := filepath.Join(s.root, filepath.FromSlash(key))
	if _, err := os.Lstat(target); err == nil {
		if _, err = s.Load(key); err != nil {
			return "", err
		}
		return key, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	staging, err := os.MkdirTemp(s.root, ".staging-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	for name, data := range expected {
		path := filepath.Join(staging, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			return "", err
		}
	}
	parent := filepath.Dir(target)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	if err = checkedDirectory(parent); err != nil {
		return "", err
	}
	if err = os.Rename(staging, target); err != nil {
		return "", err
	}
	return key, nil
}
func (s *PackageStore) Load(key string) (Package, error) {
	if !packageReference.MatchString(key) {
		return Package{}, errors.New("invalid plugin package reference")
	}
	path := filepath.Join(s.root, filepath.FromSlash(key))
	if err := checkedDirectory(filepath.Dir(path)); err != nil {
		return Package{}, err
	}
	plugin, err := InspectDirectory(path)
	if err != nil {
		return Package{}, err
	}
	digest, err := packageDigest(plugin)
	if err != nil {
		return Package{}, err
	}
	if key != plugin.Manifest.ID+"/"+plugin.Manifest.Version+"-"+digest {
		return Package{}, errors.New("plugin integrity check failed")
	}
	return plugin, nil
}

func packageFileLimit(name string) int {
	if strings.HasPrefix(name, "backend/") {
		return 48 * 1024 * 1024
	}
	return archiveFileLimit
}
func (s *PackageStore) Executable(key string) (string, error) {
	p, err := s.Load(key)
	if err != nil {
		return "", err
	}
	if p.Manifest.Runtime != "native" {
		return "", errors.New("not native")
	}
	path := filepath.Join(s.root, filepath.FromSlash(key), filepath.FromSlash(p.Manifest.Entry))
	if err = os.Chmod(path, 0700); err != nil {
		return "", err
	}
	return path, nil
}
