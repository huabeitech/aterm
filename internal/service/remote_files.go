package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	pathpkg "path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRemoteTextFileBytes     = 2 * 1024 * 1024
	maxRemoteImagePreviewBytes = 20 * 1024 * 1024
)

// PrepareFileConnection validates and records use of the configured SSH host.
// File transfer deliberately reuses the user's OpenSSH configuration, so
// IdentityFile, ProxyJump and agent settings behave exactly like Terminal.
func (s *RemoteFileService) PrepareFileConnection(ctx context.Context, alias string) (Host, error) {
	return s.PrepareTerminalConnection(ctx, alias)
}

// RemoteHomeDirectory returns the physical login directory used by SSH. The
// file browser uses this absolute path instead of the shell-relative "." so
// its location field always describes where the user is browsing.
func (s *RemoteFileService) RemoteHomeDirectory(ctx context.Context, alias string) (string, error) {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return "", err
	}
	output, err := s.runNativeSSH(ctx, alias, "pwd -P", nil)
	if err != nil {
		return "", fmt.Errorf("get remote home directory: %w", err)
	}
	path := strings.TrimSpace(string(output))
	if !strings.HasPrefix(path, "/") {
		return "", errors.New("remote home directory is not an absolute path")
	}
	return pathpkg.Clean(path), nil
}

func (s *RemoteFileService) ListRemoteFiles(ctx context.Context, alias, remotePath string) ([]RemoteFile, error) {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return nil, err
	}
	remotePath = normalizeRemotePath(remotePath)
	// The delimiter is a unit separator, which is valid neither in POSIX file
	// names nor normal text output. GNU find is available on the supported SSH
	// targets; a clear error is returned for hosts without a POSIX file shell.
	command := "LC_ALL=C find " + shellQuote(remotePath) + " -mindepth 1 -maxdepth 1 -printf '%f\\037%y\\037%s\\037%T@\\037%W@\\037%A@\\037%C@\\037%M\\037%u\\037%g\\037%l\\037%i\\037%n\\n'"
	output, err := s.runNativeSSH(ctx, alias, command, nil)
	if err != nil {
		return nil, fmt.Errorf("list remote files: %w", err)
	}
	return parseRemoteFileList(string(output), remotePath), nil
}

func parseRemoteFileList(output, remotePath string) []RemoteFile {
	files := make([]RemoteFile, 0)
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\037")
		if len(parts) != 13 {
			continue
		}
		size, _ := strconv.ParseInt(parts[2], 10, 64)
		inode, _ := strconv.ParseInt(parts[11], 10, 64)
		linkCount, _ := strconv.ParseInt(parts[12], 10, 64)
		files = append(files, RemoteFile{
			Name: parts[0], Path: pathpkg.Join(remotePath, parts[0]), IsDirectory: parts[1] == "d", Kind: parts[1], Size: size,
			ModifiedAt: remoteFileTime(parts[3], false), CreatedAt: remoteFileTime(parts[4], true), AccessedAt: remoteFileTime(parts[5], false), ChangedAt: remoteFileTime(parts[6], false),
			Permissions: parts[7], Owner: parts[8], Group: parts[9], LinkTarget: parts[10], Inode: inode, LinkCount: linkCount,
		})
	}
	return files
}

func remoteFileTime(value string, unavailableWhenNegative bool) string {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || (unavailableWhenNegative && seconds < 0) {
		return ""
	}
	return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
}

func (s *RemoteFileService) ReadRemoteTextFile(ctx context.Context, alias, remotePath string) (RemoteTextFile, error) {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return RemoteTextFile{}, err
	}
	remotePath = normalizeRemotePath(remotePath)
	size, err := s.remoteFileSize(ctx, alias, remotePath)
	if err != nil {
		return RemoteTextFile{}, err
	}
	if size > maxRemoteTextFileBytes {
		return RemoteTextFile{}, fmt.Errorf("remote file is too large to edit (maximum is %d bytes)", maxRemoteTextFileBytes)
	}
	output, err := s.runNativeSSH(ctx, alias, "test -f "+shellQuote(remotePath)+" && test $(wc -c < "+shellQuote(remotePath)+") -le "+strconv.Itoa(maxRemoteTextFileBytes)+" && cat -- "+shellQuote(remotePath), nil)
	if err != nil {
		return RemoteTextFile{}, fmt.Errorf("read remote text file: %w", err)
	}
	if len(output) > maxRemoteTextFileBytes {
		return RemoteTextFile{}, errors.New("remote file is too large to edit")
	}
	if bytes.IndexByte(output, 0) >= 0 || !utf8.Valid(output) {
		return RemoteTextFile{}, errors.New("remote file is binary or uses an unsupported text encoding")
	}
	revision, err := s.remoteFileRevision(ctx, alias, remotePath)
	if err != nil {
		return RemoteTextFile{}, err
	}
	return RemoteTextFile{Path: remotePath, Content: base64.StdEncoding.EncodeToString(output), Size: int64(len(output)), Revision: revision, Language: remoteTextLanguage(remotePath)}, nil
}

