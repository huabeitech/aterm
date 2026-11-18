import { hasRemoteConnection } from '../../../../shared/contracts/terminal'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  ActionButton,
  ActionMenu,
  HoverDetails,
  type MenuAction
} from '@/components/ui/workbench-controls'
import { useState, type DragEvent, type ReactNode } from 'react'

import {
  ArrowRightToLine,
  Copy,
  FolderOpen,
  PanelLeftClose,
  PanelsTopLeft,
  Pencil,
  RotateCcw,
  Server,
  SquareSplitHorizontal,
  Terminal,
  X
} from 'lucide-react'
import { domain } from '../../api/types'
import type { TerminalSession } from '../../components/terminal/types'
import type { WorkspaceTabDropPosition, WorkspaceTabID } from './workspaceModel'

type DragProps = {
  dragging: boolean
  dropPosition: WorkspaceTabDropPosition | null
  onDragStart: (event: DragEvent<HTMLElement>, tab: WorkspaceTabID) => void
  onDragOver: (event: DragEvent<HTMLElement>, tab: WorkspaceTabID) => void
  onDrop: (event: DragEvent<HTMLElement>, tab: WorkspaceTabID) => void
  onDragEnd: () => void
}

type WorkspaceTabProps = DragProps & {
  tab: WorkspaceTabID
  label: string
  icon: ReactNode
  number?: number
  tooltip?: ReactNode
  actions?: MenuAction[]
  onRename?: () => void
  onResetName?: () => void
  active: boolean
  onActivate: () => void
  onClose: () => void
  onCloseAll: () => void
  onCloseOther: (tab: WorkspaceTabID) => void
  onCloseToRight: (tab: WorkspaceTabID) => void
}

