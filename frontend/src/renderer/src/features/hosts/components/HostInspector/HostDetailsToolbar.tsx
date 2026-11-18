import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { IconButton, Tip } from '@/components/ui/desktop'
import { ActionButton } from '@/components/ui/workbench-controls'
import { Copy, FolderOpen, Pencil, Pin, Terminal } from 'lucide-react'
import type { ReactNode } from 'react'

export function HostDetailsToolbar({
  favorite,
  disabled,
  connecting,
  pluginActions,
  onConnect,
  onEdit,
  onOpenFiles,
  onCopy,
  onFavorite
}: {
  favorite: boolean
  disabled: boolean
  connecting: boolean
  pluginActions: ReactNode
  onConnect: () => void
  onEdit: () => void
  onOpenFiles: () => void
  onCopy: () => void
  onFavorite: () => void
}) {
  useTranslation()
  const action = (label: string, icon: ReactNode, onClick: () => void, pressed?: boolean) => (
    <Tip title={label}>
      <IconButton
        className="size-7 min-w-7 rounded-md p-0 text-muted-foreground aria-pressed:text-amber-600 dark:aria-pressed:text-amber-500"
        icon={icon}
        aria-label={label}
        aria-pressed={pressed}
        disabled={disabled}
        onClick={onClick}
      />
    </Tip>
  )
  return (
    <header
      className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 py-0 [-webkit-app-region:no-drag]"
      role="toolbar"
      aria-label={t('hosts:hostActions')}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Tip title={t('hosts:openSshTerminal')}>
          <ActionButton
            variant="default"
            icon={<Terminal size={15} aria-hidden="true" />}
            disabled={disabled}
            busy={connecting}
            onClick={onConnect}
          >
            {t('hosts:connect')}
          </ActionButton>
        </Tip>
        <Tip title={t('hosts:openSftp')}>
          <ActionButton
            icon={<FolderOpen size={15} aria-hidden="true" />}
            disabled={disabled}
            onClick={onOpenFiles}
          >
            SFTP
          </ActionButton>
        </Tip>
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        {pluginActions}
        {action(t('hosts:edit'), <Pencil size={16} />, onEdit)}
        {action(t('hosts:copySshCommand'), <Copy size={16} />, onCopy)}
        {action(
          favorite ? t('hosts:removeFromFavorites') : t('hosts:addToFavorites'),
          <Pin size={16} fill={favorite ? 'currentColor' : 'none'} />,
          onFavorite,
          favorite
        )}
      </div>
    </header>
  )
}
