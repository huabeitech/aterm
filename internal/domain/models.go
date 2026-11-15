package domain

// Host is a persisted SSH host record.
type Host struct {
	ID                  string         `json:"id"`
	DirectoryID         string         `json:"directoryID"`
	Alias               string         `json:"alias"`
	HostName            string         `json:"hostName"`
	User                string         `json:"user"`
	Port                int            `json:"port"`
	IdentityFile        string         `json:"identityFile"`
	PasswordAuth        bool           `json:"passwordAuth"`
	ProxyJump           string         `json:"proxyJump"`
	JumpHostID          string         `json:"jumpHostID"`
	ForwardAgent        bool           `json:"forwardAgent"`
	Compression         bool           `json:"compression"`
	ServerAliveInterval int            `json:"serverAliveInterval"`
	ServerAliveCountMax int            `json:"serverAliveCountMax"`
	ConnectTimeout      int            `json:"connectTimeout"`
	Remark              string         `json:"remark"`
	Favorite            bool           `json:"favorite"`
	LastUsedAt          string         `json:"lastUsedAt"`
	SortOrders          map[string]int `json:"sortOrders"`
	CreatedAt           string         `json:"createdAt"`
	UpdatedAt           string         `json:"updatedAt"`
}

// HostInput is the frontend payload for creating or updating a host.
type HostInput struct {
	DirectoryID         string `json:"directoryID"`
	Alias               string `json:"alias"`
	HostName            string `json:"hostName"`
	User                string `json:"user"`
	Port                int    `json:"port"`
	IdentityFile        string `json:"identityFile"`
	PasswordAuth        bool   `json:"passwordAuth"`
	Password            string `json:"password"`
	ProxyJump           string `json:"proxyJump"`
	JumpHostID          string `json:"jumpHostID"`
	ForwardAgent        bool   `json:"forwardAgent"`
	Compression         bool   `json:"compression"`
	ServerAliveInterval int    `json:"serverAliveInterval"`
	ServerAliveCountMax int    `json:"serverAliveCountMax"`
	ConnectTimeout      int    `json:"connectTimeout"`
	Remark              string `json:"remark"`
	Favorite            bool   `json:"favorite"`
}

// Directory is a persisted sidebar directory record.
type Directory struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentID"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// DirectoryInput is the frontend payload for creating or updating a directory.
type DirectoryInput struct {
	ParentID string `json:"parentID"`
	Name     string `json:"name"`
}

// DirectoryMoveInput is the frontend payload for changing a directory's tree position.
type DirectoryMoveInput struct {
	ParentID  string `json:"parentID"`
	SortOrder int    `json:"sortOrder"`
}

const (
	HostSortScopeAll           = "all"
	HostSortScopeFavorites     = "favorites"
	HostSortScopeDirectory     = "directory"
	HostSortScopeUncategorized = "uncategorized"
)

