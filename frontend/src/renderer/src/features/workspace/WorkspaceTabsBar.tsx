import { usePluginCatalog } from '../plugins/pluginCatalog'
import { pluginTitle, PluginIcon } from '../plugins/NativeWorkspace'
import { hasRemoteConnection } from '../../../../shared/contracts/terminal'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { IconButton, Tip } from '@/components/ui/desktop'
import { HoverDetailsGroup, type MenuAction } from '@/components/ui/workbench-controls'
import { useAppUI } from '@/components/ui/confirm-provider'
import type { DragEvent, RefObject, WheelEvent } from 'react'

import {
  ChevronLeft,
  ChevronRight,
  Copy,
  Crosshair,
  FolderOpen,
  Plus,
  Search,
  Server,
  Settings,
  Terminal,
} from 'lucide-react'
import { desktop } from '../../api/app'
import { domain } from '../../api/types'
import type { TerminalSession } from '../../components/terminal/types'
import { formatCommandPaletteShortcut } from '../../components/common/commandPaletteShortcut'
import { WorkspaceTab } from './WorkspaceTab'
import type {
  WorkspaceTab as WorkspaceTabModel,
  WorkspaceTabDropPosition,
  WorkspaceTabID,
} from './workspaceModel'

interface WorkspaceTabsBarProps {
  tabs: WorkspaceTabModel[]
  activeTab: WorkspaceTabID
  hosts: domain.Host[]
  fileSessions: import('../../../../shared/contracts/files').FileSession[]
  sessions: TerminalSession[]
  draggingTab: WorkspaceTabID | null
  dropTarget: { id: WorkspaceTabID; position: WorkspaceTabDropPosition } | null
  scrollState: { canScrollBack: boolean; canScrollForward: boolean }
  containerRef: RefObject<HTMLDivElement>
  commandPaletteShortcut?: string
  newTerminalShortcut?: string
  onOpenSettings: () => void
  onCloseSettings: () => void
  onActivateTab: (tab: WorkspaceTabID) => void
  onActivateTerminal: (id: string) => void
  terminalActions: (id: string) => MenuAction[]
  onRenameTerminal: (id: string) => void
  onResetTerminalName: (id: string) => void
  onCloseTerminal: (id: string) => void
  onCloseFiles: (tab: WorkspaceTabID) => void
  onOpenFiles: (host: domain.Host, afterTab: WorkspaceTabID) => void
  onOpenFilesSplit: (host: domain.Host, terminalID: string) => void
  onCopyCommand: (host: domain.Host) => void
  onLocateHost: (hostID: string, tab: WorkspaceTabID) => void
  onReconnect: (host: domain.Host) => void
  onCloseAll: () => void
  onCloseOther: (tab: WorkspaceTabID) => void
  onCloseToRight: (tab: WorkspaceTabID) => void
  onDragStart: (event: DragEvent<HTMLElement>, tab: WorkspaceTabID) => void
  onDragOver: (event: DragEvent<HTMLElement>, tab: WorkspaceTabID) => void
  onDrop: (event: DragEvent<HTMLElement>, tab: WorkspaceTabID) => void
  onDragEnd: () => void
  onWheel: (event: WheelEvent<HTMLDivElement>) => void
  onScroll: (direction: -1 | 1) => void
  onOpenLocalTerminal: () => void
  localTerminalAvailable: boolean
  onOpenSearch: () => void
}