func (s *RemoteFileService) WriteRemoteTextFile(ctx context.Context, alias, remotePath, content, expectedRevision string, overwrite bool) (RemoteTextFile, error) {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return RemoteTextFile{}, err
	}
	remotePath = normalizeRemotePath(remotePath)
	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return RemoteTextFile{}, errors.New("text content is invalid")
	}
	if len(data) > maxRemoteTextFileBytes {
		return RemoteTextFile{}, errors.New("remote file is too large to edit")
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return RemoteTextFile{}, errors.New("text content is binary or uses an unsupported text encoding")
	}
	if !overwrite {
		if expectedRevision == "" {
			return RemoteTextFile{}, errors.New("remote file revision is required to save")
		}
		currentRevision, err := s.remoteFileRevision(ctx, alias, remotePath)
		if err != nil {
			return RemoteTextFile{}, err
		}
		if currentRevision != expectedRevision {
			return RemoteTextFile{}, errors.New("remote file changed since it was opened")
		}
	}
	_, err = s.runNativeSSH(ctx, alias, "cat > "+shellQuote(remotePath), data)
	if err != nil {
		return RemoteTextFile{}, fmt.Errorf("save remote text file: %w", err)
	}
	revision, err := s.remoteFileRevision(ctx, alias, remotePath)
	if err != nil {
		return RemoteTextFile{}, err
	}
	return RemoteTextFile{Path: remotePath, Content: content, Size: int64(len(data)), Revision: revision, Language: remoteTextLanguage(remotePath)}, nil
}

func (s *RemoteFileService) ReadRemoteImagePreview(ctx context.Context, alias, remotePath string) (RemoteImagePreview, error) {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return RemoteImagePreview{}, err
	}
	remotePath = normalizeRemotePath(remotePath)
	size, err := s.remoteFileSize(ctx, alias, remotePath)
	if err != nil {
		return RemoteImagePreview{}, err
	}
	if size > maxRemoteImagePreviewBytes {
		return RemoteImagePreview{}, fmt.Errorf("remote image is too large to preview (maximum is %d bytes)", maxRemoteImagePreviewBytes)
	}
	output, err := s.runNativeSSH(ctx, alias, "test -f "+shellQuote(remotePath)+" && test $(wc -c < "+shellQuote(remotePath)+") -le "+strconv.Itoa(maxRemoteImagePreviewBytes)+" && cat -- "+shellQuote(remotePath), nil)
	if err != nil {
		return RemoteImagePreview{}, fmt.Errorf("read remote image preview: %w", err)
	}
	if len(output) > maxRemoteImagePreviewBytes {
		return RemoteImagePreview{}, errors.New("remote image is too large to preview")
	}
	mediaType, width, height, err := validatePreviewImage(output)
	if err != nil {
		return RemoteImagePreview{}, err
	}
	return RemoteImagePreview{Path: remotePath, Content: base64.StdEncoding.EncodeToString(output), MediaType: mediaType, Size: int64(len(output)), Width: width, Height: height}, nil
}

func (s *RemoteFileService) remoteFileRevision(ctx context.Context, alias, remotePath string) (string, error) {
	output, err := s.runNativeSSH(ctx, alias, "test -f "+shellQuote(remotePath)+" && stat -c '%s:%Y:%y:%i' -- "+shellQuote(remotePath), nil)
	if err != nil {
		return "", fmt.Errorf("read remote file revision: %w", err)
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "", errors.New("remote file revision is unavailable")
	}
	return revision, nil
}

