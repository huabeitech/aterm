import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { IconButton, Tip } from '@/components/ui/desktop'
import { ActionButton } from '@/components/ui/workbench-controls'
import { Copy, Ellipsis, Pencil, Pin } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
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
      className="grid min-w-0 gap-2 [-webkit-app-region:no-drag]"
    >
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-2">
        {identity}
        <div className="flex items-center gap-1" role="group" aria-label={t('hosts:hostActions')}>
          {action(t('hosts:edit'), <Pencil size={16} />, onEdit)}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <IconButton
                className="size-7 min-w-7 rounded-md p-0 text-muted-foreground"
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
              {pluginActions}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <ActionButton variant="default" disabled={disabled} onClick={onConnect} aria-label={t('hosts:openSshTerminal')}>
          SSH
        </ActionButton>
        <ActionButton disabled={disabled} onClick={onOpenFiles} aria-label={t('hosts:openSftp')}>
          SFTP
        </ActionButton>
      </div>
    </header>
  )
}
