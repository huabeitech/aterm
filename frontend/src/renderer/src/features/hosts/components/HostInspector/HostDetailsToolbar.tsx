import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { IconButton, Tip } from '@/components/ui/desktop'
import { ActionButton } from '@/components/ui/workbench-controls'
import { Copy, Ellipsis, FolderOpen, Pencil, Pin, Terminal } from 'lucide-react'
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
      <div className="flex flex-wrap items-center justify-between gap-y-1" role="group" aria-label={t('hosts:hostActions')}>
        <div className="flex items-center">
          <Tip title={t('hosts:openSshTerminal')}>
            <ActionButton
              variant="ghost"
              size="xs"
              className="h-6 has-[>svg]:px-1 text-primary hover:bg-primary/10 hover:text-primary"
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
              variant="ghost"
              size="xs"
              className="h-6 has-[>svg]:px-1 text-muted-foreground"
              icon={<FolderOpen aria-hidden="true" />}
              disabled={disabled}
              onClick={onOpenFiles}
              aria-label={t('hosts:openSftp')}
            >
              SFTP
            </ActionButton>
          </Tip>
        </div>
        <div className="ml-auto flex items-center gap-0.5">
          {action(t('hosts:edit'), <Pencil size={16} />, onEdit)}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <IconButton
                className="size-6 min-w-6 rounded-md p-0 text-muted-foreground"
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
    </header>
  )
}