// HostSortOrder stores a host's display order in a specific view scope.
type HostSortOrder struct {
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeID"`
	HostID    string `json:"hostID"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type AppSettings struct {
	Theme                          string `json:"theme"`
	TerminalTheme                  string `json:"terminalTheme"`
	DefaultHome                    string `json:"defaultHome"`
	CommandPaletteShortcut         string `json:"commandPaletteShortcut"`
	NewTerminalShortcut            string `json:"newTerminalShortcut"`
	CloseTerminalShortcut          string `json:"closeTerminalShortcut"`
	PreviousTerminalShortcut       string `json:"previousTerminalShortcut"`
	NextTerminalShortcut           string `json:"nextTerminalShortcut"`
	MoveTabLeftShortcut            string `json:"moveTabLeftShortcut"`
	MoveTabRightShortcut           string `json:"moveTabRightShortcut"`
	TerminalWordNavigation         string `json:"terminalWordNavigation"`
	TerminalFontSize               int    `json:"terminalFontSize"`
	SplitVerticalShortcut          string `json:"splitVerticalShortcut"`
	SplitHorizontalShortcut        string `json:"splitHorizontalShortcut"`
	SSHDirectoryPath               string `json:"sshDirectoryPath"`
	SSHDirectoryBookmark           string `json:"sshDirectoryBookmark"`
	SSHDirectoryBookmarkCreatedAt  string `json:"sshDirectoryBookmarkCreatedAt"`
	SyncDirectoryPath              string `json:"syncDirectoryPath"`
	SyncDirectoryBookmark          string `json:"syncDirectoryBookmark"`
	SyncDirectoryBookmarkCreatedAt string `json:"syncDirectoryBookmarkCreatedAt"`
}

type AppSettingsInput struct {
	Theme                          string `json:"theme"`
	TerminalTheme                  string `json:"terminalTheme"`
	DefaultHome                    string `json:"defaultHome"`
	CommandPaletteShortcut         string `json:"commandPaletteShortcut"`
	NewTerminalShortcut            string `json:"newTerminalShortcut"`
	CloseTerminalShortcut          string `json:"closeTerminalShortcut"`
	PreviousTerminalShortcut       string `json:"previousTerminalShortcut"`
	NextTerminalShortcut           string `json:"nextTerminalShortcut"`
	MoveTabLeftShortcut            string `json:"moveTabLeftShortcut"`
	MoveTabRightShortcut           string `json:"moveTabRightShortcut"`
	TerminalWordNavigation         string `json:"terminalWordNavigation"`
	TerminalFontSize               int    `json:"terminalFontSize"`
	SplitVerticalShortcut          string `json:"splitVerticalShortcut"`
	SplitHorizontalShortcut        string `json:"splitHorizontalShortcut"`
	SSHDirectoryPath               string `json:"sshDirectoryPath"`
	SSHDirectoryBookmark           string `json:"sshDirectoryBookmark"`
	SSHDirectoryBookmarkCreatedAt  string `json:"sshDirectoryBookmarkCreatedAt"`
	SyncDirectoryPath              string `json:"syncDirectoryPath"`
	SyncDirectoryBookmark          string `json:"syncDirectoryBookmark"`
	SyncDirectoryBookmarkCreatedAt string `json:"syncDirectoryBookmarkCreatedAt"`
}

// RemoteFile is a single entry in an SSH host's remote directory.
type RemoteFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"isDirectory"`
	Kind        string `json:"kind"`
	Size        int64  `json:"size"`
	ModifiedAt  string `json:"modifiedAt"`
	CreatedAt   string `json:"createdAt"`
	AccessedAt  string `json:"accessedAt"`
	ChangedAt   string `json:"changedAt"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	LinkTarget  string `json:"linkTarget"`
	Inode       int64  `json:"inode"`
	LinkCount   int64  `json:"linkCount"`
}

// RemoteTextFile is intentionally limited to small text files so that editing
// configuration files remains predictable in the desktop UI.
type RemoteTextFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	Revision string `json:"revision"`
	Language string `json:"language"`
}

// RemoteImagePreview is a validated, bounded image payload intended solely for
// display in the desktop preview surface.
type RemoteImagePreview struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type AppStatus struct {
	DatabasePath                       string `json:"databasePath"`
	MacAppStoreBuild                   bool   `json:"macAppStoreBuild"`
	ConnectionFeaturesAvailable        bool   `json:"connectionFeaturesAvailable"`
	LocalTerminalAvailable             bool   `json:"localTerminalAvailable"`
	SSHDirectoryPath                   string `json:"sshDirectoryPath"`
	SSHDirectoryAuthorized             bool   `json:"sshDirectoryAuthorized"`
	SSHDirectoryAuthorizationRequired  bool   `json:"sshDirectoryAuthorizationRequired"`
	SSHDirectoryAuthorizationErrorText string `json:"sshDirectoryAuthorizationErrorText"`
	HostCount                          int    `json:"hostCount"`
}

type OpenSSHExportStatus struct {
	SSHConfigPath       string `json:"sshConfigPath"`
	GeneratedConfigPath string `json:"generatedConfigPath"`
	IncludeConfigured   bool   `json:"includeConfigured"`
	SnapshotExists      bool   `json:"snapshotExists"`
	LastExportedAt      string `json:"lastExportedAt"`
}

type SyncSnapshotInfo struct {
	HostCount      int    `json:"hostCount"`
	DirectoryCount int    `json:"directoryCount"`
	UpdatedAt      string `json:"updatedAt"`
}

type SyncMetadata struct {
	SchemaVersion     int    `json:"schemaVersion"`
	AppName           string `json:"appName"`
	SnapshotCreatedAt string `json:"snapshotCreatedAt"`
	SourceDeviceName  string `json:"sourceDeviceName"`
	HostCount         int    `json:"hostCount"`
	DirectoryCount    int    `json:"directoryCount"`
	LastUpload        string `json:"lastUpload"`
	LastDownload      string `json:"lastDownload"`
}

type SyncStatus struct {
	CloudAvailable                      bool          `json:"cloudAvailable"`
	SnapshotAvailable                   bool          `json:"snapshotAvailable"`
	Message                             string        `json:"message"`
	SnapshotPath                        string        `json:"snapshotPath"`
	MetadataPath                        string        `json:"metadataPath"`
	SyncDirectoryPath                   string        `json:"syncDirectoryPath"`
	SyncDirectoryAuthorized             bool          `json:"syncDirectoryAuthorized"`
	SyncDirectoryAuthorizationRequired  bool          `json:"syncDirectoryAuthorizationRequired"`
	SyncDirectoryAuthorizationErrorText string        `json:"syncDirectoryAuthorizationErrorText"`
	LastUpload                          string        `json:"lastUpload"`
	LastDownload                        string        `json:"lastDownload"`
	Metadata                            *SyncMetadata `json:"metadata"`
}

type SyncResult struct {
	HostCount      int    `json:"hostCount"`
	DirectoryCount int    `json:"directoryCount"`
	LastUpload     string `json:"lastUpload"`
	LastDownload   string `json:"lastDownload"`
	BackupPath     string `json:"backupPath"`
}

type EncryptedSyncStatus struct {
	CloudAvailable                      bool          `json:"cloudAvailable"`
	VaultAvailable                      bool          `json:"vaultAvailable"`
	BackupUpdateRequired                bool          `json:"backupUpdateRequired"`
	LocalVaultAvailable                 bool          `json:"localVaultAvailable"`
	Unlocked                            bool          `json:"unlocked"`
	AutoUnlockStatus                    string        `json:"autoUnlockStatus"`
	AutoUnlockMessage                   string        `json:"autoUnlockMessage,omitempty"`
	AutoUnlockRepairRequired            bool          `json:"autoUnlockRepairRequired"`
	Message                             string        `json:"message"`
	VaultPath                           string        `json:"vaultPath"`
	LocalVaultPath                      string        `json:"localVaultPath"`
	SyncDirectoryPath                   string        `json:"syncDirectoryPath"`
	SyncDirectoryAuthorized             bool          `json:"syncDirectoryAuthorized"`
	SyncDirectoryAuthorizationRequired  bool          `json:"syncDirectoryAuthorizationRequired"`
	SyncDirectoryAuthorizationErrorText string        `json:"syncDirectoryAuthorizationErrorText"`
	LastUpload                          string        `json:"lastUpload"`
	LastDownload                        string        `json:"lastDownload"`
	Metadata                            *SyncMetadata `json:"metadata"`
}

type EncryptedSyncResult struct {
	HostCount            int    `json:"hostCount"`
	DirectoryCount       int    `json:"directoryCount"`
	LastUpload           string `json:"lastUpload"`
	LastDownload         string `json:"lastDownload"`
	BackupPath           string `json:"backupPath"`
	BackupUpdated        bool   `json:"backupUpdated"`
	BackupUpdateRequired bool   `json:"backupUpdateRequired"`
}

type AutoUnlockResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

const (
	HostSnippetStatusReady                      = "Ready to add"
	HostSnippetStatusAlreadyExists              = "Already exists"
	HostSnippetStatusMissingHostName            = "Missing HostName"
	HostSnippetStatusWildcardUnsupported        = "Wildcard host is not supported"
	HostSnippetStatusMultipleAliasesUnsupported = "Multiple aliases are not supported"
	HostSnippetStatusDuplicateInSnippet         = "Duplicate in snippet"
	HostSnippetStatusInvalidPort                = "Invalid port"
	HostSnippetStatusInvalidOptionValue         = "Invalid option value"
)

type HostSnippetPreviewRow struct {
	RowID        string    `json:"rowID"`
	Alias        string    `json:"alias"`
	HostName     string    `json:"hostName"`
	User         string    `json:"user"`
	Port         int       `json:"port"`
	IdentityFile string    `json:"identityFile"`
	ProxyJump    string    `json:"proxyJump"`
	Status       string    `json:"status"`
	Selectable   bool      `json:"selectable"`
	Input        HostInput `json:"input"`
}

type HostSnippetPreview struct {
	Content string                  `json:"content"`
	Rows    []HostSnippetPreviewRow `json:"rows"`
}

type HostSnippetImportInput struct {
	Content string   `json:"content"`
	RowIDs  []string `json:"rowIDs"`
}

type HostSnippetImportResult struct {
	Created []Host `json:"created"`
	Skipped int    `json:"skipped"`
}