func (s *RemoteFileService) remoteFileSize(ctx context.Context, alias, remotePath string) (int, error) {
	output, err := s.runNativeSSH(ctx, alias, "test -f "+shellQuote(remotePath)+" && stat -c '%s' -- "+shellQuote(remotePath), nil)
	if err != nil {
		return 0, fmt.Errorf("read remote file size: %w", err)
	}
	size, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || size < 0 {
		return 0, errors.New("remote file size is unavailable")
	}
	return size, nil
}

func remoteTextLanguage(remotePath string) string {
	name := strings.ToLower(pathpkg.Base(remotePath))
	if name == "dockerfile" {
		return "dockerfile"
	}
	if name == "makefile" {
		return "makefile"
	}
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return "shell"
	}
	if strings.Contains(name, "nginx") || strings.HasPrefix(name, "sshd_") {
		return "nginx"
	}
	extension := strings.TrimPrefix(strings.ToLower(pathpkg.Ext(name)), ".")
	languages := map[string]string{"txt": "plaintext", "log": "plaintext", "md": "markdown", "json": "json", "yaml": "yaml", "yml": "yaml", "toml": "toml", "xml": "xml", "html": "html", "css": "css", "js": "javascript", "jsx": "javascript", "ts": "typescript", "tsx": "typescript", "sh": "shell", "bash": "shell", "zsh": "shell", "py": "python", "go": "go", "java": "java", "c": "cpp", "h": "cpp", "cpp": "cpp", "sql": "sql", "ini": "ini", "conf": "ini", "cfg": "ini", "properties": "properties", "env": "shell"}
	if language, ok := languages[extension]; ok {
		return language
	}
	return "plaintext"
}

func validatePreviewImage(data []byte) (string, int, int, error) {
	if len(data) >= 12 && bytes.Equal(data[:4], []byte{'R', 'I', 'F', 'F'}) && bytes.Equal(data[8:12], []byte{'W', 'E', 'B', 'P'}) {
		width, height, err := webPDimensions(data)
		if err != nil {
			return "", 0, 0, err
		}
		return "image/webp", width, height, nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return "", 0, 0, errors.New("remote file is not a supported image")
	}
	mediaTypes := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}
	mediaType, ok := mediaTypes[format]
	if !ok {
		return "", 0, 0, errors.New("remote file is not a supported image")
	}
	return mediaType, config.Width, config.Height, nil
}

func webPDimensions(data []byte) (int, int, error) {
	if len(data) < 30 {
		return 0, 0, errors.New("remote WebP image is invalid")
	}
	switch string(data[12:16]) {
	case "VP8X":
		width := 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16
		height := 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16
		return width, height, nil
	case "VP8L":
		if data[20] != 0x2f {
			return 0, 0, errors.New("remote WebP image is invalid")
		}
		bits := uint32(data[21]) | uint32(data[22])<<8 | uint32(data[23])<<16 | uint32(data[24])<<24
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	case "VP8 ":
		if data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
			return 0, 0, errors.New("remote WebP image is invalid")
		}
		return int(data[26]) | int(data[27]&0x3f)<<8, int(data[28]) | int(data[29]&0x3f)<<8, nil
	default:
		return 0, 0, errors.New("remote WebP image is invalid")
	}
}

// CreateRemoteDirectory creates exactly one child directory in the supplied
// remote folder.  The basename restriction prevents a "new folder" action
// from unexpectedly navigating elsewhere in the remote filesystem.
func (s *RemoteFileService) CreateRemoteDirectory(ctx context.Context, alias, parentPath, name string) error {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return err
	}
	parentPath = normalizeRemotePath(parentPath)
	name = strings.TrimSpace(name)
	if !validRemoteBasename(name) {
		return errors.New("enter a single folder name without path separators")
	}
	targetPath := pathpkg.Join(parentPath, name)
	if _, err := s.runNativeSSH(ctx, alias, "mkdir -- "+shellQuote(targetPath), nil); err != nil {
		return fmt.Errorf("create remote folder: %w", err)
	}
	return nil
}

