import type { ThemeID } from './themes'

export namespace domain {
  export interface ManagedKey {
    id: string
    name: string
    algorithm: 'ed25519' | 'rsa' | 'ecdsa'
    publicKey: string
    fingerprint: string
    passphraseProtected: boolean
    createdAt: string
  }
  export interface ManagedKeyCreateInput {
    name: string
    algorithm: ManagedKey['algorithm']
    passphrase: string
  }
  /** Portable private material; only used inside an authenticated encrypted backup payload. */
  export interface ManagedKeyBackup extends ManagedKey {
    privateKey: string
  }
  export interface ManagedKeyImportPreview {
    token: string
    sourceName: string
    suggestedName: string
    algorithm: ManagedKey['algorithm'] | ''
    fingerprint: string
    requiresPassphrase: boolean
  }
  export interface Host {
    id: string
    directoryID: string
    alias: string
    hostName: string
    user: string
    port: number
    identityFile: string
    passwordAuth: boolean
    proxyJump: string
    jumpHostID: string
    forwardAgent: boolean
    compression: boolean
    serverAliveInterval: number
    serverAliveCountMax: number
    connectTimeout: number
    remark: string
    favorite: boolean
    lastUsedAt: string
    sortOrders: Record<string, number>
    createdAt: string
    updatedAt: string
  }

  export type HostInput = Omit<
    Host,
    'id' | 'lastUsedAt' | 'sortOrders' | 'createdAt' | 'updatedAt'
  > & { password: string }

  export interface Directory {
    id: string
    parentID: string
    name: string
    sortOrder: number
    createdAt: string
    updatedAt: string
  }

  export interface DirectoryInput {
    parentID: string
    name: string
  }
  export interface DirectoryMoveInput {
    parentID: string
    sortOrder: number
  }

  export interface AppSettings {
    language: import('./locale').LanguagePreference
    appFontScale: number
    lightTheme?: ThemeID
    darkTheme?: ThemeID
    theme: ThemeID
    defaultHome: string
    commandPaletteShortcut: string
    commandSearchShortcut: string
    toggleSidebarShortcut: string
    toggleTerminalFilesShortcut: string
    newTerminalShortcut: string
    closeTerminalShortcut: string
    previousTerminalShortcut: string
    nextTerminalShortcut: string
    moveTabLeftShortcut: string
    moveTabRightShortcut: string
    terminalFontSize: number
    terminalWordNavigation: 'shell' | 'terminal'
    splitVerticalShortcut: string
    splitHorizontalShortcut: string
    sshDirectoryPath: string
    sshDirectoryBookmark: string
    sshDirectoryBookmarkCreatedAt: string
    syncDirectoryPath: string
    syncDirectoryBookmark: string
    syncDirectoryBookmarkCreatedAt: string
  }

  export type AppSettingsInput = AppSettings

  export interface AppStatus {
    databasePath: string
    macAppStoreBuild: boolean
    connectionFeaturesAvailable: boolean
    localTerminalAvailable: boolean
    sshDirectoryPath: string
    sshDirectoryAuthorized: boolean
    sshDirectoryAuthorizationRequired: boolean
    sshDirectoryAuthorizationErrorText: string
    hostCount: number
  }

  export interface OpenSSHExportStatus {
    sshConfigPath: string
    generatedConfigPath: string
    includeConfigured: boolean
    snapshotExists: boolean
    lastExportedAt: string
  }

  export interface SyncMetadata {
    schemaVersion: number
    appName: string
    snapshotCreatedAt: string
    sourceDeviceName: string
    hostCount: number
    directoryCount: number
    lastUpload: string
    lastDownload: string
  }

  export interface SyncStatus {
    cloudAvailable: boolean
    snapshotAvailable: boolean
    message: string
    snapshotPath: string
    metadataPath: string
    syncDirectoryPath: string
    syncDirectoryAuthorized: boolean
    syncDirectoryAuthorizationRequired: boolean
    syncDirectoryAuthorizationErrorText: string
    lastUpload: string
    lastDownload: string
    metadata?: SyncMetadata
  }

  export interface EncryptedSyncStatus {
    cloudAvailable: boolean
    vaultAvailable: boolean
    backupUpdateRequired: boolean
    localVaultAvailable: boolean
    unlocked: boolean
    autoUnlockStatus: string
    autoUnlockMessage?: string
    autoUnlockRepairRequired: boolean
    message: string
    vaultPath: string
    localVaultPath: string
    syncDirectoryPath: string
    syncDirectoryAuthorized: boolean
    syncDirectoryAuthorizationRequired: boolean
    syncDirectoryAuthorizationErrorText: string
    lastUpload: string
    lastDownload: string
    metadata?: SyncMetadata
  }

  export interface EncryptedSyncResult {
    hostCount: number
    directoryCount: number
    lastUpload: string
    lastDownload: string
    backupPath: string
    backupUpdated: boolean
    backupUpdateRequired: boolean
  }

  export interface RemoteFile {
    name: string
    path: string
    isDirectory: boolean
    kind: string
    size: number
    modifiedAt: string
    createdAt: string
    accessedAt: string
    changedAt: string
    permissions: string
    owner: string
    group: string
    linkTarget: string
    inode: number
    linkCount: number
  }

  export interface HostSnippetImportInput {
    content: string
    rowIDs: string[]
  }
  export type HostSnippetStatus =
    | 'aliasIsRequired'
    | 'multipleAliasesAreNotSupported'
    | 'wildcardHostIsNotSupported'
    | 'invalidPort'
    | 'invalidOptionValue'
    | 'alreadyExists'
    | 'duplicateInSnippet'
    | 'missingHostname'
    | 'readyToAdd'
  export interface HostSnippetPreviewRow {
    statusKey: HostSnippetStatus
    rowID: string
    alias: string
    hostName: string
    user: string
    port: number
    identityFile: string
    proxyJump: string
    status: string
    selectable: boolean
    input: HostInput
  }
  export interface HostSnippetPreview {
    content: string
    rows: HostSnippetPreviewRow[]
  }
}