export function WorkspaceTabsBar(props: WorkspaceTabsBarProps) {
  const { i18n } = useTranslation()
  const plugins = usePluginCatalog()
  return (
    <HoverDetailsGroup>
      <div
        className="workspace-tabs flex flex-1 items-center min-w-0 gap-1.5 overflow-x-auto [scrollbar-width:none] select-none [-webkit-app-region:drag] [&::-webkit-scrollbar]:hidden"
        ref={props.containerRef}
        role="tablist"
        aria-label={t('terminal:workspaces')}
        onWheel={props.onWheel}
      >
        {props.tabs.map((workspaceTab) => {
          const tab = workspaceTab.id
          if (tab.startsWith('plugin:')) {
            const [, pluginID, viewID] = tab.split(':')
            const plugin = plugins.find((p) => p.manifest.id === pluginID)
            const view = plugin?.manifest.contributes.views?.find((v) => v.id === viewID)
            return (
              <WorkspaceTab
                key={tab}
                tab={tab}
                label={
                  plugin && view
                    ? pluginTitle(plugin, view.title, i18n.resolvedLanguage ?? i18n.language)
                    : pluginID
                }
                icon={<PluginIcon view={view} />}
                active={props.activeTab === tab}
                dragging={props.draggingTab === tab}
                dropPosition={dropPosition(props, tab)}
                onActivate={() => props.onActivateTab(tab)}
                onClose={() => props.onCloseFiles(tab)}
                onCloseAll={props.onCloseAll}
                onCloseOther={props.onCloseOther}
                onCloseToRight={props.onCloseToRight}
                onDragStart={props.onDragStart}
                onDragOver={props.onDragOver}
                onDrop={props.onDrop}
                onDragEnd={props.onDragEnd}
              />
            )
          }
          if (tab === 'settings') {
            return (
              <WorkspaceTab
                key={tab}
                tab={tab}
                label={t('terminal:settings')}
                icon={<Settings size={14} />}
                active={props.activeTab === tab}
                dragging={props.draggingTab === tab}
                dropPosition={dropPosition(props, tab)}
                onActivate={props.onOpenSettings}
                onClose={props.onCloseSettings}
                onCloseAll={props.onCloseAll}
                onCloseOther={props.onCloseOther}
                onCloseToRight={props.onCloseToRight}
                onDragStart={props.onDragStart}
                onDragOver={props.onDragOver}
                onDrop={props.onDrop}
                onDragEnd={props.onDragEnd}
              />
            )
          }
          if (tab.startsWith('files:')) {
            const fileSession = props.fileSessions.find((item) => `files:${item.id}` === tab)
            return fileSession ? (
              <WorkspaceTab
                key={tab}
                tab={tab}
                label={`${fileSession.title} · SFTP`}
                icon={<FolderOpen size={14} />}
                active={props.activeTab === tab}
                dragging={props.draggingTab === tab}
                dropPosition={dropPosition(props, tab)}
                onActivate={() => props.onActivateTab(tab)}
                onClose={() => props.onCloseFiles(tab)}
                onCloseAll={props.onCloseAll}
                onCloseOther={props.onCloseOther}
                onCloseToRight={props.onCloseToRight}
                onDragStart={props.onDragStart}
                onDragOver={props.onDragOver}
                onDrop={props.onDrop}
                onDragEnd={props.onDragEnd}
              />
            ) : null
          }
          const session = props.sessions.find((item) => `terminal:${item.id}` === tab)
          const hostID = workspaceTab.hostID ?? session?.hostID
          const host = props.hosts.find((item) => item.id === hostID)
          return session || workspaceTab.hostID ? (
            <WorkspaceTab
              key={tab}
              tab={tab}
              actions={session ? props.terminalActions(session.id) : undefined}
              label={(workspaceTab.hostID && host?.name) || session?.title || t('hosts:hostDetails')}
              number={session?.localNumber}
              onRename={session?.localNumber ? () => props.onRenameTerminal(session.id) : undefined}
              onResetName={
                session?.customTitle ? () => props.onResetTerminalName(session.id) : undefined
              }
              tooltip={
                session || host ? (
                  <TerminalTabDetails
                    session={session}
                    host={host}
                    onCopyCommand={props.onCopyCommand}
                    onLocateHost={(hostID) => props.onLocateHost(hostID, tab)}
                  />
                ) : undefined
              }
              icon={hostID || (session && hasRemoteConnection(session)) ? <Server size={14} /> : <Terminal size={14} />}
              active={props.activeTab === tab}
              dragging={props.draggingTab === tab}
              dropPosition={dropPosition(props, tab)}
              onActivate={() => session ? props.onActivateTerminal(session.id) : props.onActivateTab(tab)}
              onClose={() => session ? props.onCloseTerminal(session.id) : props.onCloseFiles(tab)}
              onCloseAll={props.onCloseAll}
              onCloseOther={props.onCloseOther}
              onCloseToRight={props.onCloseToRight}
              onDragStart={props.onDragStart}
              onDragOver={props.onDragOver}
              onDrop={props.onDrop}
              onDragEnd={props.onDragEnd}
            />
          ) : null
        })}
        {props.localTerminalAvailable && (
          <Tip
            title={t('terminal:newShellShortcut', {
              shortcut: formatCommandPaletteShortcut(props.newTerminalShortcut),
            })}
          >
            <button
              type="button"
              className={cn(
                'workspace-new-session [:root[data-theme=dark]_&:hover]:bg-[var(--mac-hover-background)] inline-flex items-center justify-center h-7',
                'text-[var(--mac-secondary-text)] [font:var(--app-text-12)/1_var(--font-family-sans)] bg-transparent border-0 rounded-[7px] [-webkit-app-region:no-drag] w-7',
                'p-0 hover:text-[var(--mac-text)] hover:bg-[var(--workspace-tab-hover-background)] focus-visible:relative focus-visible:z-1',
                'focus-visible:[outline:1px_solid_var(--mac-secondary-text)] focus-visible:[outline-offset:-1px]',
              )}
              aria-label={t('terminal:newLocalShell')}
              onClick={props.onOpenLocalTerminal}
            >
              <Plus size={16} />
            </button>
          </Tip>
        )}
      </div>
      {(props.scrollState.canScrollBack || props.scrollState.canScrollForward) && (
        <div className="workspace-tab-tools flex items-center gap-[3px] ml-2.5 [-webkit-app-region:no-drag]">
          <button
            type="button"
            className={cn(
              'workspace-tab-scroll [:root[data-theme=dark]_&:hover:not(:disabled)]:bg-[var(--mac-hover-background)] inline-flex items-center justify-center',
              'w-6 h-7 p-0 text-[var(--mac-secondary-text)] bg-transparent border-0 rounded-[5px] [&:hover:not(:disabled)]:text-[var(--mac-text)]',
              '[&:hover:not(:disabled)]:bg-[var(--workspace-tab-hover-background)] disabled:opacity-35 focus-visible:relative focus-visible:z-1',
              'focus-visible:[outline:1px_solid_var(--mac-secondary-text)] focus-visible:[outline-offset:-1px]',
            )}
            aria-label={t('terminal:showPreviousTabs')}
            disabled={!props.scrollState.canScrollBack}
            onClick={() => props.onScroll(-1)}
          >
            <ChevronLeft size={15} />
          </button>
          <button
            type="button"
            className={cn(
              'workspace-tab-scroll [:root[data-theme=dark]_&:hover:not(:disabled)]:bg-[var(--mac-hover-background)] inline-flex items-center justify-center',
              'w-6 h-7 p-0 text-[var(--mac-secondary-text)] bg-transparent border-0 rounded-[5px] [&:hover:not(:disabled)]:text-[var(--mac-text)]',
              '[&:hover:not(:disabled)]:bg-[var(--workspace-tab-hover-background)] disabled:opacity-35 focus-visible:relative focus-visible:z-1',
              'focus-visible:[outline:1px_solid_var(--mac-secondary-text)] focus-visible:[outline-offset:-1px]',
            )}
            aria-label={t('terminal:showNextTabs')}
            disabled={!props.scrollState.canScrollForward}
            onClick={() => props.onScroll(1)}
          >
            <ChevronRight size={15} />
          </button>
        </div>
      )}
      <div
        className="workspace-window-drag-region flex-[0_0_72px] self-stretch min-w-6 [-webkit-app-region:drag]"
        aria-hidden="true"
      />
      <div className="workspace-tab-actions flex items-center ml-3 [-webkit-app-region:no-drag]">
        <Tip
          title={t('terminal:searchShortcut', {
            shortcut: formatCommandPaletteShortcut(props.commandPaletteShortcut),
          })}
        >
          <button
            type="button"
            className={cn(
              'workspace-header-button inline-flex items-center justify-center h-7',
              'text-[var(--mac-secondary-text)] [font:var(--app-text-12)/1_var(--font-family-sans)] bg-transparent border border-transparent rounded-[7px] w-7 p-0',
              'hover:text-[var(--mac-text)] hover:bg-[var(--workspace-tab-hover-background)] hover:border-[var(--workspace-tab-hover-border)] focus-visible:relative focus-visible:z-1',
              'focus-visible:[outline:1px_solid_var(--mac-secondary-text)] focus-visible:[outline-offset:-1px]',
            )}
            aria-label={t('terminal:search')}
            onClick={props.onOpenSearch}
          >
            <Search size={16} />
          </button>
        </Tip>
      </div>
    </HoverDetailsGroup>
  )
}

