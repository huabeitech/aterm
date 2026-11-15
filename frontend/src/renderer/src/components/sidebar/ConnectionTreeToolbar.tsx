import { ChevronsDownUp, ChevronsUpDown, Crosshair, Plus, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TreeMenu, type Action } from './TreeMenu'

const actionButton = {
  variant: 'outline' as const,
  size: 'icon-sm' as const,
  className:
    'border-border bg-background text-muted-foreground shadow-xs hover:bg-[var(--theme-tab-hover)] hover:text-[var(--theme-tab-text)] dark:hover:bg-[var(--theme-tab-hover)] active:bg-[var(--theme-tab-active)] data-[state=open]:bg-[var(--theme-tab-active)] data-[state=open]:text-[var(--theme-tab-text)] data-[state=open]:hover:bg-[var(--theme-tab-active)] dark:data-[state=open]:bg-[var(--theme-tab-active)] disabled:opacity-40'
}

type Props = {
  query: string
  onQueryChange: (query: string) => void
  onExpandAll: () => void
  onCollapseAll: () => void
  onLocate: () => void
  canExpand: boolean
  canLocate: boolean
  busy: boolean
  createActions: Action[]
}

export function ConnectionTreeToolbar({
  query,
  onQueryChange,
  onExpandAll,
  onCollapseAll,
  onLocate,
  canExpand,
  canLocate,
  busy,
  createActions
}: Props) {
  useTranslation()
  return (
    <div className="connection-sidebar-toolbar mx-3 flex min-w-0 shrink-0 items-center gap-1">
      <div className="relative min-w-0 flex-1">
        <Search className="pointer-events-none absolute left-2 top-2 size-4 text-muted-foreground" />
        <Input
          data-action="search-hosts"
          className="h-8 pl-8 text-sm"
          aria-label={t('hosts:searchHosts')}
          placeholder={t('hosts:searchHosts')}
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
        />
      </div>
      <TreeMenu
        actions={[
          ...createActions,
          {
            label: t('hosts:expandAll'),
            icon: <ChevronsUpDown size={16} />,
            run: onExpandAll,
            disabled: !canExpand
          },
          {
            label: t('hosts:collapseAll'),
            icon: <ChevronsDownUp size={16} />,
            run: onCollapseAll,
            disabled: !canExpand
          },
          {
            label: t('hosts:locateHost'),
            icon: <Crosshair size={16} />,
            run: onLocate,
            afterClose: true,
            disabled: !canLocate
          }
        ]}
        disabled={busy}
        dropdown
      >
        <Button
          {...actionButton}
          aria-label={t('hosts:moreActions')}
          title={t('hosts:moreActions')}
          disabled={busy}
        >
          <Plus size={16} aria-hidden="true" />
        </Button>
      </TreeMenu>
    </div>
  )
}
