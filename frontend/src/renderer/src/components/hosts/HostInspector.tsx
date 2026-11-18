import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Plus, Save } from 'lucide-react'
import { domain as service } from '../../api/types'
import { ScrollArea } from '../common/ScrollArea'
import { type BusyAction, type EditorMode, type FormState } from '../common/types'
import { HostDetailsToolbar } from '../../features/hosts/components/HostInspector/HostDetailsToolbar'
import { HostDetails } from '../../features/hosts/components/HostInspector/HostDetails'
import { HostEditorForm } from '../../features/hosts/components/HostInspector/HostEditorForm'

type HostInspectorPanelProps = {
  mode: EditorMode
  host: service.Host | null
  directoryName: string
  directories: { id: string; label: string }[]
  hosts: service.Host[]
  form: FormState
  errors: Partial<Record<keyof FormState, string>>
  busyAction: BusyAction
  connectDisabledReason: string | null
  connectionFeaturesAvailable: boolean
  fileFeaturesAvailable?: boolean
  copySSHCommandAvailable: boolean
  onChange: (next: FormState) => void
  onCancel: () => void
  onSave: () => void
  onStartEdit: () => void
  onConnect: () => void
  pluginActions: import("react").ReactNode
  onOpenFiles: () => void
  onCopySSHCommand: () => void
  onToggleFavorite: () => void
  onStartCreate: () => void
  onGetHostPassword: (id: string) => Promise<string | null>
}

export function HostInspectorPanel({
  mode,
  host,
  directoryName,
  directories,
  hosts,
  form,
  errors,
  busyAction,
  connectDisabledReason,
  connectionFeaturesAvailable,
  fileFeaturesAvailable = false,
  copySSHCommandAvailable,
  onChange,
  onCancel,
  onSave,
  onStartEdit,
  onConnect,
  pluginActions,
  onOpenFiles,
  onCopySSHCommand,
  onToggleFavorite,
  onStartCreate,
  onGetHostPassword
}: HostInspectorPanelProps) {
  useTranslation()
  const isEditing = mode === 'create' || mode === 'edit'
  return (
    <>
      {!isEditing ? (
        <HostDetailsToolbar
          favorite={!!host?.favorite}
          disabled={!host || !!busyAction}
          connecting={busyAction === 'connect'}
          connectDisabledReason={connectDisabledReason}
          canConnect={connectionFeaturesAvailable}
          canOpenFiles={fileFeaturesAvailable}
          canCopy={copySSHCommandAvailable}
          onConnect={onConnect}
          onEdit={onStartEdit}
          onOpenFiles={onOpenFiles}
          onCopy={onCopySSHCommand}
          onFavorite={onToggleFavorite}
        />
      ) : (
        <header
          className={cn(
            'inspector-header [:root[data-theme=dark]_&]:border-[var(--mac-separator)] [:root[data-theme=dark]_&]:bg-[var(--inspector-background)]',
            '[:root[data-theme=dark]_&_h2]:text-[var(--mac-text)] [-webkit-app-region:drag] grid grid-rows-[auto] content-center min-h-17.5 pt-6 pr-3.5',
            'pb-2.5 pl-3.5 bg-[var(--inspector-background)] [border-bottom:1px_solid_rgba(0,_0,_0,_0.08)] [&_h2]:block [&_h2]:overflow-hidden',
            '[&_h2]:text-ellipsis [&_h2]:whitespace-nowrap [&_h2]:m-0 [&_h2]:text-[#1d1d1f] [&_h2]:text-lg [&_h2]:font-[650] [&_h2]:leading-[calc(1.5rem*var(--app-font-scale))]',
            '[.host-inspector_&]:min-h-19 [.host-inspector_&]:py-2.5 [.host-inspector_&]:px-3 [.host-inspector_&]:[-webkit-app-region:no-drag]',
            '[.host-inspector_&_h2]:text-[var(--mac-secondary-text)] [.host-inspector_&_h2]:text-xs [.host-inspector_&_h2]:leading-[calc(1rem*var(--app-font-scale))]',
            '[.host-inspector_&_h2]:font-semibold'
          )}
        >
          <div className="inspector-title-row grid grid-cols-[minmax(0,_1fr)_max-content] items-center gap-3 overflow-hidden min-w-0 [.host-inspector_&]:grid-cols-[minmax(0,_1fr)] [.host-inspector_&]:gap-2">
            <h2>{mode === 'create' ? t('hosts:addHost') : t('hosts:editHost')}</h2>
            <div className="flex gap-2">
              <Button size="sm" onClick={onCancel}>
                {t('hosts:cancel')}
              </Button>
              <Button
                size="sm"

                disabled={busyAction === 'save'}
                onClick={onSave}
              >
                <Save size={14} /> {t('hosts:save')}
              </Button>
            </div>
          </div>
        </header>
      )}
      <ScrollArea className="inspector-scroll min-h-0 overflow-auto overflow-y-auto [.host-inspector_&]:p-3">
        {isEditing ? (
          <HostEditorForm
            key={`${mode}:${host?.id ?? 'new'}`}
            mode={mode}
            form={form}
            directories={directories}
            hosts={hosts}
            errors={errors}
            onChange={onChange}
          />
        ) : host ? (
          <HostDetails
            host={host}
            pluginActions={pluginActions}
            directoryName={directoryName}
            onGetHostPassword={onGetHostPassword}
          />
        ) : (
          <div className="flex flex-col items-center gap-3 p-8 text-muted-foreground">
            <p>{t('hosts:addAHostOrSelectOneFromTheList')}</p>
            <Button onClick={onStartCreate}>
              <Plus size={15} /> {t('hosts:addHost')}
            </Button>
          </div>
        )}
      </ScrollArea>
    </>
  )
}
