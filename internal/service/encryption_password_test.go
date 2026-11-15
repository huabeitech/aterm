package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestEncryptionPasswordsPreserveSpacesAndSharedFormLength(t *testing.T) {
	for _, password := range []string{"        ", "  fixture-password  ", "密码测试密码测试", "🔐🔐🔐🔐"} {
		t.Run(password, func(t *testing.T) {
			app, _, _ := managedBackupService(t)
			ctx := context.Background()
			if _, err := app.SetUpSecureData(ctx, password); err != nil {
				t.Fatal("shared-form-valid password rejected", err)
			}
			path := filepath.Join(t.TempDir(), "backup.enc")
			if _, err := app.ExportEncryptedBackup(ctx, path); err != nil {
				t.Fatal(err)
			}
			destination, _, _ := managedBackupService(t)
			if _, err := destination.RestoreEncryptedBackup(ctx, path, password, func(SyncMetadata) bool { return true }); err != nil {
				t.Fatal("exact password could not restore", err)
			}
			if _, err := app.ChangeEncryptionPassword(ctx, password, "  next-fixture-password  "); err != nil {
				t.Fatal("password change normalized current secret", err)
			}
			data, err := readEncryptedBackup(app.localVaultPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, key, err := app.vaultStore.Unlock(data, "  next-fixture-password  "); err != nil {
				t.Fatal("new secret was normalized", err)
			} else {
				clear(key)
			}
		})
	}
	app, _, _ := managedBackupService(t)
	if _, err := app.SetUpSecureData(context.Background(), "四个汉字"); !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatal("UTF-8 byte count accepted a password rejected by the React form", err)
	}
}
