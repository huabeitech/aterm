import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { IconButton, Tip } from '@/components/ui/desktop'
import { ActionButton } from '@/components/ui/workbench-controls'
import { Copy, Ellipsis, FolderOpen, Pencil, Pin, Terminal } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { ReactNode } from 'react'

export function HostDetailsToolbar({
  favorite,
  disabled,
  pluginActions,
  onConnect,
  onEdit,
  onOpenFiles,
  onCopy,
  onFavorite
}: {
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
        className="size-7 min-w-7 rounded-md p-0 text-muted-foreground"
        icon={icon}
        aria-label={label}
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
            onClick={onConnect}
          >
            {t('hosts:openSshTerminal')}
          </ActionButton>
        </Tip>
        <Tip title={t('hosts:openSftp')}>
          <ActionButton
            icon={<FolderOpen size={15} aria-hidden="true" />}
            disabled={disabled}
            onClick={onOpenFiles}
          >
            {t('hosts:openSftp')}
          </ActionButton>
        </Tip>
        {pluginActions}
      </div>
      <div className="ml-auto flex min-w-0 flex-wrap items-center gap-1">
        {action(t('hosts:edit'), <Pencil size={16} />, onEdit)}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <IconButton
              icon={<Ellipsis size={16} />}
              aria-label={t('hosts:moreActions')}
              title={t('hosts:moreActions')}
              disabled={disabled}
            />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={onCopy}>
              <Copy size={14} /> {t('hosts:copySshCommand')}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={onFavorite}>
              <Pin size={14} />
              {t(favorite ? 'hosts:removeFromFavorites' : 'hosts:addToFavorites')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  )
}
