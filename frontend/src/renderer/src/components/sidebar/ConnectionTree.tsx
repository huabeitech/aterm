import { useConnectionTreeDrag } from './useConnectionTreeDrag'
import './connectionTreeDrag.css'
import { HoverDetailsGroup } from '@/components/ui/hover-details-state'
import { useEffect, useMemo, useRef, useState } from 'react'
import { FolderPlus, Plus, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import { ScrollArea } from '@/components/common/ScrollArea'
import { cn } from '@/lib/utils'
import type { domain } from '../../api/types'
import { RenameConnectionDialog } from './RenameConnectionDialog'
import { TreeMenu, type Action } from './TreeMenu'
import { buildConnectionTree } from './connectionTreeModel'
import { ConnectionTreeToolbar } from './ConnectionTreeToolbar'
import { ConnectionTreeNodeRow } from './ConnectionTreeNodeRow'
import type { ConnectionTreeProps } from './connectionTreeTypes'
export type { ConnectionTreeProps } from './connectionTreeTypes'

const COLLAPSED_DIRECTORIES_KEY = 'aterm.connectionTree.collapsedDirectories'

function readCollapsedDirectories(): Set<string> {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(COLLAPSED_DIRECTORIES_KEY) ?? '[]')
    if (Array.isArray(stored) && stored.every((id) => typeof id === 'string')) {
      return new Set(stored)
    }
  } catch {
    // Unavailable or invalid storage should not prevent the sidebar from opening.
  }
  return new Set()
}

