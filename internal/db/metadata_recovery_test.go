package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataRecoveryArchivesExactValueAndReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	original := "  { broken settings\n"
	if err := store.SetAppMetadata("preferences", original); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.RecoverAppMetadata("preferences", original, `{"theme":"system"}`)
	if err != nil || !recovered {
		t.Fatal(recovered, err)
	}
	var key, value string
	if err := store.db.QueryRow(`SELECT key,value FROM app_metadata WHERE key LIKE 'preferences.recovery-%'`).Scan(&key, &value); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "preferences.recovery-") || value != original {
		t.Fatal("archive did not preserve exact original", key, value)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.GetAppMetadata("preferences"); err != nil || got != `{"theme":"system"}` {
		t.Fatal(got, err)
	}
	if got, err := reopened.GetAppMetadata(key); err != nil || got != original {
		t.Fatal(got, err)
	}
}
func TestMetadataRecoveryDoesNotOverwriteNewerSaveOrDuplicateArchive(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetAppMetadata("preferences", "newer valid save"); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		recovered, err := store.RecoverAppMetadata("preferences", "old invalid value", "defaults")
		if err != nil || recovered {
			t.Fatal(recovered, err)
		}
	}
	if got, err := store.GetAppMetadata("preferences"); err != nil || got != "newer valid save" {
		t.Fatal(got, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM app_metadata WHERE key LIKE 'preferences.recovery-%'`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
func TestMetadataRecoveryArchiveFailureRollsBackReplacement(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetAppMetadata("preferences", "bad"); err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`CREATE TRIGGER fail_recovery BEFORE INSERT ON app_metadata WHEN NEW.key LIKE 'preferences.recovery-%' BEGIN SELECT RAISE(ABORT,'archive unavailable'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := store.RecoverAppMetadata("preferences", "bad", "defaults"); err == nil || recovered {
		t.Fatal(recovered, err)
	}
	if got, err := store.GetAppMetadata("preferences"); err != nil || got != "bad" {
		t.Fatal("original lost after archive failure", got, err)
	}
}
