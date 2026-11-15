package db

import (
	"path/filepath"
	"ssh-connect/internal/domain"
	"testing"
)

func TestWordNavigationMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.TerminalWordNavigation != "shell" {
		t.Fatalf("migration default: %q", settings.TerminalWordNavigation)
	}
	if _, err := store.UpdateSettings(domain.AppSettingsInput{TerminalWordNavigation: "terminal"}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err := reopened.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if saved.TerminalWordNavigation != "terminal" {
		t.Fatalf("persisted mode: %q", saved.TerminalWordNavigation)
	}
	if _, err := reopened.UpdateSettings(domain.AppSettingsInput{TerminalWordNavigation: "invalid"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
	saved, _ = reopened.GetSettings()
	if saved.TerminalWordNavigation != "terminal" {
		t.Fatal("invalid update changed stored mode")
	}
}
