package zmodem

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStagedDownloadPublishesOnlyOnCommit(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "received.bin")
	file, err := NewDownload(context.Background(), destination)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0, 1, 17, 24, 127, 255}, 8192)
	if _, err = file.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("download published before commit")
	}
	if err = file.Commit(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatal("committed data differs", err)
	}
	if err = file.Abort(); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(directory, ".aterm-*.zmodem"))
	if len(files) != 0 {
		t.Fatal("staged file leaked after commit", files)
	}
}
func TestStagedDownloadRefusesExistingAndLateCollision(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "existing")
	os.WriteFile(destination, []byte("preserve"), 0600)
	if _, err := NewDownload(context.Background(), destination); err == nil {
		t.Fatal("existing destination accepted")
	}
	other := filepath.Join(directory, "late")
	file, err := NewDownload(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	file.Write([]byte("new download"))
	os.WriteFile(other, []byte("other writer"), 0600)
	if err = file.Commit(); err == nil {
		t.Fatal("late collision replaced a file")
	}
	if err = file.Abort(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(other)
	if string(data) != "other writer" {
		t.Fatal("late collision data overwritten")
	}
	files, _ := filepath.Glob(filepath.Join(directory, ".aterm-*.zmodem"))
	if len(files) != 0 {
		t.Fatal("failed commit leaked partial file")
	}
}
func TestCanceledDownloadDiscardsPartialData(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "cancelled")
	ctx, cancel := context.WithCancel(context.Background())
	file, err := NewDownload(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	file.Write([]byte("partial"))
	cancel()
	if _, err = file.Write([]byte("late")); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled write accepted", err)
	}
	if err = file.Commit(); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled commit accepted", err)
	}
	if err = file.Abort(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("canceled file was retained", entries)
	}
}
