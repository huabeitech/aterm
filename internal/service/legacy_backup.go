package service

import (
	"encoding/json"
	"errors"
	"os"

	"ssh-connect/internal/desktop"
	"ssh-connect/internal/desktop/legacybackup"
	"ssh-connect/internal/domain"
)

type preparedBackup struct {
	Encoded  []byte
	Opened   OpenedVault
	Key      []byte
	Metadata map[string]string
}

func (s *SyncService) prepareIncomingBackup(data []byte, password string) (preparedBackup, error) {
	if !validEncryptionPassword(password, 1) {
		return preparedBackup{}, ErrInvalidVaultPassword
	}
	if !legacybackup.Recognizes(data) {
		opened, key, err := s.openIncomingVault(data, password)
		return preparedBackup{Encoded: data, Opened: opened, Key: key}, err
	}
	workspace, err := legacybackup.Decode(data, password)
	if err != nil {
		if errors.Is(err, legacybackup.ErrPassword) {
			err = ErrInvalidVaultPassword
		}
		return preparedBackup{}, err
	}
	defer clear(workspace.CompatibleKey)
	if len(workspace.CompatibleEncoded) > 0 {
		if _, err = s.vaultStore.Open(workspace.CompatibleEncoded, workspace.CompatibleKey); err != nil {
			return preparedBackup{}, err
		}
	}
	builder, ok := s.store.(interface {
		BuildHostConfigSnapshot(string, []domain.Host, []domain.Directory, []domain.HostSortOrder) (SyncSnapshotInfo, error)
	})
	if !ok {
		return preparedBackup{}, errors.New("legacy backup snapshot conversion is unavailable")
	}
	file, err := s.createVaultTempFile("legacy-backup-*.db")
	if err != nil {
		return preparedBackup{}, err
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)
	if _, err = builder.BuildHostConfigSnapshot(path, workspace.Hosts, workspace.Directories, workspace.SortOrders); err != nil {
		return preparedBackup{}, err
	}
	snapshot, err := os.ReadFile(path)
	if err != nil {
		return preparedBackup{}, err
	}
	payload := VaultPayload{SchemaVersion: vaultSchemaVersion, Snapshot: snapshot, Passwords: workspace.Passwords, SourceDevice: "Earlier Electron backup", HostCount: len(workspace.Hosts), DirectoryCount: len(workspace.Directories)}
	encoded, key, err := s.vaultStore.Create(password, payload)
	if err != nil {
		return preparedBackup{}, err
	}
	opened, err := s.vaultStore.Open(encoded, key)
	if err != nil {
		clear(key)
		return preparedBackup{}, err
	}
	preferences, err := json.Marshal(workspace.Preferences)
	if err != nil {
		clear(key)
		return preparedBackup{}, err
	}
	return preparedBackup{Encoded: encoded, Opened: opened, Key: key, Metadata: map[string]string{desktop.PreferencesMetadataKey: string(preferences)}}, nil
}
