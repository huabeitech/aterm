import { useWorkspaceAvailability } from '../plugins/useWorkspaceAvailability'
import { usePluginCatalog } from '../plugins/pluginCatalog'
import { PluginWorkspace } from '../plugins/PluginWorkspace'
import { hasRemoteConnection } from '../../../../shared/contracts/terminal'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useImperativeHandle, useRef, useState, type ReactNode, type Ref } from 'react'
import { FolderOpen, Terminal, Columns2, ArrowDownUp, Box, Cable } from 'lucide-react'
import { SplitLayout } from '@/components/ui/split-layout'
import { ActionButton } from '@/components/ui/workbench-controls'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import type { FileSession, FileTransfer } from '../../../../shared/contracts/files'
import './connection.css'

export type ConnectionView =
  | 'terminal'
  | 'files'
  | 'tunnels'
  | 'split'
  | `plugin:${string}:${string}`
type Props = {
  cycleViewRef: Ref<() => void>
  active: boolean
  view: ConnectionView
  session: TerminalSession
  fileSession?: FileSession
  transfers: FileTransfer[]
  tunnels: ReactNode
  terminal: ReactNode
  renderFiles: (target: HTMLElement | null, transferRequest: number) => ReactNode
  onReconnectSSH: () => void
  onReconnectSFTP: () => void
  onViewChange: (view: ConnectionView) => void
}

