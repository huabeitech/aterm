package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	desktopfiles "ssh-connect/internal/desktop/files"
	desktopkeys "ssh-connect/internal/desktop/keys"
	"ssh-connect/internal/desktop/platform"
	desktopplugins "ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/desktop/prompts"
	desktopthemes "ssh-connect/internal/desktop/themes"
	"ssh-connect/internal/terminal"
	"strings"
	"sync"
	"sync/atomic"

	"ssh-connect/internal/db"
	"ssh-connect/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// App struct
type App struct {
	ctx                         context.Context
	wailsApp                    *application.App
	mainWindowRef               atomic.Pointer[mainWindowReference]
	mainWindowCreateMu          sync.Mutex
	mainWindowPlacement         *mainWindowPlacement
	store                       *db.Store
	svc                         *service.AppService
	terminal                    *terminal.Manager
	remoteTransfer              *remoteTransferService
	desktopMu                   sync.Mutex
	desktopPreferencesMu        sync.Mutex
	desktopLocaleRevision       atomic.Uint64
	desktopPreferencesRecovered atomic.Bool
	desktopAccessibilityStop    func()
	desktopFileManager          *desktopfiles.Manager
	desktopTransfers            *desktopfiles.Transfers
	desktopEditors              *editorWindows
	desktopKeys                 *desktopkeys.Store
	desktopPrompts              *prompts.Broker
	desktopConfirmations        map[string]pendingConfirmation
	desktopThemesStore          *desktopthemes.Store
	desktopPluginRegistry       *desktopplugins.Registry
	desktopPluginRuntime        *desktopplugins.RuntimeHost
	desktopPluginManager        *desktopplugins.Manager
	desktopPluginViews          *desktopplugins.ViewHost
	desktopPluginExecutor       *desktopplugins.Executor
	notifier                    *notifications.NotificationService
	notificationInitErr         error
}

type terminalUploadFile struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

type remoteTransferResult struct {
	RemotePath string `json:"remotePath"`
	Name       string `json:"name"`
	Success    bool   `json:"success"`
	Error      string `json:"error"`
}

type remoteDownloadProgress struct {
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Name      string `json:"name"`
	Alias     string `json:"alias"`
}

const maxConcurrentRemoteTransfers = 3