export function ConnectionTree(props: ConnectionTreeProps) {
  useTranslation()
  const [renaming, setRenaming] = useState<domain.Host | null>(null)
  const [focusedNodeID, setFocusedNodeID] = useState<string | null>(null)
  const [keyboardNavigation, setKeyboardNavigation] = useState(false)
  const [collapsed, setCollapsed] = useState(readCollapsedDirectories)
  const [query, setQuery] = useState('')
  const [locateID, setLocateID] = useState<string | null>(null)
  const treeRef = useRef<HTMLDivElement>(null)
  const [revealRequest, setRevealRequest] = useState(0)
  function revealHost(id: string) {
    const host = props.hosts.find((h) => h.id === id)
    if (!host) return
    setQuery('')
    setCollapsed((current) => {
      const next = new Set(current)
      let parent = host.directoryID
      const seen = new Set<string>()
      while (parent && !seen.has(parent)) {
        seen.add(parent)
        next.delete(parent)
        parent =
          props.directoryTree.find((d) => d.directory.id === parent)?.directory.parentID ?? ''
      }
      return next
    })
    setLocateID(id)
    setRevealRequest((n) => n + 1)
  }
  useEffect(() => {
    if (!locateID) return
    const row = Array.from(
      treeRef.current?.querySelectorAll<HTMLElement>('[data-node-id]') ?? []
    ).find((element) => element.dataset.nodeId === locateID)
    if (row) {
      row.scrollIntoView({ block: 'nearest' })
      row.focus({ preventScroll: true })
      setLocateID(null)
    }
  }, [locateID, revealRequest, props.hosts, collapsed])
  const priorDirectories = useRef(new Map(props.hosts.map((h) => [h.id, h.directoryID])))
  useEffect(() => {
    const moved = props.hosts.find(
      (h) =>
        props.selectedHostIDs.includes(h.id) &&
        priorDirectories.current.has(h.id) &&
        priorDirectories.current.get(h.id) !== h.directoryID
    )
    priorDirectories.current = new Map(props.hosts.map((h) => [h.id, h.directoryID]))
    if (moved) revealHost(moved.id)
  }, [props.hosts])
  const previousParents = useRef(
    new Map(props.directoryTree.map(({ directory }) => [directory.id, directory.parentID]))
  )
  useEffect(() => {
    const moved = props.directoryTree.find(
      ({ directory }) =>
        previousParents.current.has(directory.id) &&
        previousParents.current.get(directory.id) !== directory.parentID
    )?.directory
    previousParents.current = new Map(
      props.directoryTree.map(({ directory }) => [directory.id, directory.parentID])
    )
    if (!moved) return
    setQuery('')
    setCollapsed((current) => {
      const next = new Set(current)
      let parent = moved.parentID
      const seen = new Set<string>()
      while (parent && !seen.has(parent)) {
        seen.add(parent)
        next.delete(parent)
        parent =
          props.directoryTree.find((d) => d.directory.id === parent)?.directory.parentID ?? ''
      }
      return next
    })
    setLocateID(moved.id)
  }, [props.directoryTree])
  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSED_DIRECTORIES_KEY, JSON.stringify([...collapsed]))
    } catch {
      // Keep directory interactions working even when persistence is unavailable.
    }
  }, [collapsed])
  const anchor = useRef<string | null>(null)
  const nodes = useMemo(
    () => buildConnectionTree(props.directoryTree, props.hosts, collapsed, query),
    [props.directoryTree, props.hosts, collapsed, query]
  )
  const disabled = !!props.busyAction
  const toggle = (id: string) =>
    setCollapsed((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  const reveal = (id: string) =>
    setCollapsed((current) => {
      const next = new Set(current)
      next.delete(id)
      return next
    })
  const drag = useConnectionTreeDrag(props, reveal, !!query.trim())
  const tabStop = nodes.some((node) => node.id === focusedNodeID) ? focusedNodeID : nodes[0]?.id
  const createActions = (id = ''): Action[] => [
    {
      label: t('hosts:addHost'),
      icon: <Plus size={14} />,
      run: () => {
        reveal(id)
        props.onCreateHost(id)
      }
    },
    {
      label: id ? t('hosts:newSubdirectory') : t('hosts:newDirectory'),
      icon: <FolderPlus size={14} />,
      run: () => {
        reveal(id)
        props.onStartCreateDirectory(id)
      }
    }
  ]
  const rootActions: Action[] = [
    ...createActions(),
    { label: t('hosts:batchAdd'), icon: <Upload size={14} />, run: props.onBatchAddHosts }
  ]
  function select(id: string, additive: boolean, range: boolean) {
    let ids = [id]
    if (range && anchor.current) {
      const visible = nodes.filter((n) => n.kind === 'host').map((n) => n.id)
      const start = visible.indexOf(anchor.current),
        end = visible.indexOf(id)
      if (start >= 0) ids = visible.slice(Math.min(start, end), Math.max(start, end) + 1)
    } else if (additive)
      ids = props.selectedHostIDs.includes(id)
        ? props.selectedHostIDs.filter((h) => h !== id)
        : [...props.selectedHostIDs, id]
    if (!range) anchor.current = id
    props.onSelectHost(id, ids)
    if (!additive && !range) props.onOpenHost(id)
  }
  const selectedHostID = props.selectedHostIDs.includes(props.selectedHostID ?? '')
    ? props.selectedHostID
    : props.selectedHostIDs.at(-1)
  const locateHostID = [selectedHostID, props.currentHostID].find(
    (id) => id && props.hosts.some((host) => host.id === id)
  )
  return (
    <HoverDetailsGroup>
      {renaming && (
        <RenameConnectionDialog
          key={renaming.id}
          initialName={renaming.name}
          onSave={(name) => props.onRenameHost(renaming.id, name)}
          onClose={() => setRenaming(null)}
        />
      )}
      <section
        id="connection-sidebar"
        className="connection-sidebar group/tree relative flex min-h-0 min-w-0 flex-col gap-2 overflow-hidden pt-3 [-webkit-app-region:no-drag]"
        data-keyboard-navigation={keyboardNavigation}
        onPointerDownCapture={() => setKeyboardNavigation(false)}
        onFocusCapture={(event) => {
          if (event.target.matches('[role="treeitem"]:focus-visible')) setKeyboardNavigation(true)
        }}
        onKeyDownCapture={(event) => {
          if (
            event.target instanceof HTMLElement &&
            event.target.matches('[role="treeitem"]') &&
            ['Tab', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(
              event.key
            )
          )
            setKeyboardNavigation(true)
        }}
        aria-label={t('hosts:connections')}
      >
        <ConnectionTreeToolbar
          query={query}
          onQueryChange={setQuery}
          onExpandAll={() => setCollapsed(new Set())}
          onCollapseAll={() =>
            setCollapsed(new Set(props.directoryTree.map((d) => d.directory.id)))
          }
          canExpand={props.directoryTree.length > 0}
          canLocate={!!locateHostID}
          busy={disabled}
          createActions={rootActions}
          onLocate={() => {
            if (!locateHostID) return
            if (!props.selectedHostIDs.includes(locateHostID))
              props.onSelectHost(locateHostID, [locateHostID])
            revealHost(locateHostID)
          }}
        />
        {/* Keep drag feedback out of flow: moving the source row can cancel native dragging. */}
        {drag.active && (
          <div
            role="status"
            className="pointer-events-none absolute bottom-12 left-3 right-3 z-20 rounded-md bg-popover p-2 text-xs text-popover-foreground shadow-md"
          >
            {query.trim()
              ? t('hosts:searchDragHint')
              : drag.target?.position === 'inside'
                ? t('hosts:moveIntoFolder', {
                    name:
                      props.directoryTree.find((d) => d.directory.id === drag.target?.id)?.directory
                        .name ?? ''
                  })
                : t('hosts:dragOrganizationHint')}
          </div>
        )}
        <TreeMenu actions={rootActions} disabled={disabled}>
          <ScrollArea
            ref={treeRef}
            className={cn(
              'min-h-0 min-w-0 flex-1 overflow-hidden [&_[data-slot=scroll-area-viewport]>div]:block!',
              drag.active && 'is-dragging'
            )}
            role="tree"
            aria-multiselectable="true"
            aria-label={t('hosts:connections')}
            aria-busy={props.loading}
            onKeyDown={(event) => {
              if (event.target === event.currentTarget || event.defaultPrevented) return
              const rows = Array.from(
                event.currentTarget.querySelectorAll<HTMLElement>('[role=treeitem]')
              )
              const index = rows.indexOf(document.activeElement as HTMLElement)
              let next = index
              if (event.key === 'ArrowDown') next = Math.min(rows.length - 1, index + 1)
              else if (event.key === 'ArrowUp') next = Math.max(0, index - 1)
              else if (event.key === 'Home') next = 0
              else if (event.key === 'End') next = rows.length - 1
              else if (event.key === 'ArrowLeft') {
                const depth = nodes[index]?.depth ?? 0
                next = -1
                for (let i = index - 1; i >= 0; i--) {
                  if (nodes[i].depth < depth) {
                    next = i
                    break
                  }
                }
              } else if (
                event.key === 'ArrowRight' &&
                nodes[index + 1]?.depth > (nodes[index]?.depth ?? 0)
              )
                next = index + 1
              else return
              event.preventDefault()
              rows[next]?.focus()
            }}
            onDragOver={drag.backgroundOver}
            onDragLeave={drag.leave}
            onDrop={(e) => {
              if (drag.active) {
                e.preventDefault()
                drag.end()
              }
            }}
          >
            <div className="min-w-0 px-3 pb-8">
              {drag.active && (
                <div
                  className={cn(
                    'connection-root-drop absolute bottom-0 left-3 right-3 z-10 rounded-md border border-dashed bg-background px-2 py-2 text-xs text-muted-foreground',
                    drag.target?.position === 'root' && 'bg-accent ring-1 ring-primary'
                  )}
                  onDragOver={(e) => drag.over(e)}
                  onDragLeave={drag.leave}
                  onDrop={(e) => drag.drop(e)}
                >
                  {t('hosts:moveToRoot')}
                </div>
              )}
              {props.loading && !nodes.length ? (
                <div className="space-y-2 p-2" aria-label={t('common:loading')}>
                  <div className="h-6 animate-pulse rounded bg-muted" />
                  <div className="h-6 animate-pulse rounded bg-muted" />
                </div>
              ) : (
                nodes.map((node) => (
                  <ConnectionTreeNodeRow
                    key={node.id}
                    node={node}
                    drag={drag}
                    props={props}
                    tabIndex={node.id === tabStop ? 0 : -1}
                    onFocus={() => setFocusedNodeID(node.id)}
                    toggle={toggle}
                    createActions={createActions}
                    select={select}
                    onRename={setRenaming}
                  />
                ))
              )}
              {!props.loading && !nodes.length && (
                <div className="p-4 text-center text-xs text-muted-foreground">
                  {query.trim() ? t('hosts:noMatchingConnections') : t('hosts:connectionTreeEmpty')}
                </div>
              )}
            </div>
          </ScrollArea>
        </TreeMenu>
      </section>
    </HoverDetailsGroup>
  )
}
