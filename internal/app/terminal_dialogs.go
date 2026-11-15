package app

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"ssh-connect/internal/desktop/zmodem"
)

func (a *App) chooseZmodemDownload(ctx context.Context, _ string, offer zmodem.FileOffer) (zmodem.Download, error) {
	type response struct {
		path string
		err  error
	}
	chosen := make(chan response, 1)
	go func() {
		path, err := a.saveFileDialog(application.SaveFileDialogOptions{Title: "Save ZMODEM Download", Filename: offer.Name, CanCreateDirectories: true})
		chosen <- response{path, err}
	}()
	select {
	case result := <-chosen:
		if result.err != nil {
			return nil, result.err
		}
		if result.path == "" {
			return nil, context.Canceled
		}
		return zmodem.NewDownload(ctx, result.path)
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (a *App) chooseZmodemUploads(ctx context.Context, _ string) ([]zmodem.Upload, error) {
	type response struct {
		paths []string
		err   error
	}
	chosen := make(chan response, 1)
	go func() {
		paths, err := a.openMultipleFilesDialog(application.OpenFileDialogOptions{Title: "Send Files with ZMODEM"})
		chosen <- response{paths, err}
	}()
	select {
	case result := <-chosen:
		if result.err != nil {
			return nil, result.err
		}
		return zmodem.OpenUploads(ctx, result.paths)
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}
