import type { domain } from './domain'
export interface IntegrationStatus {
  app: domain.AppStatus
  sync: domain.EncryptedSyncStatus
  openssh: domain.OpenSSHExportStatus
}
export interface IntegrationApi {
  status(): Promise<IntegrationStatus>
  chooseDirectory(kind: 'ssh' | 'sync' | 'icloud'): Promise<IntegrationStatus>
  autoUnlock(enable: boolean): Promise<IntegrationStatus>
  changePassword(current: string, next: string): Promise<IntegrationStatus>
  backup(file: boolean): Promise<IntegrationStatus>
  restore(password: string, file: boolean): Promise<IntegrationStatus>
  deleteBackup(): Promise<IntegrationStatus>
  exportSSH(): Promise<IntegrationStatus>
}
