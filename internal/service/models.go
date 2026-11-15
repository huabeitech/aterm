package service

import "ssh-connect/internal/domain"

type Host = domain.Host
type HostInput = domain.HostInput
type Directory = domain.Directory
type DirectoryInput = domain.DirectoryInput
type DirectoryMoveInput = domain.DirectoryMoveInput
type HostSortOrder = domain.HostSortOrder
type AppSettings = domain.AppSettings
type AppSettingsInput = domain.AppSettingsInput
type RemoteFile = domain.RemoteFile
type RemoteTextFile = domain.RemoteTextFile
type RemoteImagePreview = domain.RemoteImagePreview
type AppStatus = domain.AppStatus
type OpenSSHExportStatus = domain.OpenSSHExportStatus
type SyncSnapshotInfo = domain.SyncSnapshotInfo
type SyncMetadata = domain.SyncMetadata
type SyncStatus = domain.SyncStatus
type SyncResult = domain.SyncResult
type EncryptedSyncStatus = domain.EncryptedSyncStatus
type EncryptedSyncResult = domain.EncryptedSyncResult
type AutoUnlockResult = domain.AutoUnlockResult
type HostSnippetPreviewRow = domain.HostSnippetPreviewRow
type HostSnippetPreview = domain.HostSnippetPreview
type HostSnippetImportInput = domain.HostSnippetImportInput
type HostSnippetImportResult = domain.HostSnippetImportResult

const (
	HostSortScopeAll           = domain.HostSortScopeAll
	HostSortScopeFavorites     = domain.HostSortScopeFavorites
	HostSortScopeDirectory     = domain.HostSortScopeDirectory
	HostSortScopeUncategorized = domain.HostSortScopeUncategorized

	HostSnippetStatusReady                      = domain.HostSnippetStatusReady
	HostSnippetStatusAlreadyExists              = domain.HostSnippetStatusAlreadyExists
	HostSnippetStatusMissingHostName            = domain.HostSnippetStatusMissingHostName
	HostSnippetStatusWildcardUnsupported        = domain.HostSnippetStatusWildcardUnsupported
	HostSnippetStatusMultipleAliasesUnsupported = domain.HostSnippetStatusMultipleAliasesUnsupported
	HostSnippetStatusDuplicateInSnippet         = domain.HostSnippetStatusDuplicateInSnippet
	HostSnippetStatusInvalidPort                = domain.HostSnippetStatusInvalidPort
	HostSnippetStatusInvalidOptionValue         = domain.HostSnippetStatusInvalidOptionValue
)
