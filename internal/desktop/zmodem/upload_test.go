package zmodem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenUploadsRetainsVerifiedSourceAndMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source 中文.bin")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := OpenUploads(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer files[0].Reader.Close()
	if files[0].Offer.Name != "source 中文.bin" || files[0].Offer.Size != 8 || !files[0].Offer.SizeKnown {
		t.Fatal(files[0].Offer)
	}
	// Rename, rather than overwrite the inode retained by the open descriptor.
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 8)
	if _, err = files[0].Reader.ReadAt(data, 0); err != nil || string(data) != "original" {
		t.Fatal("path replacement changed source", string(data), err)
	}
}
func TestOpenUploadsRejectsInvalidSelection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "first")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {"relative"}, {dir}, {path, filepath.Join(dir, "missing")}} {
		files, err := OpenUploads(context.Background(), paths)
		if err == nil || len(files) != 0 {
			t.Fatal("invalid selection accepted", paths, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenUploads(ctx, []string{path}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	files, err := OpenUploads(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	_ = files[0].Reader.Close()
	if _, err = files[0].Reader.ReadAt(make([]byte, 1), 0); err == nil || errors.Is(err, io.EOF) {
		t.Fatal("closed upload stayed readable", err)
	}
}
