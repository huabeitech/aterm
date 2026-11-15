package service

import (
	"context"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

func (s *AppService) UploadRemotePath(ctx context.Context, alias, remotePath, localPath string) (err error) {
	client, _, err := s.OpenNativeSSH(ctx, alias)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
		err = preferNativeSSHCause(client, err)
	}()
	files, err := sftp.NewClient(client.RawClient())
	if err != nil {
		return fmt.Errorf("start SFTP: %w", err)
	}
	defer files.Close()
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("inspect upload item: %w", err)
	}
	remotePath = normalizeRemotePath(remotePath)
	if !info.IsDir() {
		return uploadSFTPFile(files, localPath, pathpkg.Join(remotePath, filepath.Base(localPath)), info)
	}
	if err := files.MkdirAll(remotePath); err != nil {
		return fmt.Errorf("create remote directory: %w", err)
	}
	return filepath.WalkDir(localPath, func(localEntry string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(localPath, localEntry)
		if err != nil || relative == "." {
			return err
		}
		target := pathpkg.Join(remotePath, filepath.ToSlash(relative))
		if entry.IsDir() {
			return files.MkdirAll(target)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return uploadSFTPFile(files, localEntry, target, info)
	})
}

func uploadSFTPFile(files *sftp.Client, localPath, remotePath string, info os.FileInfo) error {
	source, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := files.MkdirAll(pathpkg.Dir(remotePath)); err != nil {
		return err
	}
	destination, err := files.Create(remotePath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	_ = files.Chmod(remotePath, info.Mode().Perm())
	return files.Chtimes(remotePath, info.ModTime(), info.ModTime())
}

func (s *AppService) DownloadRemotePath(ctx context.Context, alias, remotePath, localPath string) (err error) {
	client, _, err := s.OpenNativeSSH(ctx, alias)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
		err = preferNativeSSHCause(client, err)
	}()
	files, err := sftp.NewClient(client.RawClient())
	if err != nil {
		return fmt.Errorf("start SFTP: %w", err)
	}
	defer files.Close()
	remotePath = normalizeRemotePath(remotePath)
	info, err := files.Stat(remotePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if localInfo, statErr := os.Stat(localPath); statErr == nil && localInfo.IsDir() {
			localPath = filepath.Join(localPath, pathpkg.Base(remotePath))
		}
		return downloadSFTPFile(files, remotePath, localPath, info)
	}
	root := filepath.Join(localPath, pathpkg.Base(remotePath))
	if err := os.MkdirAll(root, info.Mode().Perm()); err != nil {
		return err
	}
	walker := files.Walk(remotePath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}
		current := walker.Path()
		if current == remotePath {
			continue
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(current, remotePath), "/")
		target := filepath.Join(root, filepath.FromSlash(relative))
		entryInfo := walker.Stat()
		if entryInfo.IsDir() {
			if err := os.MkdirAll(target, entryInfo.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		if entryInfo.Mode().IsRegular() {
			if err := downloadSFTPFile(files, current, target, entryInfo); err != nil {
				return err
			}
		}
	}
	return nil
}

func downloadSFTPFile(files *sftp.Client, remotePath, localPath string, info os.FileInfo) error {
	source, err := files.Open(remotePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
		return err
	}
	destination, err := os.OpenFile(localPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chtimes(localPath, info.ModTime(), info.ModTime())
}
