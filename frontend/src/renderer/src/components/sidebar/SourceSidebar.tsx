import { platformFeatures } from '../../../../shared/platformFeatures'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter
} from '@/components/ui/dialog'
import { Cable, LoaderCircle } from 'lucide-react'
import { SidebarUtilityIcon } from './SidebarUtilityIcon'
import { ConnectionTree, type ConnectionTreeProps } from './ConnectionTree'
import { DIRECTORY_NAME_MAX_LENGTH, validateDirectoryName } from '../common/directoryValidation'
import type { DirectoryEditorState, ViewID } from '../common/types'
import type { domain } from '../../api/types'
import { cn } from '../../lib/utils'

type SourceSidebarProps = ConnectionTreeProps & {
  platform: string
  activeView: ViewID
  directoryEditor: DirectoryEditorState
  onDirectoryEditorChange: (next: DirectoryEditorState) => void
  onSaveDirectory: () => void
  onCancelDirectoryEditor: () => void
  syncStatus: domain.EncryptedSyncStatus | null
  quickBackupPhase: 'authorizing' | 'uploading' | null
  onOpenTunnels: () => void
  runningTunnels: number
  failedTunnels: number
  onOpenSettings: () => void
  onBackUpToICloud: () => void
  onExportOpenSSHConfig: () => void
}

export function SourceSidebar(props: SourceSidebarProps) {
  useTranslation()
  const {
    activeView,
    directoryEditor,
    busyAction,
    syncStatus,
    quickBackupPhase,
    onDirectoryEditorChange,
    onSaveDirectory,
    onCancelDirectoryEditor,
    onOpenSettings,
    onBackUpToICloud,
    onExportOpenSSHConfig
  } = props
  const creatingDirectory = directoryEditor?.mode === 'create' ? directoryEditor : null
  const renamingDirectory = directoryEditor?.mode === 'rename' ? directoryEditor : null
  const activeDirectoryEditor = directoryEditor
  const activeDirectoryError = directoryEditor ? validateDirectoryName(directoryEditor.name) : ''
  const backupBusy = quickBackupPhase !== null
  const backupDisabled = backupBusy || Boolean(busyAction)
  const backupLabel =
    quickBackupPhase === 'authorizing'
      ? t('hosts:authorizingIcloudBackup')
      : backupBusy
        ? t('hosts:backingUpToIcloud')
        : t('hosts:icloudBackup')
  const backupDescription = !syncStatus
    ? t('hosts:checkingProtectedDataStatus')
    : t('hosts:backUpToIcloud')
  const exportBusy = busyAction === 'export'
  const exportLabel = exportBusy ? t('hosts:exporting') : t('hosts:exportHostsToOpenssh')
  return (
    <>
      <ConnectionTree {...props} />
      <nav className="sidebar-utilities" aria-label={t('hosts:application')}>
        <div className="sidebar-utilities-toolbar">
          <button
            type="button"
            className={cn(
              'sidebar-utility-button sidebar-utility-settings',
              activeView === 'settings' && 'is-selected'
            )}
            aria-label={t('hosts:settings')}
            title={t('hosts:settings')}
            aria-current={activeView === 'settings' ? 'page' : undefined}
            onClick={onOpenSettings}
          >
            <SidebarUtilityIcon name="gear-six" />
            <span>{t('hosts:settings')}</span>
          </button>
          <div className="sidebar-utility-actions">
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="sidebar-utility-button sidebar-utility-icon"
                  onClick={props.onOpenTunnels}
                  aria-label={t('hosts:tunnels.sidebar', {active: props.runningTunnels, failed: props.failedTunnels})}
                >
                  <Cable size={16} aria-hidden="true" />
                </button>
              </TooltipTrigger>
              <TooltipContent side="top" sideOffset={8}>
                {t('hosts:tunnels.all')}
              </TooltipContent>
            </Tooltip>
            {platformFeatures(props.platform).cloudBackup && (
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span
                      className="sidebar-utility-target"
                      tabIndex={backupDisabled ? 0 : undefined}
                      aria-label={backupDisabled ? backupLabel : undefined}
                    >
                      <button
                        type="button"
                        className="sidebar-utility-button sidebar-utility-icon"
                        disabled={backupDisabled}
                        aria-label={backupLabel}
                        aria-busy={backupBusy}
                        onClick={onBackUpToICloud}
                      >
                        {backupBusy ? (
                          <LoaderCircle
                            className="source-sync-spinner animate-spin [animation-duration:800ms]"
                            size={16}
                            aria-hidden="true"
                          />
                        ) : (
                          <SidebarUtilityIcon name="cloud-arrow-up" />
                        )}
                      </button>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent side="top" sideOffset={8}>
                    {backupBusy ? backupLabel : backupDescription}
                  </TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span
                      className="sidebar-utility-target"
                      tabIndex={busyAction ? 0 : undefined}
                      aria-label={busyAction ? exportLabel : undefined}
                    >
                      <button
                        type="button"
                        className="sidebar-utility-button sidebar-utility-icon"
                        disabled={Boolean(busyAction)}
                        aria-label={exportLabel}
                        aria-busy={exportBusy}
                        onClick={onExportOpenSSHConfig}
                      >
                        {exportBusy ? (
                          <LoaderCircle
                            className="source-sync-spinner animate-spin [animation-duration:800ms]"
                            size={16}
                            aria-hidden="true"
                          />
                        ) : (
                          <SidebarUtilityIcon name="export" />
                        )}
                      </button>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent side="top" sideOffset={8}>
                    {exportLabel}
                  </TooltipContent>
                </Tooltip>
              </>
            )}
          </div>
        </div>
        <span className="sr-only" role="status">
          {backupBusy ? backupLabel : exportBusy ? exportLabel : ''}
        </span>
      </nav>

      <Dialog
        open={Boolean(activeDirectoryEditor)}
        onOpenChange={(open) => {
          if (!open && busyAction !== 'directory') onCancelDirectoryEditor()
        }}
      >
        <DialogContent
          className="sm:max-w-[420px]"
          aria-describedby={undefined}
          showCloseButton={busyAction !== 'directory'}
          onPointerDownOutside={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => {
            if (busyAction === 'directory') event.preventDefault()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {renamingDirectory
                ? t('hosts:renameDirectory')
                : creatingDirectory?.parentID
                  ? t('hosts:newSubdirectory')
                  : t('hosts:newDirectory')}
            </DialogTitle>
          </DialogHeader>
          {activeDirectoryEditor && (
            <Input
              autoFocus
              value={activeDirectoryEditor.name}
              onChange={(event) =>
                onDirectoryEditorChange({ ...activeDirectoryEditor, name: event.target.value })
              }
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !validateDirectoryName(activeDirectoryEditor.name))
                  onSaveDirectory()
              }}
              placeholder={t('hosts:directoryName')}
              maxLength={DIRECTORY_NAME_MAX_LENGTH}
              aria-invalid={!!activeDirectoryError}
              aria-label={t('hosts:directoryName')}
              disabled={busyAction === 'directory'}
              spellCheck={false}
              autoCapitalize="off"
              autoCorrect="off"
            />
          )}
          {activeDirectoryError && (
            <p role="alert" className="text-xs text-destructive">
              {activeDirectoryError}
            </p>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={busyAction === 'directory'}
              onClick={onCancelDirectoryEditor}
            >
              {t('hosts:cancel')}
            </Button>
            <Button
              disabled={
                !activeDirectoryEditor || !!activeDirectoryError || busyAction === 'directory'
              }
              onClick={onSaveDirectory}
            >
              {renamingDirectory ? t('hosts:save') : t('hosts:create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
