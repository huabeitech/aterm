package terminal

import (
	"path/filepath"
	"ssh-connect/internal/db"
	"ssh-connect/internal/service"
	"testing"
)

// Real SSH tests use a disposable store, independent of application/UI fixtures.
func sshServiceFixture(t *testing.T) *service.AppService {
	t.Helper()
	directory := t.TempDir()
	store, err := db.Open(filepath.Join(directory, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return service.NewAppService(service.AppServiceOptions{Store: store, DatabasePath: filepath.Join(directory, "app.db"), SSHDir: filepath.Join(directory, "ssh"), LocalVaultPath: filepath.Join(directory, "vault.enc")})
}