// LocalUploadManifest is deliberately expanded before a directory upload so
// the UI can report each file rather than treating an scp -r invocation as an
// opaque single task.
type LocalUploadManifest struct {
	Name        string   `json:"name"`
	IsDirectory bool     `json:"isDirectory"`
	Files       []string `json:"files"`
	Directories []string `json:"directories"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) setWailsApp(app *application.App) {
	a.wailsApp = app
}

func (a *App) setWindow(window application.Window) {
	if window == nil {
		a.mainWindowRef.Store(nil)
		return
	}
	a.mainWindowRef.Store(&mainWindowReference{window: window})
}

// ServiceStartup is called when the app service starts.
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	platform.AlignTrafficLights(40)
	a.notifier = notifications.New()
	a.notificationInitErr = a.notifier.ServiceStartup(ctx, application.ServiceOptions{})
	paths, err := defaultAppPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.appSupportDir, 0o700); err != nil {
		return err
	}
	store, err := db.Open(paths.databasePath)
	if err != nil {
		return err
	}
	a.store = store
	secretStore := service.NewSystemSecretStore(ctx)
	// An explicit data profile must isolate OS credentials as well as files.
	if os.Getenv("ATERM_DATA_DIR") != "" {
		profileHash := sha256.Sum256([]byte(filepath.Clean(paths.appSupportDir)))
		secretStore = service.ScopedSecretStore(secretStore, fmt.Sprintf("aterm-profile-%x", profileHash[:16]))
	}

	themeStore, err := desktopthemes.NewStore(filepath.Join(paths.appSupportDir, "themes", "packages"))
	if err != nil {
		store.Close()
		return err
	}
	a.desktopThemesStore = themeStore

	a.svc = service.NewAppService(service.AppServiceOptions{
		Store:               store,
		TunnelChanged:       func() { a.emitEvent("tunnels:changed", nil) },
		HostKeyStorePath:    filepath.Join(paths.appSupportDir, "ssh-host-keys.json"),
		PasswordStore:       secretStore,
		IdentityReader:      func(identity string) ([]byte, error) { return a.desktopKeys.ReadIdentity(identity) },
		IdentityNameReader:  func(identity string) string { return a.desktopKeys.IdentityName(identity) },
		CredentialPrompter:  a.credentialPrompter("files"),
		SSHDir:              paths.sshDir,
		ConfigPath:          paths.sshConfigPath,
		GeneratedConfigPath: paths.generatedConfigPath,
		DatabasePath:        paths.databasePath,
		SyncRootPath:        paths.syncRootPath,
		SyncSnapshotPath:    paths.syncSnapshotPath,
		SyncMetadataPath:    paths.syncMetadataPath,
		SyncVaultPath:       paths.syncVaultPath,
		LocalVaultPath:      paths.localVaultPath,
		BackupDir:           paths.backupDir,
		WailsContext:        ctx,
		HostKeyConfirmer: func(confirmCtx context.Context, key service.HostKeyInfo) (bool, error) {
			return a.confirmHostKey(confirmCtx, key), nil
		},
	})
	a.desktopKeys = desktopkeys.NewStore(a.svc.SyncService)
	a.terminal = terminal.NewManager(ctx, a.emitEvent)
	a.terminal.ConfigureTransfers(terminal.TransferOptions{Download: a.chooseZmodemDownload, Upload: a.chooseZmodemUploads})
	if err := a.initializeDesktopPlugins(ctx, filepath.Join(paths.appSupportDir, "plugins")); err != nil {
		a.desktopKeys.Close()
		store.Close()
		return err
	}

	a.remoteTransfer = newRemoteTransferService(ctx, a.svc, func(name string, payload any) {
		a.emitEvent(name, payload)
	})
	return nil
}

func (a *App) confirmHostKey(ctx context.Context, key service.HostKeyInfo) bool {
	title, message := hostKeyPrompt(key)
	label := "Trust and Connect"
	if key.PreviousFingerprint != "" {
		label = "Replace and Connect"
	}
	return a.confirmAction(ctx, windowConfirmation{
		Title: title, Message: message, ConfirmLabel: label,
	})
}

func (a *App) emitEvent(name string, payload any) {
	if a.wailsApp == nil {
		return
	}
	a.wailsApp.Event.Emit(name, payload)
	if output, ok := payload.(terminal.OutputEvent); ok && output.State != "" {
		if window := a.mainWindow(); a.desktopPluginManager != nil && window != nil {
			owner := window.ID()
			go a.desktopPluginManager.SessionChanged(owner, output.SessionID, false)
		}

		if session, err := a.desktopTerminalSession(output.SessionID); err == nil {
			a.wailsApp.Event.Emit("terminal:event", map[string]any{"type": "state", "session": session})
		}
	}
}

func (a *App) ServiceShutdown() error {
	a.mainWindowPlacement.close()
	a.stopDesktopAccessibility()
	platform.StopInputModes()
	if a.desktopPluginManager != nil {
		a.desktopPluginManager.Close()
	}
	if a.desktopPluginRuntime != nil {
		a.desktopPluginRuntime.Close()
	}
	if a.svc != nil {
		a.svc.CloseTunnels()
	}
	if a.desktopPrompts != nil {
		a.desktopPrompts.Close()
	}
	if a.desktopKeys != nil {
		a.desktopKeys.Close()
	}
	if a.desktopTransfers != nil {
		a.desktopTransfers.Close()
	}
	if a.desktopFileManager != nil {
		a.desktopFileManager.CloseAll()
	}
	if a.terminal != nil {
		a.terminal.CloseAll()
	}
	if a.notifier != nil && a.notificationInitErr == nil {
		_ = a.notifier.ServiceShutdown()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
	return nil
}

func (a *App) service() (*service.AppService, error) {
	if a.svc == nil {
		return nil, errors.New("application service is not initialized")
	}
	return a.svc, nil
}

type appPaths struct {
	appSupportDir       string
	databasePath        string
	backupDir           string
	syncRootPath        string
	syncSnapshotPath    string
	syncMetadataPath    string
	syncVaultPath       string
	localVaultPath      string
	sshDir              string
	sshConfigPath       string
	generatedConfigPath string
}

func defaultAppPaths() (appPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return appPaths{}, err
	}
	realUserHome := realUserHomeForExternalUserFiles(home)
	appSupportDir := filepath.Join(home, "Library", "Application Support", service.DataDirectoryName())
	syncRootDir := filepath.Join(realUserHome, "Library", "Mobile Documents", "com~apple~CloudDocs")
	sshDir := defaultSSHDirectory(realUserHome)
	// An explicit profile isolates development/testing from production databases,
	// vaults, cloud backups and SSH trust files without changing HOME.
	if profile := strings.TrimSpace(os.Getenv("ATERM_DATA_DIR")); profile != "" {
		if !filepath.IsAbs(profile) {
			return appPaths{}, errors.New("ATERM_DATA_DIR must be an absolute path")
		}
		appSupportDir = filepath.Clean(profile)
		syncRootDir = filepath.Join(appSupportDir, "cloud")
		sshDir = filepath.Join(appSupportDir, "ssh")
	}
	syncDir := filepath.Join(syncRootDir, service.CloudBackupDirectoryName, "sync")
	return appPaths{
		appSupportDir:       appSupportDir,
		databasePath:        filepath.Join(appSupportDir, "app.db"),
		localVaultPath:      filepath.Join(appSupportDir, "vault.enc"),
		backupDir:           filepath.Join(appSupportDir, "backups"),
		syncRootPath:        syncRootDir,
		syncSnapshotPath:    filepath.Join(syncDir, "app.db"),
		syncMetadataPath:    filepath.Join(syncDir, "metadata.json"),
		syncVaultPath:       filepath.Join(syncDir, "vault.enc"),
		sshDir:              sshDir,
		sshConfigPath:       filepath.Join(sshDir, "config"),
		generatedConfigPath: filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf"),
	}, nil
}

func defaultSSHDirectory(home string) string {
	return filepath.Join(realUserHomeForExternalUserFiles(home), ".ssh")
}

func realUserHomeForExternalUserFiles(home string) string {
	cleanHome := filepath.Clean(strings.TrimSpace(home))
	if cleanHome == "." || cleanHome == string(filepath.Separator) {
		return cleanHome
	}
	separator := string(filepath.Separator)
	marker := separator + "Library" + separator + "Containers" + separator
	markerIndex := strings.Index(cleanHome, marker)
	if markerIndex <= 0 {
		return cleanHome
	}
	remainder := cleanHome[markerIndex+len(marker):]
	parts := strings.Split(remainder, separator)
	if len(parts) != 2 || parts[0] == "" || parts[1] != "Data" {
		return cleanHome
	}
	return cleanHome[:markerIndex]
}