function TerminalTabDetails({
  session,
  host,
  onCopyCommand,
  onLocateHost,
}: {
  session?: TerminalSession
  host?: domain.Host
  onCopyCommand: (host: domain.Host) => void
  onLocateHost: (hostID: string) => void
}) {
  useTranslation()
  const { message } = useAppUI()
  const copy = (label: string, value: string) => {
    void desktop.library.copyText(value).then(
      () => message.success(t('terminal:fieldCopied', { label })),
      () => message.error(t('terminal:copyFailed', { label })),
    )
  }
  const remote = session?.remote
  const user = remote?.user || host?.user
  const rows = (session ? hasRemoteConnection(session) : !!host)
    ? [
        [t('terminal:type'), 'SSH'],
        ...(host?.alias ? [[t('hosts:hostAlias'), host.alias]] : []),
        [t('hosts:address'), remote?.host || host?.hostName || t('terminal:unavailable')],
        [
          t('terminal:port'),
          remote
            ? String(remote.port)
            : host
              ? String(host.port)
              : t('terminal:unavailable'),
        ],
        ...(user ? [[t('terminal:username'), user]] : []),
      ]
    : [
        [t('terminal:type'), t('terminal:localTerminal')],
        [t('terminal:directory'), session?.cwd || t('terminal:unavailable')],
        [t('terminal:shell'), session?.shell || t('terminal:starting')],
      ]
  if (session) {
    rows.push([
      t('terminal:status'),
      {
        connecting: t('terminal:sessionConnecting'),
        running: t('terminal:sessionRunning'),
        exited: t('terminal:sessionExited'),
        error: t('terminal:sessionError'),
      }[session.state],
    ])
  }
  const copyLabel = t('hosts:copySshCommand')
  const locateLabel = t('hosts:locateHost')
  return (
    <div className="grid gap-2 text-[length:var(--app-text-13)] [-webkit-app-region:no-drag]">
      <dl
        className={cn(
          'workspace-host-details grid min-w-60 max-w-90 gap-1.5 m-0 [&_>_div]:grid [&_>_div]:grid-cols-[64px_minmax(0,_1fr)] [&_>_div]:gap-2.5',
          '[&_dt]:text-[var(--mac-secondary-text)] [&_dd]:min-w-0 [&_dd]:m-0 [&_dd]:text-[var(--mac-text)]',
          '[&_dd]:wrap-anywhere',
        )}
      >
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>
              {label === t('terminal:type') ||
              label === t('terminal:status') ||
              value === t('terminal:unavailable') ||
              value === t('terminal:starting') ? (
                value
              ) : (
                <button
                  type="button"
                  className="group inline-flex max-w-full items-start gap-1 p-0 text-left text-inherit bg-transparent border-0 cursor-copy hover:text-[var(--terminal-accent,#0a84ff)]"
                  aria-label={t('terminal:copyField', { label })}
                  title={t('terminal:copyField', { label })}
                  onClick={() => copy(label ?? '', value ?? '')}
                >
                  <span className="min-w-0 wrap-anywhere">{value}</span>
                  <Copy className="mt-px size-3 shrink-0 opacity-0 group-hover:opacity-100" />
                </button>
              )}
            </dd>
          </div>
        ))}
      </dl>
      {host && (
        <div
          className="flex justify-end gap-1 border-t border-border pt-2"
          role="group"
          aria-label={t('hosts:hostActions')}
        >
          <Tip title={copyLabel}>
            <IconButton
              aria-label={copyLabel}
              icon={<Copy aria-hidden="true" />}
              onClick={() => onCopyCommand(host)}
            />
          </Tip>
          <Tip title={locateLabel}>
            <IconButton
              aria-label={locateLabel}
              icon={<Crosshair aria-hidden="true" />}
              onClick={() => onLocateHost(host.id)}
            />
          </Tip>
        </div>
      )}
    </div>
  )
}

function dropPosition(props: WorkspaceTabsBarProps, tab: WorkspaceTabID) {
  return props.dropTarget?.id === tab ? props.dropTarget.position : null
}