export function WorkspaceTab({
  tab,
  label,
  icon,
  number,
  tooltip,
  actions = [],
  onRename,
  onResetName,
  active,
  dragging,
  dropPosition,
  onActivate,
  onClose,
  onCloseAll,
  onCloseOther,
  onCloseToRight,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd
}: WorkspaceTabProps) {
  useTranslation()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuItems = [
    ...actions,
    ...(onRename
      ? [
          {
            key: 'rename',
            icon: <Pencil size={14} />,
            label: t('terminal:renameTab2'),
            onClick: onRename
          },
          {
            key: 'reset-name',
            icon: <RotateCcw size={14} />,
            label: t('terminal:resetTabName'),
            onClick: onResetName,
            disabled: !onResetName
          },
          { type: 'divider' as const }
        ]
      : []),
    { key: 'close', icon: <X size={14} />, label: t('terminal:close'), onClick: onClose },
    { type: 'divider' as const },
    {
      key: 'close-all',
      icon: <PanelsTopLeft size={14} />,
      label: t('terminal:closeAllTabs'),
      onClick: onCloseAll
    },
    {
      key: 'close-other',
      icon: <PanelLeftClose size={14} />,
      label: t('terminal:closeOtherTabs'),
      onClick: () => onCloseOther(tab)
    },
    {
      key: 'close-right',
      icon: <ArrowRightToLine size={14} />,
      label: t('terminal:closeTabsToTheRight'),
      onClick: () => onCloseToRight(tab)
    }
  ]
  const content = (
    <div
      draggable
      onDragStart={(event) => onDragStart(event, tab)}
      onDragOver={(event) => onDragOver(event, tab)}
      onDrop={(event) => onDrop(event, tab)}
      onDragEnd={onDragEnd}
      className={cn(
        'workspace-tab workspace-tab-enter relative items-center justify-center h-7 text-[var(--mac-secondary-text)] [font:var(--app-text-12)/var(--app-text-16)_var(--font-family-sans)] bg-transparent border border-transparent rounded-[7px]',
        'flex-none gap-1.5 py-0 px-2 whitespace-nowrap cursor-default [transition:color_120ms_ease,_background-color_120ms_ease,_border-color_120ms_ease,_box-shadow_120ms_ease,_transform_140ms_ease]',
        '[-webkit-app-region:no-drag] [&[draggable=true]]:cursor-grab [&.is-dragging]:opacity-42 [&.is-drop-before]:relative',
        '[&.is-drop-after]:relative [&.is-drop-before::before]:absolute [&.is-drop-before::before]:top-[3px] [&.is-drop-before::before]:bottom-[3px]',
        "[&.is-drop-before::before]:w-[3px] [&.is-drop-before::before]:content-[''] [&.is-drop-before::before]:bg-[#0a84ff]",
        '[&.is-drop-before::before]:rounded-[2px] [&.is-drop-before::before]:shadow-[0_0_0_2px_rgba(10,_132,_255,_0.18)]',
        '[&.is-drop-before::before]:animate-in [&.is-drop-before::before]:fade-in-0 [&.is-drop-before::before]:duration-150',
        '[&.is-drop-before::before]:left-[-3px] [&.is-drop-after::after]:absolute [&.is-drop-after::after]:top-[3px]',
        "[&.is-drop-after::after]:bottom-[3px] [&.is-drop-after::after]:w-[3px] [&.is-drop-after::after]:content-['']",
        '[&.is-drop-after::after]:bg-[#0a84ff] [&.is-drop-after::after]:rounded-[2px]',
        '[&.is-drop-after::after]:shadow-[0_0_0_2px_rgba(10,_132,_255,_0.18)] [&.is-drop-after::after]:animate-in [&.is-drop-after::after]:fade-in-0',
        '[&.is-drop-after::after]:duration-150 [&.is-drop-after::after]:right-[-3px] hover:text-[var(--mac-text)]',
        'hover:bg-[var(--workspace-tab-hover-background)] hover:border-[var(--workspace-tab-hover-border)] [&.is-active]:text-[var(--workspace-tab-active-text)]',
        '[&.is-active]:bg-[var(--workspace-tab-active-background)] [&.is-active]:border-[var(--workspace-tab-active-border)] [&.is-active]:shadow-[var(--workspace-tab-active-shadow)] [&.is-active]:font-medium',
        'workspace-session-tab flex min-w-0 pt-0 pr-0.5 pb-0 pl-2 [&_>_button:first-child]:flex [&_>_button:first-child]:items-center',
        '[&_>_button:first-child]:min-w-22 [&_>_button:first-child]:max-w-45 [&_>_button:first-child]:gap-1.5 [&_>_button:first-child]:h-7',
        '[&_>_button:first-child]:py-0 [&_>_button:first-child]:px-1 [&_>_button:first-child]:overflow-hidden [&_>_button:first-child]:text-inherit',
        '[&_>_button:first-child]:[font:inherit] [&_>_button:first-child]:text-ellipsis [&_>_button:first-child]:whitespace-nowrap',
        '[&_>_button:first-child]:bg-transparent [&_>_button:first-child]:border-0',
        `[&_>_button:first-child_>_svg]:flex-none${active ? ' is-active' : ''}${dragging ? ' is-dragging' : ''}${dropPosition ? ` is-drop-${dropPosition}` : ''}`
      )}
    >
      <button type="button" role="tab" aria-selected={active} onClick={onActivate}>
        {icon}
        <span className="workspace-tab-label min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
          {label}
        </span>
        {number !== undefined && (
          <span className="workspace-tab-number shrink-0 opacity-65 tabular-nums"> · {number}</span>
        )}
      </button>
      <button
        type="button"
        className={cn(
          'workspace-tab-close inline-flex items-center justify-center h-7 text-[var(--mac-secondary-text)] [font:var(--app-text-12)/1_var(--font-family-sans)]',
          'bg-transparent border-0 rounded-[7px] w-7 p-0 opacity-66 hover:opacity-100'
        )}
        aria-label={t('terminal:closeTabLabel', {
          label: `${label}${number !== undefined ? ` · ${number}` : ''}`
        })}
        onClick={onClose}
      >
        <X size={13} />
      </button>
    </div>
  )
  return (
    <ActionMenu context actions={menuItems} onOpenChange={setMenuOpen}>
      {tooltip ? (
        <HoverDetails disabled={menuOpen || dragging} content={tooltip}>
          {content}
        </HoverDetails>
      ) : (
        content
      )}
    </ActionMenu>
  )
}

type TerminalSessionTabProps = DragProps & {
  session: TerminalSession
  active: boolean
  host: domain.Host | null
  onActivate: () => void
  onClose: () => void
  onOpenFiles: (host: domain.Host, afterTab: WorkspaceTabID) => void
  onOpenFilesSplit: (host: domain.Host, terminalID: string) => void
  onCopyCommand: (host: domain.Host) => void
  onReconnect: (host: domain.Host) => void
  onCloseAll: () => void
  onCloseOther: (tab: WorkspaceTabID) => void
  onCloseToRight: (tab: WorkspaceTabID) => void
}

