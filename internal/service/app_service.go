package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"ssh-connect/internal/domain"
	"ssh-connect/internal/ssh/hostkeys"
)

const IncludeLine = "Include ~/.ssh/config.d/*.conf"

var (
	ErrHostNotFound                       = domain.ErrHostNotFound
	ErrSSHDirectoryAuthorizationRequired  = errors.New("ssh directory authorization required")
	ErrUnexpectedSSHDirectory             = errors.New("selected directory is not the expected ssh directory")
	ErrSyncDirectoryAuthorizationRequired = errors.New("sync directory authorization required")
	ErrUnexpectedSyncDirectory            = errors.New("selected directory is not the expected sync directory")
	ErrConnectionFeaturesUnavailable      = errors.New("connection features unavailable in mac app store build")
)

type HostStore interface {
	ListHosts() ([]Host, error)
	CountHosts() (int, error)
	CreateHost(input HostInput) (Host, error)
	UpdateHost(id string, input HostInput) (Host, error)
	MoveHosts(ids []string, directoryID string) ([]Host, error)
	ReorderHosts(scopeType string, scopeID string, ids []string) ([]Host, error)
	MarkHostUsed(id string, usedAt string) (Host, error)
	DeleteHost(id string) error
	DeleteHosts(ids []string) error
	ListDirectories() ([]Directory, error)
	CreateDirectory(input DirectoryInput) (Directory, error)
	UpdateDirectory(id string, input DirectoryInput) (Directory, error)
	MoveDirectory(id string, input DirectoryMoveInput) (Directory, error)
	DeleteDirectory(id string) error
	GetSettings() (AppSettings, error)
	UpdateSettings(input AppSettingsInput) (AppSettings, error)
}

type HostKeyConfirmer func(ctx context.Context, key HostKeyInfo) (bool, error)

type HostKeyInfo struct {
	Host                string
	Port                string
	KeyType             string
	Fingerprint         string
	PreviousFingerprint string
}

type AppServiceOptions struct {
	Store                HostStore
	SSHDir               string
	ConfigPath           string
	GeneratedConfigPath  string
	DatabasePath         string
	SyncRootPath         string
	SyncSnapshotPath     string
	SyncMetadataPath     string
	SyncVaultPath        string
	LocalVaultPath       string
	BackupDir            string
	DeviceName           string
	ExpectedSSHDirectory string
	HostKeyConfirmer     HostKeyConfirmer
	HostKeyStorePath     string
	PasswordStore        PasswordStore
	IdentityReader       func(string) ([]byte, error)
	IdentityNameReader   func(string) string
	CredentialPrompter   CredentialPrompter
	WailsContext         context.Context
}

type AppService struct {
	*ConnectionPolicy
	*HostService
	*DirectoryService
	*SSHConfigService
	*SettingsService
	*ConnectionService
	*SyncService
	*RemoteFileService
	*HostImportService
	store                HostStore
	sshDir               string
	configPath           string
	generatedConfigPath  string
	databasePath         string
	syncRootPath         string
	syncSnapshotPath     string
	syncMetadataPath     string
	syncVaultPath        string
	localVaultPath       string
	backupDir            string
	deviceName           string
	expectedSSHDirectory string
	openSSHExportMu      sync.Mutex
	hostKeyConfirmer     HostKeyConfirmer
	hostKeyStore         *hostkeys.Store
	passwordStore        PasswordStore
	identityReader       func(string) ([]byte, error)
	identityNameReader   func(string) string
	credentialPrompter   CredentialPrompter
	wailsContext         context.Context
	hostPasswordStore    *switchablePasswordStore
	vaultStore           *VaultStore
	vaultSession         *vaultSession
	encryptedSyncMu      sync.Mutex
	workspaceRestoreGate workspaceRestoreGate
	autoUnlockResult     AutoUnlockResult
}

func NewAppService(options AppServiceOptions) *AppService {
	service := &AppService{
		ConnectionPolicy:     NewConnectionPolicy(true),
		store:                options.Store,
		sshDir:               options.SSHDir,
		configPath:           options.ConfigPath,
		generatedConfigPath:  options.GeneratedConfigPath,
		databasePath:         options.DatabasePath,
		syncRootPath:         options.SyncRootPath,
		syncSnapshotPath:     options.SyncSnapshotPath,
		syncMetadataPath:     options.SyncMetadataPath,
		syncVaultPath:        options.SyncVaultPath,
		localVaultPath:       options.LocalVaultPath,
		backupDir:            options.BackupDir,
		deviceName:           options.DeviceName,
		expectedSSHDirectory: strings.TrimSpace(options.ExpectedSSHDirectory),
		hostKeyConfirmer:     options.HostKeyConfirmer,
		passwordStore:        options.PasswordStore,
		identityReader:       options.IdentityReader,
		identityNameReader:   options.IdentityNameReader,
		credentialPrompter:   options.CredentialPrompter,
		wailsContext:         options.WailsContext,
	}
	if options.HostKeyStorePath != "" {
		service.hostKeyStore = hostkeys.New(options.HostKeyStorePath)
	}
	if service.passwordStore == nil {
		service.passwordStore = newPasswordStore(service.wailsContext)
	}
	service.hostPasswordStore = newSwitchablePasswordStore(lockedPasswordStore{})
	service.vaultStore = NewVaultStore()
	service.SSHConfigService = newSSHConfigService(service)
	service.SettingsService = &SettingsService{AppService: service}
	service.ConnectionService = &ConnectionService{AppService: service}
	service.SyncService = &SyncService{AppService: service}
	service.RemoteFileService = &RemoteFileService{AppService: service}
	service.HostImportService = &HostImportService{AppService: service}
	service.HostService = NewHostService(options.Store, service.hostPasswordStore)
	service.DirectoryService = NewDirectoryService(options.Store)
	return service
}

func (s *AppService) Status(ctx context.Context) (AppStatus, error) {
	if err := ctx.Err(); err != nil {
		return AppStatus{}, err
	}
	count, err := s.store.CountHosts()
	if err != nil {
		return AppStatus{}, err
	}
	status := AppStatus{
		DatabasePath:                s.databasePath,
		HostCount:                   count,
		MacAppStoreBuild:            macAppStoreBuild,
		ConnectionFeaturesAvailable: true,
		LocalTerminalAvailable:      LocalTerminalAvailable(),
	}
	paths, done, err := s.authorizedSSHPaths(ctx)
	if err != nil {
		status.SSHDirectoryPath = s.settingsSSHDirectoryPath()
		status.SSHDirectoryAuthorizationRequired = true
		status.SSHDirectoryAuthorizationErrorText = err.Error()
		return status, nil
	}
	defer done()
	status.SSHDirectoryPath = paths.sshDir
	status.SSHDirectoryAuthorized = true
	return status, nil
}
