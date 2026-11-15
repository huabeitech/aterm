package service

import (
	"bytes"
	"os"
	"sync"
	"testing"
)

func TestRestorePointsNeverReplaceEarlierBackups(t *testing.T) {
	app, _, _ := managedBackupService(t)
	previous := []byte("disposable encrypted fixture")
	if err := os.WriteFile(app.localVaultPath, previous, 0600); err != nil {
		t.Fatal(err)
	}
	const count = 12
	paths := make(chan string, count)
	failures := make(chan error, count)
	var callers sync.WaitGroup
	for i := 0; i < count; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			path, err := app.backupLocalVault()
			if err != nil {
				failures <- err
			} else {
				paths <- path
			}
		}()
	}
	callers.Wait()
	close(paths)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for path := range paths {
		if seen[path] {
			t.Fatal("a restore point was overwritten", path)
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, previous) {
			t.Fatal("restore point differs", err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("restore point permissions differ", err)
		}
	}
	if len(seen) != count {
		t.Fatal("missing restore points", len(seen))
	}
}
