import { usePluginCatalog } from '../plugins/pluginCatalog'
import type { ConnectionView } from '../workspace/ConnectionWorkspace'
import { TerminalPluginWorkspace } from '../plugins/TerminalPluginBars'
import type { FileTarget } from '../../../../shared/contracts/files'
import { hasRemoteConnection, remoteFileTarget } from '../../../../shared/contracts/terminal'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useAppUI } from '@/components/ui/confirm-provider'
import { TextField, WorkbenchDialog, type MenuAction } from '@/components/ui/workbench-controls'
import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import {
  FolderOpen,
  Info,
  RotateCcw,
  Search,
  SquareSplitHorizontal,
  SquareSplitVertical,
  X
} from 'lucide-react'

import { desktop } from '../../api/app'
import type {
  TerminalPrompt,
  TerminalRequest,
  TerminalSession
} from '../../../../shared/contracts/terminal'
import type { useWorkspaceTabs } from '../workspace/useWorkspaceTabs'
import { TerminalPane } from './TerminalPane'
import { TerminalPromptDialog } from './TerminalPromptDialog'
import { terminalTheme } from './themes'
import {
  leaf,
  paneIDs,
  splitPane,
  removePane,
  resizeSplit,
  layoutPanes,
  type PaneNode,
  type Axis
} from './layout'
interface Group {
  id: string
  root: PaneNode
  focused: string
  localNumber?: number
  customTitle?: string
}
export function useTerminalWorkbench(
  tabs: ReturnType<typeof useWorkspaceTabs>,
  theme: string,
  fontSize: number,
  wordNavigation: 'shell' | 'terminal',
  unlock: () => Promise<boolean>,
  onConnected: () => void,
  onOpenFiles: (id: FileTarget, groupID: string) => void,
  onOpenHost: (id: string) => void,
  connectionViews: Record<string, ConnectionView>,
  renderConnection: (
    id: string,
    session: TerminalSession,
    terminal: ReactNode,
    onReconnect: () => void
  ) => ReactNode
) {
  useTranslation()
  const workspacePluginsAvailable = usePluginCatalog().some(
    (p) =>
      p.enabled && p.manifest.contributes.views?.some((v) => v.placement === 'terminal.workspace')
  )
  const { message, modal } = useAppUI()
  const [sessions, setSessions] = useState<Record<string, TerminalSession>>({}),
    [groups, setGroups] = useState<Group[]>([]),
    [prompts, setPrompts] = useState<TerminalPrompt[]>([])
  const [findRequest, setFindRequest] = useState<{ id: string; sequence: number } | null>(null)
  const current = useRef({ sessions, groups, unlock, onConnected })
  current.current = { sessions, groups, unlock, onConnected }
  const [renaming, setRenaming] = useState<{ id: string; name: string } | null>(null)
  const groupNodes = useRef(new Map<string, HTMLDivElement>())
  function report(error: unknown) {
    void message.error(error instanceof Error ? error.message : t('terminal:unableToOpenTerminal'))
  }
  function updateGroups(update: (groups: Group[]) => Group[]) {
    const next = update(current.current.groups)
    current.current.groups = next
    setGroups(next)
  }
  useEffect(
    () =>
      desktop.terminal.onEvent((event) => {
        if (event.type === 'state') {
          setSessions((previous) => ({ ...previous, [event.session.id]: event.session }))
          if (event.session.state === 'running' && hasRemoteConnection(event.session))
            current.current.onConnected()
        } else if (event.type === 'prompt') {
          setPrompts((previous) => [...previous, event.prompt])
        } else if (event.type === 'prompt-closed')
          setPrompts((previous) => previous.filter((p) => p.id !== event.id))
      }),
    []
  )
  useEffect(() => {
    // React after both the session and its pane are registered, including fast-exiting shells.
    for (const session of Object.values(sessions)) {
      if (session.state !== 'exited') continue
      const group = current.current.groups.find((g) => paneIDs(g.root).includes(session.id))
      // A connection that has used files must survive its SSH process exiting.
      if (group && !(session.kind === 'ssh' && connectionViews[group.id]))
        closePane(group.id, session.id)
    }
  }, [sessions, groups, connectionViews])
  async function create(request: TerminalRequest) {
    if (
      request.kind === 'ssh' &&
      (await desktop.terminal.requiresUnlock(request.hostID)) &&
      !(await current.current.unlock())
    )
      return null
    const session = await desktop.terminal.open(request)
    setSessions((previous) => ({ ...previous, [session.id]: session }))
    return session
  }
  async function open(request: TerminalRequest, onCreated?: (id: string) => void) {
    try {
      const session = await create(request)
      if (!session) return
      onCreated?.(session.id)
      updateGroups((previous) => [
        ...previous,
        {
          id: session.id,
          root: leaf(session.id),
          focused: session.id,
          localNumber: session.localNumber
        }
      ])
      tabs.openWorkspaceTab(`terminal:${session.id}`)
      tabs.setActiveWorkspaceTab(`terminal:${session.id}`)
      return session.id
    } catch (error) {
      report(error)
    }
  }
  useEffect(() => {
    const adopt = async (id: string) => {
      try {
        const session = (await desktop.terminal.list()).find((s) => s.id === id)
        if (!session || current.current.groups.some((g) => paneIDs(g.root).includes(id))) return
        setSessions((previous) => ({ ...previous, [id]: session }))
        updateGroups((previous) => [
          ...previous,
          { id, root: leaf(id), focused: id, localNumber: session.localNumber }
        ])
        tabs.openWorkspaceTab(`terminal:${id}`)
        tabs.setActiveWorkspaceTab(`terminal:${id}`)
      } catch (error) {
        report(error)
      }
    }
    return desktop.plugins.onTerminalOpened(adopt)
  }, [])
  function activate(id: string) {
    tabs.setActiveWorkspaceTab(`terminal:${id}`)
  }
  function closeGroup(id: string) {
    const group = current.current.groups.find((g) => g.id === id)
    if (!group) return
    for (const sessionID of paneIDs(group.root))
      void desktop.terminal.close(sessionID).catch(() => {})
    const removed = new Set(paneIDs(group.root))
    setSessions((previous) =>
      Object.fromEntries(Object.entries(previous).filter(([id]) => !removed.has(id)))
    )
    updateGroups((previous) => previous.filter((g) => g.id !== id))
    tabs.removeTabs([`terminal:${id}`])
  }
  function closePane(groupID: string, id: string) {
    const group = current.current.groups.find((g) => g.id === groupID)
    if (!group) return
    const root = removePane(group.root, id)
    if (!root) {
      closeGroup(groupID)
      return
    }
    void desktop.terminal.close(id).catch(() => {})
    setSessions((previous) => {
      const next = { ...previous }
      delete next[id]
      return next
    })
    updateGroups((previous) =>
      previous.map((g) =>
        g.id === groupID
          ? { ...g, root, focused: g.focused === id ? paneIDs(root)[0] : g.focused }
          : g
      )
    )
  }
  async function split(groupID: string, id: string, axis: Axis) {
    try {
      const group = current.current.groups.find((g) => g.id === groupID),
        source = current.current.sessions[id]
      if (!group || !source) return
      if (paneIDs(group.root).length >= 8) {
        void message.info(t('terminal:aWorkspaceCanContainUpTo8TerminalPanes'))
        return
      }
      const session = await create(
        source.kind === 'ssh'
          ? { kind: 'ssh', hostID: source.hostID! }
          : { kind: 'local', newTab: false }
      )
      if (!session) return
      const currentGroup = current.current.groups.find((g) => g.id === groupID)
      if (!currentGroup || !paneIDs(currentGroup.root).includes(id)) {
        await desktop.terminal.close(session.id)
        return
      }
      updateGroups((previous) =>
        previous.map((g) =>
          g.id === groupID
            ? { ...g, root: splitPane(g.root, id, session.id, axis), focused: session.id }
            : g
        )
      )
    } catch (error) {
      report(error)
    }
  }
  function reconnect(groupID: string, id: string) {
    const previous = current.current.sessions[id]
    if (!previous) return
    const run = async () => {
      try {
        const next = await create(
          previous.kind === 'ssh'
            ? { kind: 'ssh', hostID: previous.hostID! }
            : { kind: 'local', newTab: false }
        )
        if (!next) return
        const group = current.current.groups.find((g) => g.id === groupID)
        if (!group || !paneIDs(group.root).includes(id)) {
          await desktop.terminal.close(next.id)
          return
        }
        const replace = (node: PaneNode): PaneNode =>
          node.kind === 'leaf'
            ? node.id === id
              ? leaf(next.id)
              : node
            : { ...node, first: replace(node.first), second: replace(node.second) }
        updateGroups((groups) =>
          groups.map((g) =>
            g.id === groupID ? { ...g, root: replace(g.root), focused: next.id } : g
          )
        )
        await desktop.terminal.close(id)
        setSessions((previous) => {
          const rest = { ...previous }
          delete rest[id]
          return rest
        })
      } catch (error) {
        report(error)
      }
    }
    if (previous.state === 'running')
      modal.confirm({
        title:
          previous.kind === 'ssh'
            ? t('terminal:reconnectSshSession')
            : t('terminal:restartLocalShell'),
        content: t('terminal:theCurrentProcessWillBeStoppedCommandsWillNotBeReplayed'),
        okText: previous.kind === 'ssh' ? t('terminal:reconnect') : t('terminal:restart'),
        onOk: run
      })
    else void run()
  }
  function splitActive(axis: Axis) {
    const group = current.current.groups.find((g) => `terminal:${g.id}` === tabs.activeWorkspaceTab)
    if (group) void split(group.id, group.focused, axis)
  }
  function closeActive() {
    const group = current.current.groups.find((g) => `terminal:${g.id}` === tabs.activeWorkspaceTab)
    if (group) closePane(group.id, group.focused)
  }
  function findActive() {
    const group = current.current.groups.find((g) => `terminal:${g.id}` === tabs.activeWorkspaceTab)
    if (group)
      setFindRequest((previous) => ({ id: group.focused, sequence: (previous?.sequence ?? 0) + 1 }))
  }
  function tabActions(groupID: string): MenuAction[] {
    const group = current.current.groups.find((g) => g.id === groupID)
    const session = group && current.current.sessions[group.focused]
    if (!group || !session) return []
    const id = group.focused
    return [
      ...(remoteFileTarget(session)
        ? [
            {
              key: 'files',
              icon: <FolderOpen size={14} />,
              label: t('terminal:openSftp'),
              onClick: () => onOpenFiles(remoteFileTarget(session)!, groupID)
            }
          ]
        : []),
      {
        key: 'find',
        icon: <Search size={14} />,
        label: t('terminal:findInTerminal'),
        onClick: () => {
          activate(groupID)
          setFindRequest((previous) => ({ id, sequence: (previous?.sequence ?? 0) + 1 }))
        }
      },
      {
        key: 'split-right',
        icon: <SquareSplitHorizontal size={14} />,
        label: t('terminal:splitRight'),
        onClick: () => {
          activate(groupID)
          void split(groupID, id, 'vertical')
        }
      },
      {
        key: 'split-down',
        icon: <SquareSplitVertical size={14} />,
        label: t('terminal:splitDown'),
        onClick: () => {
          activate(groupID)
          void split(groupID, id, 'horizontal')
        }
      },
      {
        key: 'reconnect',
        icon: <RotateCcw size={14} />,
        label: session.kind === 'local' ? t('terminal:restartShell') : t('terminal:reconnect'),
        onClick: () => {
          activate(groupID)
          reconnect(groupID, id)
        }
      },
      ...(session.hostID
        ? [
            {
              key: 'details',
              icon: <Info size={14} />,
              label: t('terminal:hostConnectionDetails'),
              onClick: () => onOpenHost(session.hostID!)
            }
          ]
        : []),
      ...(paneIDs(group.root).length > 1
        ? [
            {
              key: 'close-pane',
              icon: <X size={14} />,
              label: t('terminal:closeTerminalPane'),
              onClick: () => closePane(groupID, id)
            }
          ]
        : []),
      { type: 'divider' }
    ]
  }
  const tabSessions = groups.map((group) => {
    const session = sessions[group.focused] ?? sessions[paneIDs(group.root)[0]]
    return {
      ...session,
      id: group.id,
      title: group.customTitle || session?.title || t('terminal:terminal'),
      localNumber: group.localNumber,
      customTitle: group.customTitle
    }
  })
  const palette = terminalTheme(theme)
  const pages = groups.map((group) => {
    const { panes, handles } = layoutPanes(group.root),
      visible =
        tabs.activeWorkspaceTab === `terminal:${group.id}` &&
        (connectionViews[group.id] === undefined ||
          connectionViews[group.id] === 'terminal' ||
          connectionViews[group.id] === 'split')
    const content = (
      <div
        key={group.id}
        className="terminal-workspace relative w-full h-full min-h-0 overflow-hidden"
        style={
          {
            display: visible ? 'block' : 'none',
            background: palette.background,
            '--terminal-bg': palette.background,
            '--terminal-fg': palette.foreground
          } as CSSProperties
        }
        ref={(element) => {
          if (element) groupNodes.current.set(group.id, element)
          else groupNodes.current.delete(group.id)
        }}
      >
        {panes.map(
          ({ id, box }) =>
            sessions[id] && (
              <TerminalPane
                key={id}
                session={sessions[id]}
                visible={visible}
                focused={group.focused === id}
                split={panes.length > 1}
                hideStatus={sessions[id].kind === 'ssh' || workspacePluginsAvailable}
                onClose={() => closePane(group.id, id)}
                findRequest={findRequest?.id === id ? findRequest.sequence : 0}
                inputBlocked={prompts.length > 0 || !!renaming}
                theme={theme}
                fontSize={fontSize}
                wordNavigation={wordNavigation}
                style={{
                  left: `${box.x}%`,
                  top: `${box.y}%`,
                  width: `${box.width}%`,
                  height: `${box.height}%`
                }}
                onFocus={() =>
                  updateGroups((previous) =>
                    previous.map((g) => (g.id === group.id ? { ...g, focused: id } : g))
                  )
                }
                onReconnect={() => reconnect(group.id, id)}
              />
            )
        )}
        {handles.map(({ node, box }) => (
          <div
            key={node.id}
            role="separator"
            tabIndex={0}
            aria-label={
              node.axis === 'vertical'
                ? t('terminal:resizeTerminalColumns')
                : t('terminal:resizeTerminalRows')
            }
            aria-orientation={node.axis === 'vertical' ? 'vertical' : 'horizontal'}
            aria-valuemin={15}
            aria-valuemax={85}
            aria-valuenow={Math.round(node.ratio * 100)}
            className={cn(
              'terminal-split-handle absolute z-5 touch-none',
              '[&.vertical]:cursor-col-resize [&.horizontal]:cursor-row-resize',
              `${node.axis}`
            )}
            style={
              node.axis === 'vertical'
                ? {
                    left: `${box.x + box.width * node.ratio}%`,
                    top: `${box.y}%`,
                    height: `${box.height}%`
                  }
                : {
                    top: `${box.y + box.height * node.ratio}%`,
                    left: `${box.x}%`,
                    width: `${box.width}%`
                  }
            }
            onKeyDown={(event) => {
              if (['ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown'].includes(event.key)) {
                event.preventDefault()
                updateGroups((previous) =>
                  previous.map((g) =>
                    g.id === group.id
                      ? {
                          ...g,
                          root: resizeSplit(
                            g.root,
                            node.id,
                            node.ratio +
                              (['ArrowLeft', 'ArrowUp'].includes(event.key) ? -0.05 : 0.05)
                          )
                        }
                      : g
                  )
                )
              }
            }}
            onPointerDown={(event) => {
              event.preventDefault()
              event.currentTarget.setPointerCapture(event.pointerId)
              event.currentTarget.dataset.dragging = 'true'
            }}
            onPointerMove={(event) => {
              if (!event.currentTarget.hasPointerCapture(event.pointerId)) return
              const bounds = groupNodes.current.get(group.id)?.getBoundingClientRect()
              if (!bounds) return
              const ratio =
                node.axis === 'vertical'
                  ? (((event.clientX - bounds.left) / bounds.width) * 100 - box.x) / box.width
                  : (((event.clientY - bounds.top) / bounds.height) * 100 - box.y) / box.height
              updateGroups((previous) =>
                previous.map((g) =>
                  g.id === group.id ? { ...g, root: resizeSplit(g.root, node.id, ratio) } : g
                )
              )
            }}
            onPointerUp={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
            onLostPointerCapture={(event) => {
              delete event.currentTarget.dataset.dragging
            }}
          />
        ))}
      </div>
    )
    const session = sessions[group.focused] ?? sessions[paneIDs(group.root)[0]]
    const workspace = (
      <TerminalPluginWorkspace key={group.id} session={session} visible={visible}>
        {content}
      </TerminalPluginWorkspace>
    )
    return session
      ? renderConnection(group.id, session, workspace, () => reconnect(group.id, group.focused))
      : workspace
  })
  const prompt = prompts[0]
  function renameTab(id: string) {
    const group = current.current.groups.find((g) => g.id === id)
    if (group?.localNumber)
      setRenaming({
        id,
        name:
          group.customTitle || current.current.sessions[group.focused]?.title || t('terminal:local')
      })
  }
  function resetTabName(id: string) {
    updateGroups((groups) =>
      groups.map((g) => (g.id === id ? { ...g, customTitle: undefined } : g))
    )
  }
  function saveTabName() {
    if (!renaming?.name.trim()) return
    const { id, name } = renaming
    updateGroups((groups) =>
      groups.map((g) => (g.id === id ? { ...g, customTitle: name.trim() } : g))
    )
    setRenaming(null)
  }
  const dialogs = (
    <>
      <WorkbenchDialog
        title={t('terminal:renameTab')}
        open={!!renaming}
        submitLabel={t('terminal:rename')}
        submitDisabled={!renaming?.name.trim()}
        onSubmit={saveTabName}
        onClose={() => setRenaming(null)}
        onOpened={() => document.getElementById('terminal-tab-name')?.focus()}
      >
        <label htmlFor="terminal-tab-name">{t('terminal:tabName')}</label>
        <TextField
          id="terminal-tab-name"
          autoFocus
          maxLength={80}
          value={renaming?.name ?? ''}
          onFocus={(event) => event.target.select()}
          onChange={(event) =>
            setRenaming((value) =>
              value ? { ...value, name: event.target.value.replace(/[\x00-\x1f\x7f]/g, '') } : null
            )
          }
          onEnter={saveTabName}
        />
      </WorkbenchDialog>
      {prompt && <TerminalPromptDialog key={prompt.id} prompt={prompt} />}
    </>
  )
  return {
    sessionForGroup(id: string): TerminalSession | undefined {
      const group = current.current.groups.find((group) => group.id === id)
      return group ? current.current.sessions[group.focused] : undefined
    },
    tabActions,
    renameTab,
    resetTabName,
    open,
    activate,
    closeGroup,
    closeActive,
    findActive,
    splitActive,
    pages,
    dialogs,
    tabSessions
  }
}
