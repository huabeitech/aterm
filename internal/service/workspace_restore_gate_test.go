package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ssh-connect/internal/db"
)

type blockingRestoreStore struct {
	*db.Store
	started chan struct{}
	resume  chan struct{}
	calls   atomic.Int32
}

func (s *blockingRestoreStore) ImportHostConfigSnapshot(path string) (SyncSnapshotInfo, error) {
	if s.calls.Add(1) == 1 {
		close(s.started)
		<-s.resume
	}
	return s.Store.ImportHostConfigSnapshot(path)
}
func TestConcurrentRestoreWaitCanCancelWithoutTouchingActiveCommit(t *testing.T) {
	source, path := makeFileBackup(t)
	incoming, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	session, ok := source.currentVaultSessionSnapshot()
	if !ok {
		t.Fatal("fixture vault is locked")
	}
	defer clear(session.key)
	opened, err := source.vaultStore.Open(incoming, session.key)
	if err != nil {
		t.Fatal(err)
	}
	other, otherKey, err := source.vaultStore.Create("another-fixture-password", opened.Payload)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(otherKey)
	otherOpened, err := source.vaultStore.Open(other, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	app, _, store := managedBackupService(t)
	if _, err = app.SetUpSecureData(context.Background(), "previous-fixture-password"); err != nil {
		t.Fatal(err)
	}
	blocked := &blockingRestoreStore{Store: store, started: make(chan struct{}), resume: make(chan struct{})}
	app.store = blocked
	var once sync.Once
	unblock := func() { once.Do(func() { close(blocked.resume) }) }
	defer unblock()
	first := make(chan error, 1)
	go func() {
		_, err := app.installIncomingWorkspace(context.Background(), incoming, opened, session.key, nil)
		first <- err
	}()
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("first restore did not reach commit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err = app.installIncomingWorkspace(ctx, other, otherOpened, otherKey, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("queued restore ignored cancellation", err)
	}
	if blocked.calls.Load() != 1 {
		t.Fatal("second restore entered live database installation")
	}
	active, _ := os.ReadFile(app.localVaultPath)
	if !bytes.Equal(active, incoming) {
		t.Fatal("waiting restore replaced the active vault")
	}
	backups, _ := filepath.Glob(filepath.Join(app.backupDir, "vault-*.enc"))
	if len(backups) != 1 {
		t.Fatal("waiting restore created or overwrote restore points", backups)
	}
	unblock()
	select {
	case err = <-first:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("first restore did not finish")
	}
	saved, err := app.passwordStore.Get(VaultUnlockAccount)
	if err != nil {
		t.Fatal(err)
	}
	remembered, err := base64.StdEncoding.DecodeString(saved)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(remembered)
	if !bytes.Equal(remembered, session.key) {
		t.Fatal("queued restore replaced the active auto-unlock key")
	}
	current, ok := app.currentVaultSessionSnapshot()
	if !ok {
		t.Fatal("completed restore did not activate")
	}
	defer clear(current.key)
	if !bytes.Equal(current.key, session.key) || current.lastDownload == "" {
		t.Fatal("completion state does not match committed vault")
	}
}
