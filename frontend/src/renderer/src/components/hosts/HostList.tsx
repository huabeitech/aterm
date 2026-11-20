import { useTranslation } from 'react-i18next'
import { t, i18n } from '../../../../shared/i18n'
import { cn } from '@/lib/utils'
import { TableHeader, TableBody, TableRow, TableCell } from '@/components/ui/table'
import { type MouseEvent as ReactMouseEvent, memo, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { ActionItems, Tip, type ActionItem } from '@/components/ui/desktop'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { ClipboardList, Plus, Settings2, Server, Loader2, PanelRight } from 'lucide-react'
import { domain as service } from '../../api/types'
import { ScrollArea } from '../common/ScrollArea'
import { type BusyAction, type HostSortScope } from '../common/types'
import { HostListAddAction, HostListSearch } from '../../features/hosts/components/HostListToolbar'
import { ResizableHeaderCell } from '../../shared/table/ResizableHeaderCell'
import { useHostContextMenu } from '../../features/hosts/hooks/useHostContextMenu'
import { useHostReordering } from '../../features/hosts/hooks/useHostReordering'
import { useHostColumns } from '../../features/hosts/hooks/useHostColumns'

type HostListProps = {
  hosts: service.Host[]
  directories: { id: string; label: string }[]
  reorderScope: HostSortScope | null
  totalHosts: number
  selectedID: string | null
  selectedIDs: string[]
  loading: boolean
  busyAction: BusyAction
  fileFeaturesAvailable?: boolean
  connectionFeaturesAvailable: boolean
  editorMode: 'view' | 'create' | 'edit'
  searchPlaceholder?: string
  hostListQuery: string
  detailsCollapsed: boolean
  onHostListQueryChange: (query: string) => void
  onToggleDetails: () => void
  onSelectHost: (id: string, options?: { additive?: boolean; range?: boolean }) => void
  onOpenHost: (id: string) => void
  onEditHost: (id: string) => void
  onConnectHost: (id: string) => void
  onOpenFiles: (id: string) => void
  onCopySSHCommand: (id: string) => Promise<boolean>
  onDeleteHost: (id: string) => void
  onDeleteHosts: (ids: string[]) => void
  onAddHost: () => void
  onBatchAddHosts: () => void
  onHostDragStart: (ids: string[]) => void
  onHostDragEnd: () => void
  onReorderHosts: (scope: HostSortScope, ids: string[]) => void
}

type HostColumnState = Record<string, { show?: boolean }>

const HOST_COLUMNS_STATE_KEY = 'aterm-host-list-columns-v4'
const HOST_COLUMNS_DEFAULT_STATE: HostColumnState = {
  directory: { show: true },
  auth: { show: false },
  identityFile: { show: false },
  jumpHost: { show: false },
  forwardAgent: { show: false },
  compression: { show: false },
  serverAliveInterval: { show: false },
  serverAliveCountMax: { show: false },
  connectTimeout: { show: false },
  lastUsedAt: { show: false },
  createdAt: { show: false },
  updatedAt: { show: false }
}

export const HostList = memo(function HostList({
  hosts,
  directories,
  reorderScope,
  totalHosts,
  selectedID,
  selectedIDs,
  loading,
  busyAction,
  fileFeaturesAvailable = false,
  connectionFeaturesAvailable,
  editorMode,
  hostListQuery,
  searchPlaceholder,
  detailsCollapsed,
  onHostListQueryChange,
  onToggleDetails,
  onSelectHost,
  onOpenHost,
  onEditHost,
  onConnectHost,
  onOpenFiles,
  onCopySSHCommand,
  onDeleteHost,
  onDeleteHosts,
  onAddHost,
  onBatchAddHosts,
  onHostDragStart,
  onHostDragEnd,
  onReorderHosts
}: HostListProps) {
  useTranslation()
  const scrollContainerRef = useRef<HTMLDivElement | null>(null)
  const {
    orderedHosts,
    draggingHostIDs,
    hostDropTarget,
    canReorder,
    startHostDrag,
    finishHostDrag,
    handleHostDragOver,
    dropHostsToPosition
  } = useHostReordering({
    hosts,
    selectedIDs,
    reorderScope,
    onHostDragStart,
    onHostDragEnd,
    onReorderHosts
  })

  const { contextMenu, setContextMenu, contextMenuItems, handleContextMenuClick } =
    useHostContextMenu({
      fileFeaturesAvailable,
      connectionFeaturesAvailable,
      onOpenHost,
      onEditHost,
      onConnectHost,
      onOpenFiles,
      onCopySSHCommand,
      onDeleteHost,
      onDeleteHosts
    })

  const { columns } = useHostColumns({
    directories,
    busyAction,
    onCopySSHCommand
  })
  const [columnsState, setColumnsState] = useState<HostColumnState>(readInitialColumnsState)
  const tableScrollX = useMemo(
    () =>
      columns.reduce((total, column) => {
        const columnKey = String(column.key ?? '')
        if (columnKey && columnsState[columnKey]?.show === false) return total
        return total + (typeof column.width === 'number' ? column.width : 0)
      }, 0),
    [columns, columnsState]
  )

  const addMenuItems = useMemo<ActionItem[]>(
    () => [
      {
        key: 'batch-add',
        icon: <ClipboardList size={14} />,
        label: t('hosts:batchAdd')
      }
    ],
    [i18n.language]
  )
  const addDisabled = Boolean(busyAction)

  function handleAddMenuClick({ key }: { key: string }) {
    if (key === 'batch-add') onBatchAddHosts()
  }

  function selectHostFromMouse(event: ReactMouseEvent<HTMLTableRowElement>, host: service.Host) {
    onSelectHost(host.id, {
      additive: event.metaKey || event.ctrlKey,
      range: event.shiftKey
    })
  }

  const visibleColumns = columns.filter((column) => columnsState[column.key]?.show !== false)
  function toggleColumn(key: string, show: boolean) {
    const next = { ...columnsState, [key]: { show } }
    setColumnsState(next)
    window.localStorage.setItem(HOST_COLUMNS_STATE_KEY, JSON.stringify(next))
  }
  return (
    <section
      className="host-list w-full max-w-full min-w-0 overflow-hidden bg-[var(--hosts-background)] flex min-h-0 flex-col"
      aria-label={t('hosts:hosts')}
      aria-busy={loading}
    >
      <div className="host-list-toolbar flex min-h-[calc(53px*var(--app-layout-scale))] flex-wrap shrink-0 items-center gap-2 border-b border-border bg-background px-3 py-2">
        <HostListSearch
          searchPlaceholder={searchPlaceholder}
          hostListQuery={hostListQuery}
          onHostListQueryChange={onHostListQueryChange}
        />
        <div className="ml-auto flex items-center gap-1.5">
          {loading && (
            <Loader2
              className="size-3.5 animate-spin text-muted-foreground"
              aria-label={t('hosts:loadingHosts')}
            />
          )}
          <HostListAddAction
            addDisabled={addDisabled}
            addMenuItems={addMenuItems}
            onAddHost={onAddHost}
            onAddMenuClick={handleAddMenuClick}
          />
          <Tip title={t('hosts:tableSettings')}>
            <div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-[30px]"
                    aria-label={t('hosts:tableSettings')}
                  >
                    <Settings2 size={16} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="max-h-[70vh] overflow-y-auto">
                  <DropdownMenuLabel>{t('hosts:tableSettings')}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {columns
                    .filter((column) => !column.hideInSetting)
                    .map((column) => (
                      <DropdownMenuCheckboxItem
                        key={column.key}
                        checked={columnsState[column.key]?.show !== false}
                        onSelect={(event) => event.preventDefault()}
                        onCheckedChange={(show) => toggleColumn(column.key, show)}
                      >
                        {column.title}
                      </DropdownMenuCheckboxItem>
                    ))}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </Tip>
          <Tip title={detailsCollapsed ? t('hosts:showHostDetails') : t('hosts:hideHostDetails')}>
            <Button
              variant="ghost"
              size="icon"
              className="size-[30px] aria-pressed:bg-accent aria-pressed:text-accent-foreground"
              aria-label={
                detailsCollapsed ? t('hosts:showHostDetails') : t('hosts:hideHostDetails')
              }
              aria-pressed={!detailsCollapsed}
              onClick={onToggleDetails}
            >
              <PanelRight size={16} />
            </Button>
          </Tip>
        </div>
      </div>
      <ScrollArea
        ref={scrollContainerRef}
        className="list-scroll [container-type:inline-size] min-h-0 h-full overflow-auto bg-[var(--hosts-background)] [.host-list_>_&]:h-auto [.host-list_>_&]:flex-1 flex-1"
      >
        <table
          className={cn(
            'host-native-table text-[var(--mac-text)] [&_tbody_tr:hover]:bg-[var(--mac-hover-background)]',
            '[&_tbody_tr.is-selected]:bg-[var(--mac-selection-background)] [&_tr.is-host-dragging]:opacity-45',
            '[&_tr.is-host-drop-before_>_td]:shadow-[inset_0_2px_0_#1677ff] [&_tr.is-host-drop-after_>_td]:shadow-[inset_0_-2px_0_#1677ff]',
            '[&_tr.is-primary-selected_>_td:first-child]:shadow-[inset_3px_0_0_#1677ff] w-full table-fixed border-collapse text-[length:var(--app-text-13)]'
          )}
          style={{ minWidth: `calc(${tableScrollX}px * var(--app-layout-scale))` }}
        >
          <colgroup>
            {visibleColumns.map((column) => (
              <col
                key={column.key}
                style={{ width: `calc(${column.width}px * var(--app-layout-scale))` }}
              />
            ))}
          </colgroup>
          <TableHeader className="sticky top-0 z-10 bg-background">
            <TableRow>
              {visibleColumns.map((column) => (
                <ResizableHeaderCell
                  key={column.key}
                  {...column.onHeaderCell?.()}
                  className="relative border-b border-border px-3 py-2 text-left text-xs font-medium text-muted-foreground"
                  style={{ textAlign: column.align }}
                >
                  {column.title}
                </ResizableHeaderCell>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {orderedHosts.map((host) => (
              <TableRow
                key={host.id}
                data-row-key={host.id}
                aria-selected={selectedIDs.includes(host.id) && editorMode !== 'create'}
                className={[
                  selectedIDs.includes(host.id) && editorMode !== 'create' ? 'is-selected' : '',
                  host.id === selectedID && editorMode !== 'create' ? 'is-primary-selected' : '',
                  draggingHostIDs.includes(host.id) ? 'is-host-dragging' : '',
                  hostDropTarget?.id === host.id ? `is-host-drop-${hostDropTarget.placement}` : ''
                ]
                  .filter(Boolean)
                  .join(' ')}
                draggable={canReorder}
                onClick={(event) => selectHostFromMouse(event, host)}
                onDoubleClick={(event) => {
                  const target = event.target instanceof Element ? event.target : null
                  if (
                    !connectionFeaturesAvailable ||
                    busyAction ||
                    target?.closest(
                      'button, a, input, textarea, select, [role="button"], .table-column-resizer'
                    )
                  )
                    return
                  onConnectHost(host.id)
                }}
                onDragStart={(event) => startHostDrag(event, host)}
                onDragOver={(event) => handleHostDragOver(event, host)}
                onDrop={(event) => dropHostsToPosition(event, host)}
                onDragEnd={finishHostDrag}
                onContextMenu={(event) => {
                  event.preventDefault()
                  const ids = selectedIDs.includes(host.id) ? selectedIDs : [host.id]
                  if (!selectedIDs.includes(host.id)) onSelectHost(host.id)
                  setContextMenu({ host, ids, x: event.clientX, y: event.clientY })
                }}
              >
                {visibleColumns.map((column) => (
                  <TableCell
                    key={column.key}
                    className="overflow-hidden border-b border-border/50 px-3 py-2 text-ellipsis whitespace-nowrap"
                    style={{ textAlign: column.align }}
                  >
                    {column.render
                      ? column.render(undefined, host)
                      : String(column.dataIndex ? (host[column.dataIndex] ?? '') : '')}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </table>
        {!orderedHosts.length && (
          <div className="sticky left-0 flex w-[100cqw] max-w-full flex-col items-center gap-3 py-16 text-muted-foreground">
            <Server size={28} strokeWidth={1.2} />
            <p>{totalHosts === 0 ? t('hosts:noHostsYet') : t('hosts:noHostsMatchThisView')}</p>
            {totalHosts === 0 && (
              <>
                <p>{t('hosts:addAHostToStartAnSshSession')}</p>
                <Button onClick={onAddHost}>
                  <Plus size={15} />
                  {t('hosts:addHost')}
                </Button>
              </>
            )}
          </div>
        )}
      </ScrollArea>
      {contextMenu && (
        <DropdownMenu
          open
          onOpenChange={(open) => {
            if (!open) setContextMenu(null)
          }}
        >
          <DropdownMenuTrigger asChild>
            <span
              className="host-context-menu-anchor fixed z-1000 w-px h-px pointer-events-none"
              style={{ position: 'fixed', left: contextMenu.x, top: contextMenu.y }}
            />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" onCloseAutoFocus={(event) => event.preventDefault()}>
            <ActionItems items={contextMenuItems} onAction={handleContextMenuClick} />
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </section>
  )
}, hostListPropsEqual)

function hostListPropsEqual(previous: HostListProps, next: HostListProps) {
  return (
    previous.hosts === next.hosts &&
    previous.directories === next.directories &&
    previous.reorderScope?.type === next.reorderScope?.type &&
    previous.reorderScope?.id === next.reorderScope?.id &&
    previous.totalHosts === next.totalHosts &&
    previous.selectedID === next.selectedID &&
    sameIDs(previous.selectedIDs, next.selectedIDs) &&
    previous.loading === next.loading &&
    previous.busyAction === next.busyAction &&
    previous.fileFeaturesAvailable === next.fileFeaturesAvailable &&
    previous.connectionFeaturesAvailable === next.connectionFeaturesAvailable &&
    previous.editorMode === next.editorMode &&
    previous.hostListQuery === next.hostListQuery
  )
}

function sameIDs(first: string[], second: string[]) {
  return (
    first === second ||
    (first.length === second.length && first.every((id, index) => id === second[index]))
  )
}

function readInitialColumnsState() {
  if (typeof window === 'undefined') return HOST_COLUMNS_DEFAULT_STATE
  try {
    const persisted = window.localStorage.getItem(HOST_COLUMNS_STATE_KEY)
    if (!persisted) return HOST_COLUMNS_DEFAULT_STATE
    const saved = JSON.parse(persisted) as HostColumnState
    // Preserve visibility when merging the former Proxy and Jump Host columns.
    if (saved.proxyJump?.show) saved.jumpHost = { ...saved.jumpHost, show: true }
    delete saved.proxyJump
    return { ...HOST_COLUMNS_DEFAULT_STATE, ...saved }
  } catch {
    return HOST_COLUMNS_DEFAULT_STATE
  }
}