// EnsureRemoteDirectory creates a remote directory tree for uploads. Unlike
// CreateRemoteDirectory this accepts a full path because upload manifests keep
// their original nested directory structure.
func (s *RemoteFileService) EnsureRemoteDirectory(ctx context.Context, alias, remotePath string) error {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return err
	}
	remotePath = normalizeRemotePath(remotePath)
	if remotePath == "." {
		return errors.New("enter a remote directory")
	}
	if _, err := s.runNativeSSH(ctx, alias, "mkdir -p -- "+shellQuote(remotePath), nil); err != nil {
		return fmt.Errorf("create remote upload directory: %w", err)
	}
	return nil
}

func validRemoteBasename(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

// RemotePathExists is used before upload to make replacement an explicit user
// choice rather than allowing scp to overwrite a remote entry silently.
func (s *RemoteFileService) RemotePathExists(ctx context.Context, alias, remotePath string) (bool, error) {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return false, err
	}
	remotePath = normalizeRemotePath(remotePath)
	_, err := s.runNativeSSH(ctx, alias, "test -e "+shellQuote(remotePath)+" || test -L "+shellQuote(remotePath), nil)
	if err == nil {
		return true, nil
	}
	// A failed test is expected for a missing path.  Ask the shell to report
	// existence explicitly so SSH/connection failures still surface clearly.
	output, probeErr := s.runNativeSSH(ctx, alias, "if test -e "+shellQuote(remotePath)+" || test -L "+shellQuote(remotePath)+"; then printf yes; else printf no; fi", nil)
	if probeErr != nil {
		return false, fmt.Errorf("check remote path: %w", probeErr)
	}
	return string(output) == "yes", nil
}

// RenameRemoteFile moves one entry within its current directory. Keeping the
// new name to a basename makes this a rename operation rather than an
// unexpected move elsewhere on the remote machine.
func (s *RemoteFileService) RenameRemoteFile(ctx context.Context, alias, remotePath, newName string) error {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return err
	}
	remotePath, err := editableRemotePath(remotePath)
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == "." || newName == ".." || strings.Contains(newName, "/") {
		return errors.New("enter a valid file name")
	}
	targetPath := pathpkg.Join(pathpkg.Dir(remotePath), newName)
	if _, err := s.runNativeSSH(ctx, alias, "mv -- "+shellQuote(remotePath)+" "+shellQuote(targetPath), nil); err != nil {
		return fmt.Errorf("rename remote file: %w", err)
	}
	return nil
}

// DeleteRemoteFile removes a file or a directory tree after the desktop UI
// has presented an explicit confirmation. rm does not follow a symlink here;
// it removes the link itself.
func (s *RemoteFileService) DeleteRemoteFile(ctx context.Context, alias, remotePath string) error {
	return s.DeleteRemoteFiles(ctx, alias, []string{remotePath})
}

func (s *RemoteFileService) DeleteRemoteFiles(ctx context.Context, alias string, remotePaths []string) error {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return err
	}
	paths, err := editableRemotePaths(remotePaths)
	if err != nil {
		return err
	}
	if _, err := s.runNativeSSH(ctx, alias, "rm -rf -- "+shellQuoteList(paths), nil); err != nil {
		return fmt.Errorf("delete remote file: %w", err)
	}
	return nil
}

func (s *RemoteFileService) MoveRemoteFiles(ctx context.Context, alias string, remotePaths []string, destination string) error {
	if _, err := s.PrepareFileConnection(ctx, alias); err != nil {
		return err
	}
	paths, err := editableRemotePaths(remotePaths)
	if err != nil {
		return err
	}
	destination = normalizeRemotePath(destination)
	if destination == "." {
		return errors.New("enter a destination folder")
	}
	command := "test -d " + shellQuote(destination) + " && mv -- " + shellQuoteList(paths) + " " + shellQuote(destination)
	if _, err := s.runNativeSSH(ctx, alias, command, nil); err != nil {
		return fmt.Errorf("move remote files: %w", err)
	}
	return nil
}

func editableRemotePath(value string) (string, error) {
	value = normalizeRemotePath(value)
	if value == "." || value == "/" {
		return "", errors.New("the current root folder cannot be changed")
	}
	return value, nil
}

func editableRemotePaths(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("select at least one item")
	}
	paths := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		path, err := editableRemotePath(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths, nil
}

func shellQuoteList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, shellQuote(value))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func normalizeRemotePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "."
	}
	return pathpkg.Clean(value)
}
