import { platformFeatures } from '../../../../shared/platformFeatures'
import { PluginManagement } from '../../features/plugins/PluginManagement'
import { ThemeManagement } from './ThemeManagement'
import { languageOptions } from '../../../../shared/contracts/locale'
import { localizedError } from '../../../../shared/i18n/errors'
import { t, i18n } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useId, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  Puzzle,
  Settings2,
  KeyRound,
  ShieldCheck,
  Cloud,
  Keyboard,
  Info,
  ExternalLink,
  MoreHorizontal
} from 'lucide-react'
import {
  ActionButton,
  ActionMenu,
  SelectField,
  TextField,
  type MenuAction
} from '../ui/workbench-controls'
import { desktop } from '../../api/app'
import { BrowserOpenURL } from '../../api/runtime'
import { domain as service } from '../../api/types'
import { ScrollArea } from '../common/ScrollArea'
import { type BusyAction } from '../common/types'
import {
  formatCommandPaletteShortcut,
  shortcutFromKeyboardEvent
} from '../common/commandPaletteShortcut'
import { appFontScaleOptions } from '../../../../shared/contracts/preferences'
import { ManagedKeysSection } from './ManagedKeysSection'
import './settings.css'

type SettingsPageProps = {
  platform: string
  onExportBackup: () => void
  onRestoreBackup: () => void
  shortcutSectionRequest?: number
  settings: service.AppSettings | null
  version: string
  status: service.AppStatus | null
  openSSHExportStatus: service.OpenSSHExportStatus | null
  syncStatus: service.EncryptedSyncStatus | null
  busyAction: BusyAction
  onChange: (settings: service.AppSettings) => Promise<void>
  onExportOpenSSHConfig: () => void
  onAuthorizeSSHDirectory: () => void
  onAuthorizeSyncDirectory: () => void
  onUploadSync: () => void
  onDownloadSync: () => void
  onAutoUnlockAction: () => void
  onChangeEncryptionPassword: () => void
  onDeleteICloudBackup: () => void
}

