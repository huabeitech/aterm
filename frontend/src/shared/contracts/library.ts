import type { domain } from './domain'
export interface LibraryApi {
  listHosts(): Promise<domain.Host[]>
  createHost(input: domain.HostInput): Promise<domain.Host>
  updateHost(id: string, input: domain.HostInput): Promise<domain.Host>
  deleteHosts(ids: string[]): Promise<void>
  moveHosts(ids: string[], directoryID: string): Promise<domain.Host[]>
  reorderHosts(scopeType: string, scopeID: string, ids: string[]): Promise<domain.Host[]>
  listDirectories(): Promise<domain.Directory[]>
  createDirectory(input: domain.DirectoryInput): Promise<domain.Directory>
  updateDirectory(id: string, input: domain.DirectoryInput): Promise<domain.Directory>
  moveDirectory(id: string, input: domain.DirectoryMoveInput): Promise<domain.Directory>
  deleteDirectory(id: string): Promise<void>
  previewSnippet(content: string): Promise<domain.HostSnippetPreview>
  importSnippet(
    input: domain.HostSnippetImportInput
  ): Promise<{ created: domain.Host[]; skipped: number }>
  vaultStatus(): Promise<{ localVaultAvailable: boolean; unlocked: boolean }>
  unlockVault(password: string, setup: boolean): Promise<void>
  getHostPassword(id: string): Promise<string>
  copyText(text: string): Promise<void>
  listManagedKeys(): Promise<domain.ManagedKey[]>
  createManagedKey(input: domain.ManagedKeyCreateInput): Promise<domain.ManagedKey>
  chooseManagedKeyImport(): Promise<domain.ManagedKeyImportPreview | null>
  importManagedKey(token: string, name: string, passphrase: string): Promise<domain.ManagedKey>
  exportManagedKey(id: string): Promise<boolean>
  addManagedKeyToAgent(id: string, passphrase: string): Promise<void>
  deleteManagedKey(id: string): Promise<void>
}
