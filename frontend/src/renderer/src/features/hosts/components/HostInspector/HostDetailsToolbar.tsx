import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { IconButton, Tip } from '@/components/ui/desktop'
import { ActionButton } from '@/components/ui/workbench-controls'
import { Copy, FolderOpen, Pin, PinOff, SquarePen, Terminal } from 'lucide-react'
import type { ReactNode } from 'react'

export function HostDetailsToolbar({
  identity,
  favorite,
  disabled,
  pluginActions,
  onConnect,
  onEdit,
  onOpenFiles,
  onCopy,
  onFavorite
}: {
  identity: ReactNode
  favorite: boolean
  disabled: boolean
  pluginActions: ReactNode
  onConnect: () => void
  onEdit: () => void
  onOpenFiles: () => void
  onCopy: () => void
  onFavorite: () => void
}) {
  useTranslation()
  const action = (label: string, icon: ReactNode, onClick: () => void) => (
    <Tip title={label}>
      <IconButton
        className="size-6 min-w-6 rounded-md p-0 text-muted-foreground"
        icon={icon}
        aria-label={label}
        disabled={disabled}
        onClick={onClick}
      />
    </Tip>
  )
  return (
    <header className="grid min-w-0 gap-2 [-webkit-app-region:no-drag]">
      {identity}
      <div className="flex flex-wrap items-center justify-between gap-2" role="group" aria-label={t('hosts:hostActions')}>
        <div className="flex items-center gap-1">
          <Tip title={t('hosts:openSshTerminal')}>
            <ActionButton
              size="xs"
              className="h-6"
              icon={<Terminal aria-hidden="true" />}
              disabled={disabled}
              onClick={onConnect}
              aria-label={t('hosts:openSshTerminal')}
            >
              SSH
            </ActionButton>
          </Tip>
          <Tip title={t('hosts:openSftp')}>
            <ActionButton
              size="xs"
              className="h-6"
              icon={<FolderOpen aria-hidden="true" />}
              disabled={disabled}
              onClick={onOpenFiles}
              aria-label={t('hosts:openSftp')}
            >
              SFTP
            </ActionButton>
          </Tip>
        </div>
        <div className="ml-auto flex flex-wrap justify-end items-center gap-0.5">
          {action(t('hosts:edit'), <SquarePen size={16} />, onEdit)}
          {action(t('hosts:copySshCommand'), <Copy size={16} />, onCopy)}
          {action(
            t(favorite ? 'hosts:removeFromFavorites' : 'hosts:addToFavorites'),
            favorite ? <PinOff size={16} /> : <Pin size={16} />,
            onFavorite
          )}
          {pluginActions}
        </div>
      </div>
    </header>
  )
}