export function ConnectionWorkspace({
  cycleViewRef,
  active,
  view,
  session,
  fileSession,
  transfers,
  terminal,
  tunnels,
  renderFiles,
  onReconnectSSH,
  onReconnectSFTP,
  onViewChange
}: Props) {
  useTranslation()
  const catalog = usePluginCatalog()
  const availability = useWorkspaceAvailability(catalog, session, active)
  const configuredViews = catalog
    .filter((p) => p.enabled)
    .flatMap((plugin) =>
      (plugin.manifest.contributes.views ?? [])
        .filter((v) => v.placement === 'terminal.workspace')
        .map((v) => ({
          plugin,
          view: v,
          key: `plugin:${plugin.manifest.id}:${v.id}` as ConnectionView
        }))
    )
  const views = configuredViews.filter(
    (item) =>
      item.view.availability !== 'dynamic' ||
      availability[item.plugin.manifest.id]?.[item.view.id]?.visible !== false
  )
  const selectedPlugin = views.find((v) => v.key === view)
  const pluginSelected = view.startsWith('plugin:')
  const openedPlugins = useRef(new Set<string>())
  if (selectedPlugin) openedPlugins.current.add(selectedPlugin.key)
  const remote = hasRemoteConnection(session)
  useImperativeHandle(active ? cycleViewRef : null, () => () => {
    const order: ConnectionView[] = ['terminal']
    if (remote) order.push('files')
    if (session.hostID) order.push('tunnels')
    for (const item of views) order.push(item.key)
    if (order.length < 2) return
    // Split is a separate control; start the tab cycle from Terminal in split mode.
    const current = order.indexOf(view === 'split' ? 'terminal' : view)
    onViewChange(order[(current + 1) % order.length])
  })
  useEffect(() => {
    if (pluginSelected && !selectedPlugin) onViewChange('terminal')
  }, [pluginSelected, !!selectedPlugin])
  const workspaceRef = useRef<HTMLElement>(null)
  useEffect(() => {
    if (!active || view === 'split' || pluginSelected) return
    const frame = requestAnimationFrame(() => {
      if (document.querySelector('[role="dialog"]')) return
      workspaceRef.current
        ?.querySelector<HTMLElement>(
          view === 'files'
            ? '.files-table-scroll'
            : '.terminal-pane.is-focused .xterm-helper-textarea'
        )
        ?.focus({ preventScroll: true })
    })
    return () => cancelAnimationFrame(frame)
  }, [active, view, fileSession?.state])
  const terminalOpened = useRef(view !== 'files')
  if (view !== 'files') terminalOpened.current = true
  const [statusTarget, setStatusTarget] = useState<HTMLDivElement | null>(null)
  const [transferRequest, setTransferRequest] = useState(0)
  const [size, setSize] = useState<number | string>('50%')
  const activeTransfers = transfers.filter((t) => t.state === 'running' || t.state === 'queued')
  const failed = transfers.filter((t) => t.state === 'failed').length
  const total = activeTransfers.reduce((sum, t) => sum + t.total, 0)
  const transferred = activeTransfers.reduce((sum, t) => sum + t.transferred, 0)
  const progress = total ? Math.min(100, Math.round((transferred / total) * 100)) : null
  const fileVisible = view === 'files' || view === 'split'
  const connectionNotices = [
    ...(terminalOpened.current
      ? [
          {
            protocol: remote ? 'SSH' : 'Terminal',
            state: session.state,
            error: session.error,
            retry: onReconnectSSH
          }
        ]
      : []),
    ...(fileSession
      ? [
          {
            protocol: 'SFTP',
            state: fileSession.state,
            error: fileSession.error,
            retry: onReconnectSFTP
          }
        ]
      : [])
  ].filter(({ state }) => state === 'connecting' || state === 'error' || state === 'exited')
  const files = (
    <div className="file-workspace-container h-full min-h-0" data-active={active && fileVisible}>
      {fileSession ? (
        renderFiles(statusTarget, transferRequest)
      ) : (
        <div className="connection-empty">
          <FolderOpen size={24} aria-hidden="true" />
          <span>{t('terminal:openFilesOnThisServer')}</span>
          <ActionButton onClick={() => onViewChange(view === 'split' ? 'split' : 'files')}>
            {t('terminal:connectSftp')}
          </ActionButton>
        </div>
      )}
    </div>
  )
  // Keep the original local terminal chrome when no workspace plugin is enabled.
  if (!remote && !configuredViews.length) return terminal
  return (
    <section
      ref={workspaceRef}
      data-active={active}
      className="connection-workspace"
      style={{ display: active ? 'flex' : 'none' }}
      aria-label={t('terminal:hostConnection', { host: session.title })}
    >
      <div className="connection-content" data-view={view}>
        {views
          .filter((item) => openedPlugins.current.has(item.key))
          .map((item) => (
            <div
              key={`${session.id}:${item.key}`}
              style={{ display: view === item.key ? 'block' : 'none', height: '100%' }}
            >
              <PluginWorkspace
                plugin={item.plugin}
                session={session}
                viewId={item.view.id}
                onClose={() => onViewChange('terminal')}
                active={active && view === item.key}
              />
            </div>
          ))}
        <div style={{ display: pluginSelected || view === 'tunnels' ? 'none' : 'contents' }}>
          <SplitLayout
            className="connection-split h-full min-h-0"
            size={size}
            min={240}
            max="80%"
            minimumOther={240}
            onResize={(sizes) => setSize(sizes[0])}
            first={
              <div
                className="connection-terminal h-full"
                style={{ display: view === 'files' ? 'none' : 'block' }}
              >
                {terminalOpened.current && terminal}
              </div>
            }
            second={files}
          />
        </div>
        {view === 'tunnels' && tunnels}
        {/* Keep the same mounted panels when moving between single and split views. */}
      </div>
      <footer
        role="toolbar"
        className="connection-statusbar"
        aria-label={t('terminal:connectionStatusAndViews')}
      >
        <div className="connection-views" role="group" aria-label={t('terminal:connectionView')}>
          <button
            type="button"
            aria-pressed={view === 'terminal' || view === 'split'}
            onClick={() => onViewChange('terminal')}
          >
            <Terminal size={14} aria-hidden="true" />
            <span>{t(remote ? 'terminal:sshTerminal' : 'terminal:terminal')}</span>
          </button>
          {remote && (
            <button type="button" aria-pressed={fileVisible} onClick={() => onViewChange('files')}>
              <FolderOpen size={14} aria-hidden="true" />
              <span>{t('terminal:sftpFiles')}</span>
            </button>
          )}
          {session.hostID && (
            <button
              type="button"
              aria-pressed={view === 'tunnels'}
              onClick={() => onViewChange('tunnels')}
            >
              <Cable size={14} aria-hidden="true" />
              <span>Tunnels</span>
            </button>
          )}
          {views.map((item) => (
            <button
              key={item.key}
              type="button"
              aria-pressed={view === item.key}
              aria-label={item.view.title}
              title={availability[item.plugin.manifest.id]?.[item.view.id]?.message}
              onClick={() => onViewChange(item.key)}
            >
              <Box size={14} aria-hidden="true" />
              <span>{item.view.title}</span>
              {availability[item.plugin.manifest.id]?.[item.view.id]?.available === false && (
                <span className="connection-state-dot" data-state="error" aria-hidden="true" />
              )}
            </button>
          ))}
        </div>
        {connectionNotices.map(({ protocol, state, error, retry }) => (
          <div
            key={protocol}
            className="connection-session-state"
            data-state={state}
            role="status"
            title={error}
          >
            <span className="connection-state-dot" data-state={state} aria-hidden="true" />
            <span>
              {state === 'connecting'
                ? t('terminal:connectingProtocol', { protocol })
                : state === 'exited'
                  ? t('terminal:protocolDisconnected', { protocol })
                  : t('terminal:protocolFailed', { protocol })}
            </span>
            {state !== 'connecting' && (
              <button
                type="button"
                onClick={retry}
                aria-label={t('terminal:reconnectProtocol', { protocol })}
              >
                {t('terminal:retry')}
              </button>
            )}
          </div>
        ))}
        <div className="connection-details">
          <div
            ref={setStatusTarget}
            className="connection-file-details"
            style={{ display: fileVisible ? 'flex' : 'none' }}
          />
        </div>
        {!fileVisible && transfers.length > 0 && (
          <button
            type="button"
            className="connection-transfer"
            data-error={failed > 0}
            onClick={() => {
              onViewChange('files')
              setTransferRequest((n) => n + 1)
            }}
          >
            <ArrowDownUp size={14} />
            <span>
              {failed
                ? t('terminal:transfersFailed', { count: failed })
                : activeTransfers.length
                  ? t('terminal:transferring', {
                      count: activeTransfers.length,
                      progress: progress === null ? '' : `· ${progress}%`
                    })
                  : t('terminal:transfers')}
            </span>
          </button>
        )}
        {remote && (
          <button
            type="button"
            className="connection-split-button"
            aria-label={t('terminal:splitTerminalAndFiles')}
            title={t('terminal:showTerminalAndFilesSideBySide')}
            aria-pressed={view === 'split'}
            onClick={() => onViewChange(view === 'split' ? 'terminal' : 'split')}
          >
            <Columns2 size={14} />
            <span>{t('terminal:split')}</span>
          </button>
        )}
      </footer>
    </section>
  )
}
