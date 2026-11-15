import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { IconButton, Tip } from '@/components/ui/desktop'
import { Copy, FolderOpen, Pencil, Pin, Terminal } from 'lucide-react'
import type { ReactNode } from 'react'

export function HostDetailsToolbar({
  favorite,
  disabled,
  connecting,
  connectDisabledReason,
  canConnect,
  canOpenFiles,
  canCopy,
  onConnect,
  onEdit,
  onOpenFiles,
  onCopy,
  onFavorite
}: {
  favorite: boolean
  disabled: boolean
  connecting: boolean
  connectDisabledReason: string | null
  canConnect: boolean
  canOpenFiles: boolean
  canCopy: boolean
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
    <header className="flex min-h-[calc(53px*var(--app-layout-scale))] shrink-0 min-w-0 flex-col justify-center gap-1.5 border-b border-border bg-background px-3 py-2 [-webkit-app-region:no-drag]">
      <div
        className="flex min-h-7.5 min-w-0 flex-wrap items-center justify-between gap-1"
        role="toolbar"
        aria-label={t('hosts:hostActions')}
      >
        {canConnect && (
          <Tip title={connectDisabledReason ?? t('hosts:openSshTerminal')}>
            <IconButton
              className="h-7.5 w-auto shrink-0 px-2 text-xs font-medium text-foreground"
              icon={<Terminal size={16} />}
              disabled={disabled || !!connectDisabledReason}
              loading={connecting}
              onClick={onConnect}
            >
              {t('hosts:connect')}
            </IconButton>
          </Tip>
        )}
        <div className="flex min-w-0 items-center gap-1">
          {canOpenFiles && action('SFTP', <FolderOpen size={16} />, onOpenFiles)}
          {action(t('hosts:edit'), <Pencil size={16} />, onEdit)}
          {canCopy && action(t('hosts:copySshCommand'), <Copy size={16} />, onCopy)}
          {action(
            favorite ? t('hosts:removeFromFavorites') : t('hosts:addToFavorites'),
            <Pin size={16} fill={favorite ? 'currentColor' : 'none'} />,
            onFavorite,
            favorite
          )}
        </div>
      </div>
    </header>
  )
}
