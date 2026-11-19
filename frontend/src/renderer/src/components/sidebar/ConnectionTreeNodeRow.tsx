import {
  ChevronDown,
  ChevronRight,
  Copy,
  Eye,
  Folder,
  FolderOpen,
  Pencil,
  Server,
  Pin,
  PinOff,
  Trash2,
  Zap
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import { cn } from '@/lib/utils'
import type { domain } from '../../api/types'
import type { ConnectionTreeDrag } from './useConnectionTreeDrag'
import { TreeMenu, type Action } from './TreeMenu'
import type { ConnectionTreeNode } from './connectionTreeModel'
import type { ConnectionTreeProps } from './connectionTreeTypes'

const rowIndent = (depth: number) => 8 + depth * 16

const rowClass =
  'connection-tree-row flex min-h-8 w-full select-none items-center gap-2 rounded-md pr-2 text-sm outline-none hover:bg-[var(--theme-tab-hover)] group-data-[keyboard-navigation=true]/tree:focus-visible:ring-2 group-data-[keyboard-navigation=true]/tree:focus-visible:ring-ring aria-selected:bg-[var(--theme-tab-active)] aria-selected:text-[var(--theme-tab-text)]'

export function ConnectionTreeNodeRow({
  node,
  props,
  tabIndex,
  onFocus,
  toggle,
  createActions,
  select,
  onRename,
  drag
}: {
  node: ConnectionTreeNode
  drag: ConnectionTreeDrag
  props: ConnectionTreeProps
  tabIndex: number
  onFocus: () => void
  toggle: (id: string) => void
  createActions: (id?: string) => Action[]
  select: (id: string, additive: boolean, range: boolean) => void
  onRename: (host: domain.Host) => void
}) {
  useTranslation()
  const disabled = !!props.busyAction
  const dropPosition = drag.target?.id === node.id ? drag.target.position : undefined
  const dragProps = {
    'data-drop-position': dropPosition,
    'data-node-id': node.id,
    'data-dragging':
      (node.kind === 'host'
        ? props.draggingHostIDs.includes(node.id)
        : props.draggingDirectoryID === node.id) || undefined,
    onDragStart: (e: import('react').DragEvent<HTMLElement>) => drag.start(e, node),
    onDragOver: (e: import('react').DragEvent<HTMLElement>) => drag.over(e, node),
    onDragLeave: drag.leave,
    onDrop: (e: import('react').DragEvent<HTMLElement>) => drag.drop(e, node),
    onDragEnd: drag.end
  }

  if (node.kind === 'directory') {
    const directory = node.directory
    return (
      <TreeMenu
        key={node.id}
        disabled={disabled}
        actions={[
          ...createActions(node.id),
          {
            label: t('hosts:rename'),
            icon: <Pencil size={14} />,
            run: () => props.onStartRenameDirectory(directory)
          },
          {
            label: t('hosts:delete'),
            icon: <Trash2 size={14} />,
            danger: true,
            run: () => props.onDeleteDirectory(directory)
          }
        ]}
      >
        <div
          role="treeitem"
          aria-level={node.depth + 1}
          aria-expanded={node.expanded}
          aria-selected={props.activeDirectoryID === node.id && !props.selectedHostIDs.length}
          tabIndex={tabIndex}
          onFocus={onFocus}
          draggable={!disabled}
          className={cn(
            rowClass,
            'directory-item',
            dropPosition === 'inside' && 'bg-accent ring-1 ring-inset ring-primary'
          )}
          style={
            {
              paddingLeft: rowIndent(node.depth),
              '--drop-indent': `${rowIndent(node.depth)}px`
            } as import('react').CSSProperties
          }
          onClick={() => {
            if (drag.suppressClick()) return
            props.onSelectDirectory(node.id)
            toggle(node.id)
          }}
          onContextMenu={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              props.onSelectDirectory(node.id)
              toggle(node.id)
            }
            if (e.key === 'ArrowRight' && !node.expanded) {
              e.preventDefault()
              toggle(node.id)
            }
            if (e.key === 'ArrowLeft' && node.expanded) {
              e.preventDefault()
              toggle(node.id)
            }
            if (e.key === 'F2' && !disabled) {
              e.preventDefault()
              props.onStartRenameDirectory(directory)
            }
          }}
          {...dragProps}
        >
          <span className="flex w-3 shrink-0 items-center justify-center" aria-hidden="true">
            {node.expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          </span>
          {node.expanded ? (
            <FolderOpen className="connection-node-icon shrink-0 text-muted-foreground" size={16} />
          ) : (
            <Folder className="connection-node-icon shrink-0 text-muted-foreground" size={16} />
          )}
          <span className="truncate">{directory.name}</span>
        </div>
      </TreeMenu>
    )
  }
  const host = node.host
  const selected = props.selectedHostIDs.includes(host.id)
  const ids = selected ? props.selectedHostIDs : [host.id]
  return (
    <TreeMenu
      key={host.id}
      disabled={disabled}
      actions={[
        {
          label: t('hosts:connect'),
          icon: <Zap size={14} />,
          run: () => props.onConnectHost(host.id)
        },
        {
          label: t('hosts:details'),
          icon: <Eye size={14} />,
          run: () => props.onOpenHost(host.id)
        },
        {
          label: t('hosts:edit'),
          icon: <Pencil size={14} />,
          run: () => props.onEditHost(host.id)
        },
        {
          label: t('hosts:rename'),
          icon: <Pencil size={14} />,
          run: () => onRename(host)
        },
        {
          label: host.favorite ? t('hosts:removeFromFavorites') : t('hosts:addToFavorites'),
          icon: host.favorite ? <PinOff size={14} /> : <Pin size={14} />,
          run: () => props.onToggleFavoriteHost(host.id)
        },
        {
          label: t('hosts:copySshCommand'),
          icon: <Copy size={14} />,
          run: () => props.onCopySSHCommand(host.id)
        },
        {
          label: ids.length > 1 ? t('hosts:deleteHosts', { count: ids.length }) : t('hosts:delete'),
          icon: <Trash2 size={14} />,
          danger: true,
          run: () => props.onDeleteHosts(ids)
        }
      ]}
    >
      <div
        role="treeitem"
        aria-level={node.depth + 1}
        aria-label={host.name}
        aria-selected={selected}
        tabIndex={tabIndex}
        onFocus={onFocus}
        draggable={!disabled}
        className={cn(rowClass, 'group/host host-tree-item')}
        style={
          {
            paddingLeft: rowIndent(node.depth),
            '--drop-indent': `${rowIndent(node.depth)}px`
          } as import('react').CSSProperties
        }
        onClick={(e) => {
          if (!drag.suppressClick()) select(host.id, e.metaKey || e.ctrlKey, e.shiftKey)
        }}
        onDoubleClick={() => {
          if (!disabled && !drag.suppressClick()) props.onConnectHost(host.id)
        }}
        onContextMenu={(e) => {
          e.stopPropagation()
          if (!selected) props.onSelectHost(host.id, [host.id])
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !disabled) {
            e.preventDefault()
            props.onConnectHost(host.id)
          }
          if (e.key === ' ') {
            e.preventDefault()
            select(host.id, e.metaKey || e.ctrlKey, e.shiftKey)
          }
          if (e.key === 'F2' && !disabled) {
            e.preventDefault()
            onRename(host)
          }
          if (e.key === 'Delete' && !disabled) {
            e.preventDefault()
            props.onDeleteHosts(ids)
          }
        }}
        {...dragProps}
      >
        <span className="w-3 shrink-0" aria-hidden="true" />
        <Server size={16} className="connection-node-icon shrink-0 text-muted-foreground" />
        <span className="truncate">{host.name}</span>
        <button
          type="button"
          aria-label={t(host.favorite ? 'hosts:removeFromFavorites' : 'hosts:addToFavorites')}
          title={t(host.favorite ? 'hosts:removeFromFavorites' : 'hosts:addToFavorites')}
          aria-pressed={host.favorite}
          disabled={disabled}
          draggable={false}
          className={cn(
            'ml-auto flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
            !host.favorite &&
              'opacity-0 group-hover/host:opacity-100 group-focus-within/host:opacity-100'
          )}
          onClick={(e) => {
            e.stopPropagation()
            props.onToggleFavoriteHost(host.id)
          }}
          onDoubleClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => e.stopPropagation()}
        >
          {host.favorite ? (
            <PinOff size={12} aria-hidden="true" />
          ) : (
            <Pin size={12} aria-hidden="true" />
          )}
        </button>
      </div>
    </TreeMenu>
  )
}