export function TerminalSessionTab({
  session,
  active,
  dragging,
  dropPosition,
  host,
  onActivate,
  onClose,
  onOpenFiles,
  onOpenFilesSplit,
  onCopyCommand,
  onReconnect,
  onCloseAll,
  onCloseOther,
  onCloseToRight,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd
}: TerminalSessionTabProps) {
  useTranslation()
  const terminalTab: WorkspaceTabID = `terminal:${session.id}`
  const SessionIcon = hasRemoteConnection(session) ? Server : Terminal
  const [menuOpen, setMenuOpen] = useState(false)
  const menuItems = [
    { key: 'close', icon: <X size={14} />, label: t('terminal:close'), onClick: onClose },
    ...(host
      ? [
          { type: 'divider' as const },
          {
            key: 'reconnect',
            icon: <RotateCcw size={14} />,
            label: t('terminal:reconnect'),
            onClick: () => onReconnect(host)
          },
          {
            key: 'files',
            icon: <FolderOpen size={14} />,
            label: t('terminal:openSftp'),
            onClick: () => onOpenFiles(host, terminalTab)
          },
          {
            key: 'files-split',
            icon: <SquareSplitHorizontal size={14} />,
            label: t('terminal:openSftpInSplitView'),
            onClick: () => onOpenFilesSplit(host, session.id)
          },
          {
            key: 'copy-command',
            icon: <Copy size={14} />,
            label: t('terminal:copySshCommand'),
            onClick: () => onCopyCommand(host)
          }
        ]
      : []),
    { type: 'divider' as const },
    {
      key: 'close-all',
      icon: <PanelsTopLeft size={14} />,
      label: t('terminal:closeAllTabs'),
      onClick: onCloseAll
    },
    {
      key: 'close-other',
      icon: <PanelLeftClose size={14} />,
      label: t('terminal:closeOtherTabs'),
      onClick: () => onCloseOther(terminalTab)
    },
    {
      key: 'close-right',
      icon: <ArrowRightToLine size={14} />,
      label: t('terminal:closeTabsToTheRight'),
      onClick: () => onCloseToRight(terminalTab)
    }
  ]
  const tabContent = (
    <div
      draggable
      onDragStart={(event) => onDragStart(event, terminalTab)}
      onDragOver={(event) => onDragOver(event, terminalTab)}
      onDrop={(event) => onDrop(event, terminalTab)}
      onDragEnd={onDragEnd}
      className={cn(
        'workspace-tab workspace-tab-enter relative items-center justify-center h-7 text-[var(--mac-secondary-text)] [font:var(--app-text-12)/var(--app-text-16)_var(--font-family-sans)] bg-transparent border border-transparent rounded-[7px]',
        'flex-none gap-1.5 py-0 px-2 whitespace-nowrap cursor-default [transition:color_120ms_ease,_background-color_120ms_ease,_border-color_120ms_ease,_box-shadow_120ms_ease,_transform_140ms_ease]',
        '[-webkit-app-region:no-drag] [&[draggable=true]]:cursor-grab [&.is-dragging]:opacity-42 [&.is-drop-before]:relative',
        '[&.is-drop-after]:relative [&.is-drop-before::before]:absolute [&.is-drop-before::before]:top-[3px] [&.is-drop-before::before]:bottom-[3px]',
        "[&.is-drop-before::before]:w-[3px] [&.is-drop-before::before]:content-[''] [&.is-drop-before::before]:bg-[#0a84ff]",
        '[&.is-drop-before::before]:rounded-[2px] [&.is-drop-before::before]:shadow-[0_0_0_2px_rgba(10,_132,_255,_0.18)]',
        '[&.is-drop-before::before]:animate-in [&.is-drop-before::before]:fade-in-0 [&.is-drop-before::before]:duration-150',
        '[&.is-drop-before::before]:left-[-3px] [&.is-drop-after::after]:absolute [&.is-drop-after::after]:top-[3px]',
        "[&.is-drop-after::after]:bottom-[3px] [&.is-drop-after::after]:w-[3px] [&.is-drop-after::after]:content-['']",
        '[&.is-drop-after::after]:bg-[#0a84ff] [&.is-drop-after::after]:rounded-[2px]',
        '[&.is-drop-after::after]:shadow-[0_0_0_2px_rgba(10,_132,_255,_0.18)] [&.is-drop-after::after]:animate-in [&.is-drop-after::after]:fade-in-0',
        '[&.is-drop-after::after]:duration-150 [&.is-drop-after::after]:right-[-3px] hover:text-[var(--mac-text)]',
        'hover:bg-[var(--workspace-tab-hover-background)] hover:border-[var(--workspace-tab-hover-border)] [&.is-active]:text-[var(--workspace-tab-active-text)]',
        '[&.is-active]:bg-[var(--workspace-tab-active-background)] [&.is-active]:border-[var(--workspace-tab-active-border)] [&.is-active]:shadow-[var(--workspace-tab-active-shadow)] [&.is-active]:font-medium',
        'workspace-session-tab flex min-w-0 pt-0 pr-0.5 pb-0 pl-2 [&_>_button:first-child]:flex [&_>_button:first-child]:items-center',
        '[&_>_button:first-child]:min-w-22 [&_>_button:first-child]:max-w-45 [&_>_button:first-child]:gap-1.5 [&_>_button:first-child]:h-7',
        '[&_>_button:first-child]:py-0 [&_>_button:first-child]:px-1 [&_>_button:first-child]:overflow-hidden [&_>_button:first-child]:text-inherit',
        '[&_>_button:first-child]:[font:inherit] [&_>_button:first-child]:text-ellipsis [&_>_button:first-child]:whitespace-nowrap',
        '[&_>_button:first-child]:bg-transparent [&_>_button:first-child]:border-0',
        `[&_>_button:first-child_>_svg]:flex-none${active ? ' is-active' : ''}${dragging ? ' is-dragging' : ''}${dropPosition ? ` is-drop-${dropPosition}` : ''}`
      )}
    >
      <button type="button" role="tab" aria-selected={active} onClick={onActivate}>
        <i className={`terminal-status [.workspace-session-tab_&]:flex-none is-${session.state}`} />
        <SessionIcon size={14} aria-hidden="true" />
        {session.title}
      </button>
      <button
        type="button"
        className={cn(
          'workspace-tab-close inline-flex items-center justify-center h-7 text-[var(--mac-secondary-text)] [font:var(--app-text-12)/1_var(--font-family-sans)]',
          'bg-transparent border-0 rounded-[7px] w-7 p-0 opacity-66 hover:opacity-100'
        )}
        aria-label={t('terminal:closeTabLabel', { label: session.title })}
        onClick={onClose}
      >
        <X size={13} />
      </button>
    </div>
  )
  return (
    <ActionMenu context actions={menuItems} onOpenChange={setMenuOpen}>
      {host ? (
        <HoverDetails
          disabled={menuOpen}
          content={
            <HostDetails
              host={host}
              onOpenFiles={() => onOpenFiles(host, terminalTab)}
              onCopyCommand={() => onCopyCommand(host)}
            />
          }
        >
          {tabContent}
        </HoverDetails>
      ) : (
        tabContent
      )}
    </ActionMenu>
  )
}