export function SettingsPage({
  platform,
  onExportBackup,
  onRestoreBackup,
  shortcutSectionRequest = 0,
  settings,
  version,
  status,
  openSSHExportStatus,
  syncStatus,
  busyAction,
  onChange,
  onExportOpenSSHConfig,
  onAuthorizeSSHDirectory,
  onAuthorizeSyncDirectory,
  onUploadSync,
  onDownloadSync,
  onAutoUnlockAction,
  onChangeEncryptionPassword,
  onDeleteICloudBackup
}: SettingsPageProps) {
  useTranslation()
  const features = platformFeatures(platform)
  const sections = [
    { key: 'general', label: t('settings:general'), icon: Settings2 },
    { key: 'ssh', label: 'SSH', icon: KeyRound },
    { key: 'security', label: t('settings:security'), icon: ShieldCheck },
    { key: 'backup', label: t('settings:backup'), icon: Cloud },
    { key: 'shortcuts', label: t('settings:shortcuts'), icon: Keyboard },
    { key: 'plugins', label: t('settings:pluginsTitle'), icon: Puzzle },
    { key: 'about', label: t('settings:about'), icon: Info }
  ] as const
  const shortcutGroups = [
    {
      title: t('settings:navigation'),
      items: [
        ['commandPaletteShortcut', t('settings:openHostSearch')],
        ['commandSearchShortcut', t('settings:openCommandPalette')],
        ['toggleSidebarShortcut', t('hosts:toggleSidebar')],
        ['toggleTerminalFilesShortcut', t('commands:toggleTerminalFiles')],
        ['previousTerminalShortcut', t('settings:previousWorkspaceTab')],
        ['nextTerminalShortcut', t('settings:nextWorkspaceTab')],
        ['moveTabLeftShortcut', t('settings:moveCurrentTabLeft')],
        ['moveTabRightShortcut', t('settings:moveCurrentTabRight')]
      ]
    },
    {
      title: t('settings:terminalWorkspace'),
      items: [
        ['newTerminalShortcut', t('settings:newTerminalTab')],
        ['closeTerminalShortcut', t('settings:closeWorkspaceTab')],
        ['splitVerticalShortcut', t('settings:splitTerminalVertically')],
        ['splitHorizontalShortcut', t('settings:splitTerminalHorizontally')]
      ]
    }
  ] as const

  const [activeSection, setActiveSection] = useState<(typeof sections)[number]['key']>('general')
  useEffect(() => {
    if (shortcutSectionRequest) setActiveSection('shortcuts')
  }, [shortcutSectionRequest])
  const [saveError, setSaveError] = useState<{ field: string; message: string } | null>(null)
  const busy = Boolean(busyAction)
  const macAppStoreBuild = status?.macAppStoreBuild === true
  const authorizationRequired = Boolean(
    macAppStoreBuild && syncStatus?.syncDirectoryAuthorizationRequired
  )
  const autoUnlockNotice = getAutoUnlockNotice(syncStatus)
  const current = sections.find((section) => section.key === activeSection)!
  const moreActions: MenuAction[] = [
    ...(macAppStoreBuild
      ? [
          { key: 'authorize', label: t('settings:reauthorizeIcloudDrive') },
          { type: 'divider' as const }
        ]
      : []),
    {
      key: 'delete',
      label: t('settings:deleteIcloudBackup'),
      danger: true,
      disabled: authorizationRequired || !syncStatus?.vaultAvailable
    }
  ]
  async function change(field: string, next: service.AppSettings) {
    setSaveError(null)
    try {
      await onChange(next)
    } catch (error) {
      setSaveError({
        field,
        message:
          error instanceof Error ? localizedError(error) : t('settings:unableToSavePleaseTryAgain')
      })
    }
  }
  const errorFor = (field: string) => (saveError?.field === field ? saveError.message : undefined)

  return (
    <section className="settings-page" aria-label={t('settings:settings')}>
      <nav className="settings-tabs" aria-label={t('settings:settingsSections')}>
        <div className="settings-nav-title">{t('settings:settings')}</div>
        {sections.map(({ key, label, icon: Icon }) => (
          <button
            key={key}
            type="button"
            className={key === activeSection ? 'is-selected' : undefined}
            aria-current={key === activeSection ? 'page' : undefined}
            onClick={() => setActiveSection(key)}
          >
            <Icon size={16} strokeWidth={1.6} />
            <span>{label}</span>
          </button>
        ))}
      </nav>
      <div className="settings-detail">
        <header className="settings-toolbar">
          <h1>{current.label}</h1>
          <span role="status">{busyAction === 'settings' ? t('settings:saving') : ''}</span>
        </header>
        <ScrollArea className="settings-scroll" key={activeSection}>
          <div className="settings-content">
            {!settings ? (
              <p className="settings-footnote">{t('settings:loadingPreferences')}</p>
            ) : (
              <>
                {activeSection === 'plugins' && (
                  <PluginManagement macAppStoreBuild={macAppStoreBuild} />
                )}
                {activeSection === 'general' && (
                  <>
                    <SettingsGroup
                      title={t('settings:language')}
                      footer={t('settings:languageHelp')}
                    >
                      <SettingRow label={t('settings:language')} error={errorFor('language')}>
                        <SelectField
                          aria-label={t('settings:language')}
                          data-testid="language-select"
                          value={settings.language}
                          disabled={busy}
                          options={[
                            { value: 'system', label: t('settings:system') },
                            ...languageOptions
                          ]}
                          onValueChange={(language) =>
                            void change('language', {
                              ...settings,
                              language: language as service.AppSettings['language']
                            })
                          }
                        />
                      </SettingRow>
                    </SettingsGroup>
                    {!macAppStoreBuild && (
                      <SettingsGroup title={t('settings:startup')}>
                        <SettingRow label={t('settings:onLaunch')} error={errorFor('defaultHome')}>
                          <SelectField
                            aria-label={t('settings:defaultHome')}
                            value={settings.defaultHome}
                            disabled={busy}
                            options={[
                              { label: t('settings:home'), value: 'hosts' },
                              { label: t('settings:newLocalTerminal'), value: 'local-terminal' }
                            ]}
                            onValueChange={(defaultHome) =>
                              void change('defaultHome', { ...settings, defaultHome })
                            }
                          />
                        </SettingRow>
                      </SettingsGroup>
                    )}
                    <SettingsGroup title={t('settings:appearance')} footer="">
                      <SettingRow
                        label={t('settings:interfaceTextSize')}
                        error={errorFor('appFontScale')}
                      >
                        <SelectField
                          aria-label={t('settings:interfaceTextSize2')}
                          options={appFontScaleOptions.map((scale) => ({
                            label: scale === 100 ? t('settings:100Default') : `${scale}%`,
                            value: String(scale)
                          }))}
                          value={String(settings.appFontScale)}
                          disabled={busy}
                          onValueChange={(scale) =>
                            void change('appFontScale', {
                              ...settings,
                              appFontScale: Number(scale)
                            })
                          }
                        />
                      </SettingRow>
                    </SettingsGroup>
                    <SettingsGroup title={t('settings:manageThemes')}>
                      <ThemeManagement settings={settings} onChange={onChange} />
                    </SettingsGroup>
                    <SettingsGroup title={t('settings:terminal')} footer="">
                      <SettingRow
                        label={t('settings:fontSize')}
                        error={errorFor('terminalFontSize')}
                      >
                        <FontSizeField
                          value={settings.terminalFontSize}
                          disabled={busy}
                          onCommit={(terminalFontSize) =>
                            void change('terminalFontSize', { ...settings, terminalFontSize })
                          }
                        />
                      </SettingRow>
                      <SettingRow
                        label={t('settings:optionAltArrows')}
                        error={errorFor('terminalWordNavigation')}
                      >
                        <SelectField
                          aria-label={t('settings:optionAltArrowBehavior')}
                          options={[
                            { label: t('settings:moveByWordInShell'), value: 'shell' },
                            { label: t('settings:terminalDefault'), value: 'terminal' }
                          ]}
                          value={settings.terminalWordNavigation}
                          disabled={busy}
                          onValueChange={(value) =>
                            void change('terminalWordNavigation', {
                              ...settings,
                              terminalWordNavigation:
                                value as service.AppSettings['terminalWordNavigation']
                            })
                          }
                        />
                      </SettingRow>
                    </SettingsGroup>
                  </>
                )}
                {activeSection === 'ssh' && (
                  <>
                    <ManagedKeysSection
                      macAppStoreBuild={macAppStoreBuild}
                      unlocked={!!syncStatus?.unlocked}
                    />
                    <SettingsGroup
                      title={t('settings:directoryAccess')}
                      footer={
                        sshDirectoryMessage(status) ||
                        t('settings:allowTerminalAndSftpToUseKeysAndKnownHostsInYourSshDirectory')
                      }
                    >
                      <SettingRow label={t('settings:sshDirectory')}>
                        <span className="settings-value settings-path">
                          {sshDirectoryLabel(status)}
                        </span>
                      </SettingRow>
                      <SettingRow label={t('settings:directoryPermission')}>
                        <ActionButton
                          busy={busyAction === 'sshDirectory'}
                          disabled={busy}
                          onClick={onAuthorizeSSHDirectory}
                        >
                          {status?.sshDirectoryAuthorized
                            ? t('settings:reauthorize')
                            : t('settings:grantAccess')}
                        </ActionButton>
                      </SettingRow>
                    </SettingsGroup>
                    {features.openSSHExport && (
                      <SettingsGroup
                        title={t('settings:opensshExport')}
                        footer={t('settings:exportDescription', {
                          path:
                            openSSHExportStatus?.generatedConfigPath ||
                            '~/.ssh/config.d/ssh-connect.generated.conf'
                        })}
                      >
                        <SettingRow label={t('settings:status')}>
                          <span className="settings-value">
                            {openSSHSetupLabel(status, openSSHExportStatus)}
                          </span>
                        </SettingRow>
                        <SettingRow label={t('settings:lastExport')}>
                          <span className="settings-value">
                            {lastOpenSSHExportLabel(openSSHExportStatus)}
                          </span>
                        </SettingRow>
                        <SettingRow label={t('settings:exportCurrentHosts')}>
                          <ActionButton
                            busy={busyAction === 'export'}
                            disabled={busy}
                            onClick={onExportOpenSSHConfig}
                          >
                            {t('settings:exportHosts')}
                          </ActionButton>
                        </SettingRow>
                      </SettingsGroup>
                    )}
                  </>
                )}
                {activeSection === 'security' && (
                  <>
                    <SettingsGroup
                      title={t('settings:dataProtection')}
                      footer={t('settings:savedPasswordsAreProtectedWithYourEncryptionPassword')}
                    >
                      <SettingRow label={t('settings:encryption')}>
                        <span className="settings-value">{protectionLabel(syncStatus)}</span>
                      </SettingRow>
                      <SettingRow label={t('settings:encryptionPassword')}>
                        <ActionButton
                          disabled={busy || !syncStatus?.localVaultAvailable}
                          onClick={onChangeEncryptionPassword}
                        >
                          {t('settings:changePassword')}
                        </ActionButton>
                      </SettingRow>
                    </SettingsGroup>
                    <SettingsGroup
                      title={t('settings:autoUnlock')}
                      footer={
                        autoUnlockNotice?.description ||
                        t('settings:thisDeviceUsesTheSystemCredentialStoreToUnlockProtectedData')
                      }
                    >
                      <SettingRow label={t('settings:onThisDevice')}>
                        <span className="settings-value">{autoUnlockLabel(syncStatus)}</span>
                      </SettingRow>
                      {autoUnlockNotice?.actionLabel && (
                        <SettingRow label={autoUnlockNotice.title}>
                          <ActionButton disabled={busy} onClick={onAutoUnlockAction}>
                            {autoUnlockNotice.actionLabel}
                          </ActionButton>
                        </SettingRow>
                      )}
                    </SettingsGroup>
                    {autoUnlockNotice && autoUnlockNotice.type !== 'info' && (
                      <p className="settings-inline-error" role="alert">
                        {autoUnlockNotice.title}
                      </p>
                    )}
                  </>
                )}
                {activeSection === 'backup' && !features.cloudBackup && (
                  <SettingsGroup
                    title={t('settings:backup')}
                    footer={t(
                      'settings:backupsAreEncryptedYourEncryptionPasswordIsRequiredToRestoreThem'
                    )}
                  >
                    <SettingRow label={t('native:exportEncryptedBackup')}>
                      <ActionButton disabled={busy} onClick={onExportBackup}>
                        {t('security:export')}
                      </ActionButton>
                    </SettingRow>
                    <SettingRow label={t('native:restoreEncryptedBackup')}>
                      <ActionButton disabled={busy} onClick={onRestoreBackup}>
                        {t('settings:restore2')}
                      </ActionButton>
                    </SettingRow>
                  </SettingsGroup>
                )}
                {activeSection === 'backup' && features.cloudBackup && (
                  <>
                    <SettingsGroup
                      title={t('settings:icloudBackup')}
                      footer={t(
                        'settings:backupsAreEncryptedYourEncryptionPasswordIsRequiredToRestoreThem'
                      )}
                    >
                      <SettingRow label={t('settings:status')}>
                        <span className="settings-value">
                          {syncAvailabilityLabel(syncStatus, macAppStoreBuild)}
                        </span>
                      </SettingRow>
                      <SettingRow label={t('settings:lastBackup')}>
                        <span className="settings-value">{lastSyncLabel(syncStatus)}</span>
                      </SettingRow>
                      {syncStatus?.metadata && (
                        <>
                          <SettingRow label={t('settings:sourceDevice')}>
                            <span className="settings-value">
                              {syncStatus.metadata.sourceDeviceName || t('settings:unknownDevice')}
                            </span>
                          </SettingRow>
                          <SettingRow label={t('settings:contents')}>
                            <span className="settings-value">
                              {t('settings:backupContents', {
                                hosts: syncStatus.metadata.hostCount,
                                directories: syncStatus.metadata.directoryCount
                              })}
                            </span>
                          </SettingRow>
                        </>
                      )}
                      {authorizationRequired ? (
                        <SettingRow label={t('settings:icloudDriveAccess')}>
                          <ActionButton
                            disabled={busy}
                            busy={busyAction === 'syncDirectory'}
                            onClick={onAuthorizeSyncDirectory}
                          >
                            {t('settings:grantAccess')}
                          </ActionButton>
                        </SettingRow>
                      ) : null}
                      <SettingRow label={t('settings:backUpCurrentData')}>
                        <ActionButton
                          variant="default"
                          busy={busyAction === 'sync'}
                          disabled={busy || authorizationRequired}
                          onClick={onUploadSync}
                        >
                          {t('settings:backUpNow')}
                        </ActionButton>
                      </SettingRow>
                    </SettingsGroup>
                    {syncStatus &&
                      (authorizationRequired ||
                        !syncStatus.cloudAvailable ||
                        syncStatus.backupUpdateRequired) && (
                        <p className="settings-footnote" role="status">
                          {syncStatusMessage(syncStatus, macAppStoreBuild)}
                        </p>
                      )}
                    <SettingsGroup
                      title={t('settings:restore')}
                      footer={t('settings:restoreDescription')}
                    >
                      <SettingRow label={t('settings:restoreFromIcloud')}>
                        <ActionButton
                          disabled={busy || authorizationRequired || !syncStatus?.vaultAvailable}
                          onClick={onDownloadSync}
                        >
                          {t('settings:restore2')}
                        </ActionButton>
                      </SettingRow>
                    </SettingsGroup>
                    <div className="settings-management">
                      <span>{t('settings:manageBackup')}</span>
                      <ActionMenu
                        actions={moreActions}
                        onAction={({ key }) =>
                          key === 'authorize' ? onAuthorizeSyncDirectory() : onDeleteICloudBackup()
                        }
                      >
                        <ActionButton
                          aria-label={t('settings:manageBackup')}
                          icon={<MoreHorizontal size={16} />}
                          disabled={busy}
                        />
                      </ActionMenu>
                    </div>
                  </>
                )}
                {activeSection === 'shortcuts' && (
                  <>
                    <p className="settings-intro">{t('settings:shortcutRecorderHelp')}</p>
                    {shortcutGroups.map((group) => (
                      <SettingsGroup key={group.title} title={group.title}>
                        {group.items.map(([field, label]) => (
                          <SettingRow key={field} label={label} error={errorFor(field)}>
                            <ShortcutRecorder
                              value={settings[field]}
                              disabled={busy}
                              ariaLabel={t('settings:shortcutLabel', { label })}
                              invalid={Boolean(errorFor(field))}
                              onChange={(value) =>
                                void change(field, { ...settings, [field]: value })
                              }
                            />
                          </SettingRow>
                        ))}
                      </SettingsGroup>
                    ))}
                  </>
                )}
                {activeSection === 'about' && (
                  <>
                    <div className="settings-about">
                      <div className="settings-app-name">aTerm</div>
                      <p>{t('settings:version', { version })}</p>
                      <p>{t('settings:sshSftpTerminal')}</p>
                    </div>
                    <SettingsGroup title={t('settings:helpLegal')}>
                      {[
                        [t('settings:support'), 'https://aterm.huabei.pro/support'],
                        [t('settings:privacyPolicy'), 'https://aterm.huabei.pro/privacy'],
                        [t('settings:termsOfUse'), 'https://aterm.huabei.pro/terms']
                      ].map(([label, url]) => (
                        <button
                          key={url}
                          type="button"
                          className="settings-link-row"
                          onClick={() => BrowserOpenURL(url)}
                        >
                          <span>{label}</span>
                          <ExternalLink size={13} />
                        </button>
                      ))}
                    </SettingsGroup>
                  </>
                )}
              </>
            )}
          </div>
        </ScrollArea>
      </div>
    </section>
  )
}

