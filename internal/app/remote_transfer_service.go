package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ssh-connect/internal/service"
)

type remoteConnectionPreparer interface {
	PrepareFileConnection(context.Context, string) (service.Host, error)
	UploadRemotePath(context.Context, string, string, string) error
	DownloadRemotePath(context.Context, string, string, string) error
}

type remoteTransferService struct {
	ctx     context.Context
	files   remoteConnectionPreparer
	publish func(string, any)
	workers int
}

func newRemoteTransferService(ctx context.Context, files remoteConnectionPreparer, publish func(string, any)) *remoteTransferService {
	return &remoteTransferService{ctx: ctx, files: files, publish: publish, workers: maxConcurrentRemoteTransfers}
}

func (s *remoteTransferService) inspectLocalUploadPath(localPath string) (LocalUploadManifest, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return LocalUploadManifest{}, fmt.Errorf("inspect upload item: %w", err)
	}
	manifest := LocalUploadManifest{Name: info.Name(), IsDirectory: info.IsDir(), Files: []string{}, Directories: []string{}}
	if !info.IsDir() {
		manifest.Files = []string{""}
		return manifest, nil
	}
	err = filepath.WalkDir(localPath, func(entryPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entryPath == localPath {
			return nil
		}
		relativePath, err := filepath.Rel(localPath, entryPath)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			manifest.Directories = append(manifest.Directories, filepath.ToSlash(relativePath))
			return nil
		}
		if entry.Type().IsRegular() {
			manifest.Files = append(manifest.Files, filepath.ToSlash(relativePath))
		}
		return nil
	})
	if err != nil {
		return LocalUploadManifest{}, fmt.Errorf("scan upload folder: %w", err)
	}
	return manifest, nil
}

func (s *remoteTransferService) upload(alias, remotePath, localPath string) error {
	if _, err := s.files.PrepareFileConnection(s.ctx, alias); err != nil {
		return err
	}
	return s.files.UploadRemotePath(s.ctx, alias, remotePath, localPath)
}

func (s *remoteTransferService) downloadMany(alias string, remotePaths []string, localDirectory string) ([]remoteTransferResult, error) {
	if _, err := s.files.PrepareFileConnection(s.ctx, alias); err != nil {
		return nil, err
	}
	if strings.TrimSpace(localDirectory) == "" {
		return nil, errors.New("choose a local download folder")
	}
	results := make([]remoteTransferResult, len(remotePaths))
	if len(remotePaths) == 0 {
		return results, nil
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	var progressMu sync.Mutex
	completed := 0
	worker := func() {
		defer workers.Done()
		for index := range jobs {
			remotePath := strings.TrimSpace(remotePaths[index])
			result := remoteTransferResult{RemotePath: remotePath, Name: filepath.Base(remotePath)}
			if remotePath == "" {
				result.Error = "invalid remote path"
			} else if err := s.files.DownloadRemotePath(s.ctx, alias, remotePath, localDirectory); err != nil {
				result.Error = err.Error()
			} else {
				result.Success = true
			}
			results[index] = result
			progressMu.Lock()
			completed++
			if s.publish != nil {
				s.publish("remote-download-progress", remoteDownloadProgress{Completed: completed, Total: len(remotePaths), Name: result.Name, Alias: alias})
			}
			progressMu.Unlock()
		}
	}
	workerCount := min(s.workers, len(remotePaths))
	workers.Add(workerCount)
	for range workerCount {
		go worker()
	}
	for index := range remotePaths {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return results, nil
}