function HostDetails({
  host,
  onOpenFiles,
  onCopyCommand
}: {
  host: domain.Host
  onOpenFiles: () => void
  onCopyCommand: () => void
}) {
  useTranslation()
  return (
    <dl
      className={cn(
        'workspace-host-details grid min-w-55 max-w-80 gap-1.5 m-0 [&_>_div]:grid [&_>_div]:grid-cols-[64px_minmax(0,_1fr)] [&_>_div]:gap-2.5',
        '[&_dt]:text-[var(--mac-secondary-text)] [&_dt]:text-xs [&_dd]:min-w-0 [&_dd]:m-0 [&_dd]:text-[var(--mac-text)] [&_dd]:text-xs',
        '[&_dd]:wrap-anywhere'
      )}
    >
      <div>
        <dt>{t('hosts:hostDisplayName')}</dt>
        <dd>{host.name}</dd>
      </div>
      {host.alias && host.alias !== host.name && (
        <div>
          <dt>{t('hosts:hostAlias')}</dt>
          <dd>{host.alias}</dd>
        </div>
      )}
      <div>
        <dt>{t('terminal:address')}</dt>
        <dd>{host.hostName}</dd>
      </div>
      {host.user && (
        <div>
          <dt>{t('terminal:username')}</dt>
          <dd>{host.user}</dd>
        </div>
      )}
      {host.remark && (
        <div>
          <dt>{t('terminal:notes')}</dt>
          <dd>{host.remark}</dd>
        </div>
      )}
      <div className="workspace-host-details-action [.workspace-host-details_>_&]:block [.workspace-host-details_>_&]:justify-self-start [.workspace-host-details_>_&]:mt-1 [&_[data-slot=button]_+_[data-slot=button]]:ml-2">
        <ActionButton variant="default" size="sm" onClick={onOpenFiles}>
          SFTP
        </ActionButton>
        <ActionButton size="sm" onClick={onCopyCommand}>
          {t('terminal:copySshCommand')}
        </ActionButton>
      </div>
    </dl>
  )
}