function SettingsGroup({
  title,
  footer,
  children
}: {
  title: string
  footer?: string
  children: ReactNode
}) {
  const id = useId()
  return (
    <section className="settings-group" aria-labelledby={id}>
      <h2 id={id}>{title}</h2>
      <div className="settings-group-rows">{children}</div>
      {footer && <p className="settings-footnote">{footer}</p>}
    </section>
  )
}

function SettingRow({
  label,
  children,
  error
}: {
  label: string
  children: ReactNode
  error?: string
}) {
  return (
    <div className="settings-row">
      <span className="settings-row-label">{label}</span>
      <div className="settings-row-control">{children}</div>
      {error && (
        <p className="settings-inline-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

function FontSizeField({
  value,
  disabled,
  onCommit
}: {
  value: number
  disabled: boolean
  onCommit: (value: number) => void
}) {
  useTranslation()
  const [draft, setDraft] = useState(String(value))
  const [error, setError] = useState('')
  useEffect(() => setDraft(String(value)), [value])
  const commit = () => {
    const next = Number(draft)
    if (!draft || !Number.isInteger(next) || next < 10 || next > 24) {
      setError(t('settings:enterAWholeNumberFrom10To24'))
      return
    }
    setError('')
    if (next !== value) onCommit(next)
  }
  return (
    <div className="settings-font-field">
      <div className="settings-font-input">
        <TextField
          aria-label={t('settings:terminalFontSize')}
          type="number"
          min={10}
          max={24}
          step={1}
          value={draft}
          disabled={disabled}
          aria-invalid={Boolean(error)}
          onChange={(event) => {
            setDraft(event.target.value)
            setError('')
          }}
          onBlur={commit}
          onEnter={(event) => event.currentTarget.blur()}
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              setDraft(String(value))
              setError('')
              event.preventDefault()
            }
          }}
        />
        <span>pt</span>
      </div>
      {error && (
        <p className="settings-inline-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

function ShortcutRecorder({
  value,
  disabled,
  ariaLabel,
  invalid,
  onChange
}: {
  value: string
  disabled: boolean
  ariaLabel: string
  invalid: boolean
  onChange: (shortcut: string) => void
}) {
  useTranslation()
  const [recording, setRecording] = useState(false)
  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Tab') return
    event.preventDefault()
    if (event.key === 'Escape') {
      event.currentTarget.blur()
      return
    }
    const shortcut = shortcutFromKeyboardEvent(event)
    if (shortcut) {
      setRecording(false)
      onChange(shortcut)
    }
  }
  return (
    <TextField
      aria-label={ariaLabel}
      className="shortcut-recorder"
      readOnly
      value={recording ? t('settings:typeShortcut') : formatCommandPaletteShortcut(value)}
      disabled={disabled}
      aria-invalid={invalid}
      onFocus={() => {
        setRecording(true)
        desktop.setInputMode('recorder')
      }}
      onBlur={() => {
        setRecording(false)
        desktop.setInputMode('normal')
      }}
      onKeyDown={handleKeyDown}
    />
  )
}

function sshDirectoryLabel(status: service.AppStatus | null) {
  if (!status) return t('settings:checking')
  if (status.sshDirectoryAuthorized) return status.sshDirectoryPath || t('settings:authorized')
  return t('settings:notAuthorized')
}

function sshDirectoryMessage(status: service.AppStatus | null) {
  const text = status?.sshDirectoryAuthorizationErrorText ?? ''
  if (!text) return ''
  if (text.includes('selected directory is not the expected ssh directory')) {
    return t('settings:selectYourSshDirectoryToGrantDirectoryAccess')
  }
  if (text.includes('ssh directory authorization required')) {
    return t('settings:authorizeAccessToYourSshDirectoryToContinue')
  }
  return text
}

function openSSHSetupLabel(
  status: service.AppStatus | null,
  exportStatus: service.OpenSSHExportStatus | null
) {
  if (!status) return t('settings:checking')
  if (status.sshDirectoryAuthorizationRequired) return t('settings:directoryAccessRequired')
  if (!exportStatus) return t('settings:checking')
  if (exportStatus.includeConfigured && exportStatus.snapshotExists) return t('settings:exported')
  if (exportStatus.snapshotExists) return t('settings:snapshotOnly')
  if (exportStatus.includeConfigured) return t('settings:incomplete')
  return t('settings:neverExported')
}

function lastOpenSSHExportLabel(status: service.OpenSSHExportStatus | null) {
  if (!status?.lastExportedAt) return t('settings:neverExported')
  const timestamp = new Date(status.lastExportedAt)
  if (Number.isNaN(timestamp.getTime())) return status.lastExportedAt
  return timestamp.toLocaleString(i18n.language)
}

function syncAvailabilityLabel(
  status: service.EncryptedSyncStatus | null,
  macAppStoreBuild: boolean
) {
  if (!status) return t('settings:checking')
  if (macAppStoreBuild && status.syncDirectoryAuthorizationRequired)
    return t('settings:authorizationRequired')
  if (status.backupUpdateRequired) return t('settings:updateRequired')
  if (status.vaultAvailable) return t('settings:available')
  if (status.cloudAvailable) return t('settings:noBackup')
  return t('settings:unavailable')
}

function protectionLabel(status: service.EncryptedSyncStatus | null) {
  if (!status) return t('settings:checking')
  if (status.unlocked) return t('settings:protected')
  if (status.localVaultAvailable) return t('settings:locked')
  return t('settings:notSetUp')
}

function autoUnlockLabel(status: service.EncryptedSyncStatus | null) {
  if (!status) return t('settings:checking')
  if (status.autoUnlockStatus === 'unlocked') return t('settings:ready')
  if (status.autoUnlockStatus === 'skipped_no_key') return t('settings:notEnabled')
  if (status.autoUnlockRepairRequired) return t('settings:needsAttention')
  if (status.autoUnlockStatus === 'keychain_unavailable') return t('settings:unavailable')
  if (status.autoUnlockStatus === 'vault_corrupted') return t('settings:failed')
  return status.localVaultAvailable ? t('settings:notReady') : t('settings:notSetUp')
}

function getAutoUnlockNotice(status: service.EncryptedSyncStatus | null): {
  type: 'info' | 'warning' | 'error'
  title: string
  description: string
  actionLabel?: string
} | null {
  if (!status) return null
  if (status.autoUnlockStatus === 'skipped_no_key') {
    return {
      type: 'info',
      title: t('settings:enableAutoUnlock'),
      description: t('settings:enableAutoUnlockDescription'),
      actionLabel: t('settings:enableAutoUnlock2')
    }
  }
  if (status.autoUnlockStatus === 'keychain_denied') {
    return {
      type: 'warning',
      title: t('settings:autoUnlockNeedsAttention'),
      description:
        status.autoUnlockMessage ||
        t('settings:atermCouldNotAccessItsAutoUnlockKeyInTheSystemCredentialStore'),
      actionLabel: t('settings:tryAgain')
    }
  }
  if (status.autoUnlockStatus === 'invalid_key') {
    return {
      type: 'warning',
      title: t('settings:reEnableAutoUnlock'),
      description:
        status.autoUnlockMessage ||
        t('settings:theSavedAutoUnlockKeyNoLongerMatchesProtectedDataOnThisDevice'),
      actionLabel: t('settings:reEnable')
    }
  }
  if (status.autoUnlockStatus === 'keychain_unavailable') {
    return {
      type: 'warning',
      title: t('settings:autoUnlockUnavailable'),
      description:
        status.autoUnlockMessage || t('settings:unlockYourSystemAndReopenAtermToTryAgain')
    }
  }
  if (status.autoUnlockStatus === 'vault_corrupted') {
    return {
      type: 'error',
      title: t('settings:protectedDataFailedItsIntegrityCheck'),
      description:
        status.autoUnlockMessage || t('settings:atermCouldNotSafelyOpenProtectedDataOnThisDevice')
    }
  }
  return null
}

function lastSyncLabel(status: service.EncryptedSyncStatus | null) {
  return formatDateTime(status?.metadata?.lastUpload || status?.lastUpload)
}

function syncStatusMessage(status: service.EncryptedSyncStatus | null, macAppStoreBuild: boolean) {
  if (!status) return t('settings:checkingSecureDataStatus')
  if (status.message) return status.message
  if (macAppStoreBuild && status.syncDirectoryAuthorizationRequired) {
    return t('settings:backupChooseFolderDescription')
  }
  if (!status.cloudAvailable) {
    return t('settings:savedPasswordsAreProtectedOnThisDeviceFolderBackupIsCurrentlyUnavailable')
  }
  if (status.backupUpdateRequired) {
    return t('settings:backupOutdatedDescription')
  }
  if (status.vaultAvailable) {
    return t('settings:backupAvailableDescription')
  }
  return t('settings:backupNotCreatedDescription')
}

function formatDateTime(value?: string) {
  if (!value) return t('settings:never')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(i18n.language, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}
