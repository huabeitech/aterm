import { NativeWorkspace, PluginActions, pluginTitle } from '../features/plugins/NativeWorkspace'
import { usePluginCatalog } from '../features/plugins/pluginCatalog'

import type { FileTarget } from '../../../shared/contracts/files'
import { hasRemoteConnection, remoteFileTarget } from '../../../shared/contracts/terminal'
import { useThemeRevision, themePreferences, effectiveAppTheme } from './themeRegistry'
import { localizedError } from '../../../shared/i18n/errors'
import { t } from '../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useAppUI } from '@/components/ui/confirm-provider'
import { ActionButton, BusyIndicator } from '@/components/ui/workbench-controls'
import { useIntegration } from '../features/data/useIntegration'
import { useFilesWorkbench } from '../features/files/useFilesWorkbench'
import { useTerminalWorkbench } from '../features/terminal/useTerminalWorkbench'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'

import { desktop } from '../api/app'
import type { Bootstrap } from '../../../shared/contracts/app'
import { getCommands, type Command } from '../../../shared/contracts/commands'
import { validatePreferences, type ThemePreference } from '../../../shared/contracts/preferences'
import type { domain } from '../api/types'
import { themeOptions } from './themes'
import { AppProviders } from './AppProviders'
import { AppAppearance } from './AppAppearance'
import { AppShell } from '../components/layout/AppShell'
import { platformFeatures } from '../../../shared/platformFeatures'
import { SourceSidebar } from '../components/sidebar/SourceSidebar'
import { ConnectionWelcome } from '../components/sidebar/ConnectionWelcome'
import { SettingsPage } from '../components/settings/SettingsPage'
import type { PaletteCommand } from '../components/common/paletteSearch'
import { CommandPalette } from '../components/common/CommandPalette'
import {
  ConnectionDetailsTab,
  ConnectionWorkspace,
  type ConnectionView,
} from '../features/workspace/ConnectionWorkspace'
import { WorkspaceTabsBar } from '../features/workspace/WorkspaceTabsBar'
import { useConnectionSidebar } from '../features/workspace/useConnectionSidebar'
import { useWorkspaceTabs } from '../features/workspace/useWorkspaceTabs'
import { useHostWorkbench } from '../features/hosts/hooks/useHostWorkbench'

function focusActiveFilesFilter() {
  const workspace = document.querySelector('.file-workspace-container[data-active="true"]')
  workspace?.querySelector<HTMLInputElement>('[data-action="filter-files"]')?.focus()
}

export function App() {
  useTranslation()
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null)
  const [selectedTheme, setSelectedTheme] = useState<ThemePreference>('system')
  const [appearance, setAppearance] = useState<'light' | 'dark'>(
    window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
  )
  const [error, setError] = useState(false)
  const load = useCallback(() => {
    setError(false)
    desktop
      .bootstrap()
      .then((value) => {
        setBootstrap(value)
        setSelectedTheme(value.preferences.theme as ThemePreference)
        setAppearance(value.resolvedTheme)
      })
      .catch(() => setError(true))
  }, [])
  useEffect(load, [load])
  useEffect(() => desktop.onThemeChanged(setAppearance), [])
  return (
    <AppProviders>
      {bootstrap ? (
        <Workbench bootstrap={bootstrap} appearance={appearance} onThemeChange={setSelectedTheme} />
      ) : (
        <div className="startup-state h-screen grid place-content-center gap-4 bg-[var(--mac-sidebar)] text-[var(--mac-text)]">
          <AppAppearance theme={selectedTheme} appearance={appearance} />
          {error ? (
            <>
              <p>{t('common:unableToOpenYourWorkspace')}</p>
              <ActionButton onClick={load}>{t('common:retry')}</ActionButton>
            </>
          ) : (
            <BusyIndicator />
          )}
        </div>
      )}
    </AppProviders>
  )
}
function Workbench({
  bootstrap,
  appearance,
  onThemeChange,
}: {
  bootstrap: Bootstrap
  appearance: 'light' | 'dark'
  onThemeChange: (theme: ThemePreference) => void
}) {
  const { i18n } = useTranslation()
  const { message, notification } = useAppUI()
  const [settings, setSettings] = useState(bootstrap.preferences)
  const themeRevision = useThemeRevision()
  useEffect(() => {
    const next = themePreferences()
    if (next) {
      savedPreferences.current = next
      setSettings(next)
      onThemeChange(next.theme)
    }
  }, [themeRevision])
  useLayoutEffect(() => {
    // Set on the document so dialogs, menus and tooltips rendered in portals scale too.
    document.documentElement.style.setProperty(
      '--app-font-scale',
      String(settings.appFontScale / 100),
    )
    return () => {
      document.documentElement.style.removeProperty('--app-font-scale')
    }
  }, [settings.appFontScale])
  const [saving, setSaving] = useState(false)
  const preferenceSaveQueue = useRef(Promise.resolve())
  const preferenceSaveID = useRef(0)
  const savedPreferences = useRef(bootstrap.preferences)
  useEffect(
    () =>
      desktop.onLocaleChanged(({ preference }) => {
        savedPreferences.current = { ...savedPreferences.current, language: preference }
        setSettings((current) => ({ ...current, language: preference }))
      }),
    [],
  )
  const sidebar = useConnectionSidebar()
  const [searchOpen, setSearchOpen] = useState(false)
  const plugins = usePluginCatalog()
  const [pluginHostIDs, setPluginHostIDs] = useState<Record<string, string | undefined>>({})
  const [searchRequest, setSearchRequest] = useState({ query: '', serial: 0 })
  const searchOpenRef = useRef(false)
  const searchOrigin = useRef<HTMLElement | null>(null)
  const [settingsSectionRequest, setSettingsSectionRequest] = useState(0)
  const tabs = useWorkspaceTabs()
  const [connectionViews, setConnectionViews] = useState<Record<string, ConnectionView>>({})
  const cycleConnectionView = useRef<(() => void) | null>(null)
  const openingHostSessions = useRef(new Set<string>())
  const [connectingHostTabs, setConnectingHostTabs] = useState<ReadonlySet<string>>(new Set())
  const closingConnections = useRef(new Set<string>())
  const settingsOpen = tabs.workspaceTabs.some((tab) => tab.id === 'settings')
  const isSettings = tabs.activeWorkspaceTab === 'settings'
  const isPluginWorkspace = tabs.activeWorkspaceTab.startsWith('plugin:')
  function openPlugin(pluginID: string, viewID: string, hostID?: string) {
    const id = `plugin:${pluginID}:${viewID}` as const
    setPluginHostIDs((current) => ({ ...current, [id]: hostID }))
    tabs.openWorkspaceTab(id)
    tabs.setActiveWorkspaceTab(id)
  }

  const notifyUnavailable = useCallback(
    (feature: string) => {
      void message.info(t('common:featureUnavailable', { feature }))
    },
    [message],
  )
  const library = useHostWorkbench(
    (id) => void connectHost(id),
    (id) => void openFiles(id),
    (id) => showHostDetails(id),
    (id) => <PluginActions location="host.details" hostID={id} onOpen={openPlugin} />,
  )
  const files = useFilesWorkbench(
    library.unlockForConnection,
    library.refresh,
    Math.max(1, settings.appFontScale / 100),
    (id): string | undefined => terminal.sessionForGroup(id)?.id,
  )
  const terminal = useTerminalWorkbench(
    tabs,
    effectiveAppTheme(settings.theme, appearance),
    settings.terminalFontSize,
    settings.terminalWordNavigation,
    library.unlockForConnection,
    library.refresh,
    (id, groupID) => void openFiles(id, groupID),
    showHostDetails,
    connectionViews,
    (id, session, content, onReconnect) => {
      const fileSession = files.sessions.find((s) => s.ownerID === id)
      return (
        <ConnectionWorkspace
          key={id}
          cycleViewRef={cycleConnectionView}
          active={tabs.activeWorkspaceTab === `terminal:${id}`}
          view={connectionViews[id] ?? 'terminal'}
          session={session}
          fileSession={fileSession}
          transfers={files.transfers.filter((t) => t.sessionID === fileSession?.id)}
          terminal={content}
          details={
            session.hostID ? renderHostDetails(session.hostID, id) : undefined
          }
          unlock={library.unlockForConnection}
          onReconnectSSH={onReconnect}
          onReconnectSFTP={() => {
            if (fileSession) void files.reconnect(fileSession.id)
          }}
          renderFiles={(target, request) => files.renderPage(id, target, request)}
          onViewChange={(view) => {
            setConnectionViews((previous) => ({ ...previous, [id]: view }))
            if (view === 'files' || view === 'split') {
              const target = remoteFileTarget(session)
              if (target) void files.open(target, id)
            }
          }}
        />
      )
    },
  )
  useEffect(() => {
    setConnectionViews((previous) => {
      const next = { ...previous }
      let changed = false
      for (const session of terminal.tabSessions) {
        if (
          !hasRemoteConnection(session) &&
          (next[session.id] === 'files' || next[session.id] === 'split')
        ) {
          delete next[session.id]
          changed = true
        }
      }
      return changed ? next : previous
    })
  }, [terminal.tabSessions])
  const integration = useIntegration(library.unlockForData, async () => {
    library.refresh()
    const preferences = (await desktop.bootstrap()).preferences
    savedPreferences.current = preferences
    setSettings(preferences)
    onThemeChange(preferences.theme as ThemePreference)
  })
  const activeConnectionID = tabs.activeWorkspaceTab.startsWith('terminal:')
    ? tabs.activeWorkspaceTab.slice(9)
    : null
  const activeHostTab = tabs.workspaceTabs.find((tab) => tab.id === tabs.activeWorkspaceTab)
  const sessionGroupIDs = new Set(terminal.tabSessions.map((session) => session.id))
  const isDetails = !!activeConnectionID && (
    connectionViews[activeConnectionID] === 'details' ||
    (!!activeHostTab?.hostID && !sessionGroupIDs.has(activeConnectionID))
  )
  const isFiles = !isDetails && !!activeConnectionID && connectionViews[activeConnectionID] === 'files'
  const isConnectionPlugin =
    !!activeConnectionID && connectionViews[activeConnectionID]?.startsWith('plugin:')
  const isPluginView =
    !!activeConnectionID && connectionViews[activeConnectionID]?.startsWith('plugin:') === true
  const isTerminal =
    tabs.activeWorkspaceTab.startsWith('terminal:') &&
    !isDetails &&
    !isFiles &&
    !isConnectionPlugin &&
    !connectionViews[activeConnectionID ?? '']?.startsWith('plugin:')
  function recentHostTab(hostID: string) {
    return tabs.findRecentTab((tab) => {
      if (!tab.id.startsWith('terminal:')) return false
      const id = tab.id.slice(9)
      return !closingConnections.current.has(id) &&
        (tab.hostID ?? terminal.sessionForGroup(id)?.hostID) === hostID
    })
  }
  function selectConnectionView(id: string, view: ConnectionView) {
    tabs.setActiveWorkspaceTab(`terminal:${id}`)
    setConnectionViews((previous) =>
      previous[id] === view ? previous : { ...previous, [id]: view },
    )
  }
  function showHostDetails(hostID: string, groupID?: string) {
    const existing = groupID ? undefined : recentHostTab(hostID)
    const id = groupID ?? existing?.id.slice(9)
    if (id) {
      if (terminal.sessionForGroup(id) || openingHostSessions.current.has(id))
        selectConnectionView(id, 'details')
      else tabs.setActiveWorkspaceTab(`terminal:${id}`)
    } else {
      tabs.openHostTab(hostID, true)
    }
  }
  function renderHostDetails(hostID: string, groupID: string) {
    return library.renderHostDetails(hostID, {
      connect: () => void connectHost(hostID, { groupID }),
      openFiles: () => void openFiles(hostID, groupID),
      connecting: connectingHostTabs.has(groupID),
    })
  }
  async function openHostSession(hostID: string, id: string, view: 'terminal' | 'files') {
    if (openingHostSessions.current.has(id) || closingConnections.current.has(id)) return
    const tabID = `terminal:${id}` as const
    tabs.keepTabOpen(tabID)
    selectConnectionView(id, view)
    if (terminal.sessionForGroup(id)) {
      if (view === 'files') await files.open(hostID, id)
      return
    }
    openingHostSessions.current.add(id)
    setConnectingHostTabs(new Set(openingHostSessions.current))
    try {
      const opened = await terminal.open({ kind: 'ssh', hostID }, id)
      if (opened && view === 'files') await files.open(hostID, id)
    } finally {
      openingHostSessions.current.delete(id)
      setConnectingHostTabs(new Set(openingHostSessions.current))
    }
  }
  async function connectHost(
    hostID: string,
    { groupID, newTab = false }: { groupID?: string; newTab?: boolean } = {},
  ) {
    const existing = newTab || groupID ? undefined : recentHostTab(hostID)
    const existingID = existing?.id.slice(9)
    const id = groupID ?? (
      existingID && (!terminal.sessionForGroup(existingID) || openingHostSessions.current.has(existingID))
        ? existingID
        : tabs.openHostTab(hostID, false).id.slice(9)
    )
    await openHostSession(hostID, id, 'terminal')
  }
  async function openFiles(target: FileTarget, groupID?: string) {
    if (typeof target !== 'string') {
      if (!groupID || closingConnections.current.has(groupID)) return
      selectConnectionView(groupID, 'files')
      await files.open(target, groupID)
      return
    }
    const id = groupID ?? (recentHostTab(target) ?? tabs.openHostTab(target, false)).id.slice(9)
    await openHostSession(target, id, 'files')
  }
  async function closeConnection(id: string) {
    if (closingConnections.current.has(id)) return
    closingConnections.current.add(id)
    try {
      if (terminal.sessionForGroup(id)) {
        if (!(await files.close(id))) return
        terminal.closeGroup(id)
      } else {
        tabs.removeTabs([`terminal:${id}`])
      }
      setConnectionViews((previous) => {
        const next = { ...previous }
        delete next[id]
        return next
      })
    } finally {
      closingConnections.current.delete(id)
    }
  }
  const { activeView, setActiveView } = library
  const unavailable = () => notifyUnavailable(t('common:hostManagement'))
  const noticeShown = useRef(false)
  useEffect(() => {
    if (noticeShown.current) return
    noticeShown.current = true
    if (bootstrap.recoveryNotice)
      notification.warning({
        title: t('common:preferencesRecovered'),
        description: bootstrap.recoveryNotice,
      })
    if (bootstrap.info.localTerminalAvailable && settings.defaultHome === 'local-terminal')
      void terminal.open({ kind: 'local' })
  }, [bootstrap.recoveryNotice, notification, notifyUnavailable, settings.defaultHome])
  useEffect(() => {
    document.title = 'aTerm'
  }, [])
  function activateHosts() {
    if (isSettings) tabs.setActiveWorkspaceTab('home')
    sidebar.revealSearch()
  }
  function openSettings() {
    void integration.refresh()
    tabs.openWorkspaceTab('settings')
    tabs.setActiveWorkspaceTab('settings')
    library.prepareSettings()
  }
  function showSearch(query = '') {
    if (
      library.overlayOpen ||
      library.busy ||
      (!searchOpenRef.current && document.querySelector('[role="dialog"]'))
    )
      return
    if (!searchOpenRef.current) searchOrigin.current = document.activeElement as HTMLElement | null
    searchOpenRef.current = true
    setSearchRequest((previous) => ({ query, serial: previous.serial + 1 }))
    setSearchOpen(true)
  }
  function closeSearch(run?: () => void) {
    if (!searchOpenRef.current) return
    searchOpenRef.current = false
    setSearchOpen(false)
    desktop.setInputMode('normal')
    requestAnimationFrame(() => {
      if (searchOrigin.current?.isConnected) searchOrigin.current.focus()
      run?.()
    })
  }
  function closeSettings() {
    if (activeView === 'settings') setActiveView('all')
    tabs.removeTabs(['settings'])
  }
  function closeTabs(ids: (typeof tabs.activeWorkspaceTab)[]) {
    for (const id of ids) if (id.startsWith('terminal:')) void closeConnection(id.slice(9))
    tabs.removeTabs(ids.filter((id) => !id.startsWith('terminal:')))
    if (ids.includes('settings') && activeView === 'settings') setActiveView('all')
  }
  function adjacent(direction: -1 | 1) {
    const next = tabs.adjacentTab(direction)
    if (next === 'settings') openSettings()
    else if (next) tabs.setActiveWorkspaceTab(next)
  }
  const execute = (command: Command) => {
    switch (command) {
      case 'settings':
        openSettings()
        break
      case 'next-connection-view': {
        if (library.overlayOpen || library.busy || document.querySelector('[role="dialog"]')) break
        cycleConnectionView.current?.()
        break
      }
      case 'toggle-sidebar':
        if (!isSettings) sidebar.toggle()
        break
      case 'hosts':
        activateHosts()
        break
      case 'search':
        showSearch()
        break
      case 'command-palette':
        showSearch('>')
        break
      case 'find':
        if (
          isFiles ||
          (activeConnectionID &&
            connectionViews[activeConnectionID] === 'split' &&
            document.activeElement?.closest('.file-workspace-container'))
        )
          focusActiveFilesFilter()
        else if (isTerminal) terminal.findActive()
        else if (tabs.activeWorkspaceTab === 'home')
          requestAnimationFrame(() =>
            document.querySelector<HTMLInputElement>('[data-action="search-hosts"]')?.focus(),
          )
        break
      case 'new-terminal':
        if (!bootstrap.info.localTerminalAvailable) break
        void terminal.open({ kind: 'local' })
        break
      case 'close-tab':
        if (
          isTerminal &&
          terminal.tabSessions.find((s) => s.id === activeConnectionID)?.kind === 'local'
        )
          terminal.closeActive()
        else if (activeConnectionID) void closeConnection(activeConnectionID)
        else if (isPluginWorkspace) tabs.removeTabs([tabs.activeWorkspaceTab])
        else if (isSettings) closeSettings()
        break
      case 'previous-tab':
        adjacent(-1)
        break
      case 'next-tab':
        adjacent(1)
        break
      case 'move-left':
        tabs.moveActiveTab(-1)
        break
      case 'move-right':
        tabs.moveActiveTab(1)
        break
      case 'split-vertical':
        if (isTerminal) terminal.splitActive('vertical')
        break
      case 'split-horizontal':
        if (isTerminal) terminal.splitActive('horizontal')
        break
      default: {
        const tab = tabs.workspaceTabs[Number(command.slice(4)) - 1]
        if (tab?.id === 'settings') openSettings()
        else if (tab) tabs.setActiveWorkspaceTab(tab.id)
        break
      }
    }
  }
  const activeIndex = tabs.workspaceTabs.findIndex((tab) => tab.id === tabs.activeWorkspaceTab)
  const features = platformFeatures(bootstrap.info.platform)
  const paletteCommands: PaletteCommand[] = [
    ...plugins
      .filter((p) => p.enabled && p.manifest.runtime === 'native')
      .flatMap((p) =>
        (p.manifest.contributes.views ?? []).map((v) => ({
          id: `plugin:${p.manifest.id}:${v.id}`,
          label: pluginTitle(p, v.title, i18n.resolvedLanguage ?? i18n.language),
          run: () => openPlugin(p.manifest.id, v.id),
        })),
      ),
    {
      id: 'connect-ssh',
      label: t('commands:connectSsh'),
      keywords: '连接 主机 ssh',
      run: () => {},
    },
    {
      id: 'connect-sftp',
      label: t('commands:connectSftp'),
      keywords: '文件 传输 sftp',
      run: () => {},
    },
    { id: 'switch-tab', label: t('commands:switchTab'), keywords: '切换 标签页', run: () => {} },
    ...(!integration.busy && features.cloudBackup
      ? [
          {
            id: 'backup',
            label: t('commands:backupCommand'),
            keywords: '备份 backup icloud',
            run: () => integration.backup(),
          },
          {
            id: 'export-openssh',
            label: t('commands:exportCommand'),
            keywords: '导出 open ssh config 配置',
            run: () => {
              integration.exportSSH()
            },
          },
        ]
      : []),
    {
      id: 'change-theme',
      label: t('commands:themeCommand'),
      keywords: '切换 主题 外观 theme appearance',
      run: () => {},
      choices: {
        title: t('common:appTheme'),
        items: themeOptions().map(({ label, value }) => ({
          id: `theme:${value}`,
          label,
          detail: settings.theme === value ? t('common:current') : undefined,
          run: () => {
            void saveSettings({ ...settings, theme: value as ThemePreference }).catch(
              reportThemeError,
            )
          },
        })),
      },
    },
    ...getCommands(settings, bootstrap.info.platform)
      .filter(({ id }) => {
        if (id === 'search' || id === 'command-palette' || id.startsWith('tab-')) return false
        if (id === 'next-connection-view') return !!activeConnectionID
        if (id === 'new-terminal') return bootstrap.info.localTerminalAvailable
        if (id === 'split-vertical' || id === 'split-horizontal') return isTerminal
        if (id === 'close-tab') return tabs.activeWorkspaceTab !== 'home'
        if (id === 'previous-tab' || id === 'next-tab') return tabs.workspaceTabs.length > 1
        if (id === 'move-left') return activeIndex > 0
        if (id === 'move-right')
          return activeIndex >= 0 && activeIndex < tabs.workspaceTabs.length - 1
        if (id === 'find') return !isSettings
        return true
      })
      .map(({ id, label, englishLabel, shortcut }) => ({
        id,
        shortcut,
        label: `${id === 'settings' ? t('common:preferences') : ['new-terminal', 'split-vertical', 'split-horizontal'].includes(id) ? t('common:terminal') : t('common:workbench')}: ${label}`,
        keywords:
          englishLabel +
          ' ' +
          (
            {
              settings: '设置 preferences',
              hosts: '主机',
              'toggle-sidebar': '侧边栏 展开 收起 collapse expand sidebar',
              find: '搜索 查找',
              'new-terminal': '终端 本地 shell',
              'split-vertical': '分屏 right',
              'split-horizontal': '分屏 down',
              'close-tab': '关闭 标签页',
            } as Record<string, string>
          )[id],
        run: () => execute(id),
      })),
    {
      id: 'settings-shortcuts',
      label: t('commands:shortcutsCommand'),
      keywords: '快捷键 设置',
      run: () => {
        setSettingsSectionRequest((value) => value + 1)
        openSettings()
      },
    },
  ]
  function nativeTabTitle(id: string) {
    const [, pluginID, viewID] = id.split(':')
    const p = plugins.find((p) => p.manifest.id === pluginID)
    const v = p?.manifest.contributes.views?.find((v) => v.id === viewID)
    return p && v ? pluginTitle(p, v.title, i18n.resolvedLanguage ?? i18n.language) : pluginID
  }
  const paletteTabs: PaletteCommand[] = tabs.workspaceTabs.map((tab) => ({
    id: tab.id,
    label: tab.id.startsWith('plugin:')
      ? nativeTabTitle(tab.id)
      : tab.id === 'settings'
        ? t('common:settings')
        : tab.kind === 'files'
          ? `${files.sessions.find((session) => session.hostID === tab.hostID)?.title ?? t('common:files')} · SFTP`
          : (library.hosts.find((host) => host.id === tab.hostID)?.name ??
            terminal.tabSessions.find((session) => `terminal:${session.id}` === tab.id)?.title ??
            t('common:terminal')),
    run: () => {
      if (tab.id === 'settings') openSettings()
      else tabs.setActiveWorkspaceTab(tab.id)
    },
  }))
  const executeRef = useRef(execute)
  executeRef.current = execute
  useEffect(() => desktop.onCommand((command) => executeRef.current(command)), [])
  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => {
      if (
        (tabs.activeWorkspaceTab !== 'home' && !isTerminal && !isFiles) ||
        event.isComposing ||
        event.altKey ||
        event.shiftKey ||
        event.metaKey === event.ctrlKey ||
        event.key.toLowerCase() !== 'f'
      )
        return
      event.preventDefault()
      if (
        isFiles ||
        (activeConnectionID &&
          connectionViews[activeConnectionID] === 'split' &&
          document.activeElement?.closest('.file-workspace-container'))
      )
        focusActiveFilesFilter()
      else if (isTerminal) terminal.findActive()
      else document.querySelector<HTMLInputElement>('[data-action="search-hosts"]')?.focus()
    }
    window.addEventListener('keydown', focusSearch, true)
    return () => window.removeEventListener('keydown', focusSearch, true)
  }, [tabs.activeWorkspaceTab, isTerminal, isFiles, connectionViews])
  useEffect(() => {
    function updateInputMode() {
      const element = document.activeElement
      const editable =
        element instanceof HTMLElement &&
        (element.isContentEditable || element.matches('input,textarea,select'))
      desktop.setInputMode(
        element?.closest('.terminal-screen')
          ? 'terminal'
          : element?.closest('.shortcut-recorder')
            ? 'recorder'
            : editable && !element?.closest('[data-allow-global-shortcuts="true"]')
              ? 'editable'
              : 'normal',
      )
    }
    document.addEventListener('focusin', updateInputMode)
    const afterFocusOut = () => queueMicrotask(updateInputMode)
    document.addEventListener('focusout', afterFocusOut)
    return () => {
      document.removeEventListener('focusin', updateInputMode)
      document.removeEventListener('focusout', afterFocusOut)
      desktop.setInputMode('normal')
    }
  }, [])
  function reportThemeError(error: unknown) {
    void message.error(
      error instanceof Error ? localizedError(error) : t('common:unableToSaveTheme'),
    )
  }
  async function saveSettings(value: domain.AppSettings) {
    const next = validatePreferences(value)
    const id = ++preferenceSaveID.current
    setSettings(next)
    onThemeChange(next.theme as ThemePreference)
    setSaving(true)
    // Persist in order; an older response must not overwrite a newer edit.
    const request = preferenceSaveQueue.current.then(async () => {
      const saved = await desktop.savePreferences(next)
      savedPreferences.current = saved
      if (id === preferenceSaveID.current) setSettings(saved)
    })
    preferenceSaveQueue.current = request.catch(() => {})
    try {
      await request
    } catch (error) {
      if (id === preferenceSaveID.current) {
        setSettings(savedPreferences.current)
        onThemeChange(savedPreferences.current.theme as ThemePreference)
      }
      throw error
    } finally {
      if (id === preferenceSaveID.current) setSaving(false)
    }
  }
  const status: domain.AppStatus = integration.status?.app ?? {
    databasePath: '',
    macAppStoreBuild: bootstrap.info.macAppStoreBuild,
    connectionFeaturesAvailable: true,
    localTerminalAvailable: bootstrap.info.localTerminalAvailable,
    sshDirectoryPath: '',
    sshDirectoryAuthorized: false,
    sshDirectoryAuthorizationRequired: false,
    sshDirectoryAuthorizationErrorText: '',
    hostCount: library.hosts.length,
  }
  return (
    <>
      <AppAppearance theme={settings.theme as ThemePreference} appearance={appearance} />
      <AppShell
        platform={bootstrap.info.platform}
        style={{}}
        sidebarWidth={sidebar.preferredWidth}
        sidebarCollapsed={sidebar.collapsed}
        onToggleSidebar={sidebar.toggle}
        toggleSidebarShortcut={settings.toggleSidebarShortcut}
        mode={
          isSettings
            ? 'settings'
            : isFiles
              ? 'files'
              : isTerminal || isPluginView || isConnectionPlugin
                ? 'terminal'
                : 'home'
        }
        onSidebarWidthCommit={sidebar.commitWidth}
        header={
          <WorkspaceTabsBar
            tabs={tabs.workspaceTabs}
            activeTab={tabs.activeWorkspaceTab}
            hosts={library.hosts}
            sessions={terminal.tabSessions}
            fileSessions={files.sessions}
            draggingTab={tabs.draggingWorkspaceTabID}
            dropTarget={tabs.workspaceTabDropTarget}
            scrollState={tabs.tabScrollState}
            containerRef={tabs.workspaceTabsRef}
            commandPaletteShortcut={settings.commandPaletteShortcut}
            newTerminalShortcut={settings.newTerminalShortcut}
            localTerminalAvailable={bootstrap.info.localTerminalAvailable}
            onOpenSettings={openSettings}
            onCloseSettings={closeSettings}
            onActivateTab={tabs.setActiveWorkspaceTab}
            onKeepTabOpen={tabs.keepTabOpen}
            onActivateTerminal={terminal.activate}
            onRenameTerminal={terminal.renameTab}
            terminalActions={terminal.tabActions}
            onResetTerminalName={terminal.resetTabName}
            onCloseTerminal={(id) => void closeConnection(id)}
            onCloseFiles={(id) => closeTabs([id])}
            onOpenFiles={(host) => void openFiles(host.id)}
            onOpenFilesSplit={unavailable}
            onCopyCommand={unavailable}
            onReconnect={unavailable}
            onCloseAll={() => closeTabs(tabs.closableTabs())}
            onCloseOther={(id) => closeTabs(tabs.closableTabs().filter((tab) => tab !== id))}
            onCloseToRight={(id) => closeTabs(tabs.tabsToRight(id))}
            onDragStart={tabs.startTabDrag}
            onDragOver={tabs.previewTabDrop}
            onDrop={tabs.dropTab}
            onDragEnd={tabs.finishTabDrag}
            onWheel={tabs.scrollWorkspaceTabsWithWheel}
            onScroll={tabs.scrollWorkspaceTabs}
            onOpenLocalTerminal={() => void terminal.open({ kind: 'local' })}
            onOpenSearch={() => showSearch()}
          />
        }
        sidebar={
          <SourceSidebar
            {...library.sidebarProps}
            platform={bootstrap.info.platform}
            onConnectHostInNewTab={(id) => void connectHost(id, { newTab: true })}
            currentHostID={
              activeHostTab?.hostID ??
              (activeConnectionID ? terminal.sessionForGroup(activeConnectionID)?.hostID : undefined)
            }
            hoverPreviewEnabled={!isSettings && !sidebar.collapsed}
            activeView={isSettings ? 'settings' : library.activeView}
            syncStatus={integration.status?.sync ?? null}
            quickBackupPhase={null}
            pluginActions={<PluginActions location="sidebar" onOpen={openPlugin} />}
            onOpenSettings={openSettings}
            onBackUpToICloud={() => integration.backup()}
            onExportOpenSSHConfig={integration.exportSSH}
          />
        }
        content={
          <>
            {terminal.pages}
            {activeConnectionID && activeHostTab?.hostID && !sessionGroupIDs.has(activeConnectionID) && (
              <section
                className="connection-workspace flex"
                aria-label={t('hosts:hostDetails')}
              >
                <div className="connection-content">
                  {renderHostDetails(activeHostTab.hostID, activeConnectionID)}
                </div>
                <footer className="connection-statusbar">
                  <div className="connection-views" role="group" aria-label={t('terminal:connectionView')}>
                    <ConnectionDetailsTab active />
                  </div>
                </footer>
              </section>
            )}
            {tabs.workspaceTabs
              .filter((tab) => tab.kind === 'plugin')
              .map((tab) => {
                const [, pluginID, viewID] = tab.id.split(':')
                const plugin = plugins.find((p) => p.manifest.id === pluginID && p.enabled)
                const view = plugin?.manifest.contributes.views?.find((v) => v.id === viewID)
                return plugin && view ? (
                  <div
                    key={tab.id}
                    style={{ display: tabs.activeWorkspaceTab === tab.id ? 'contents' : 'none' }}
                  >
                    <NativeWorkspace
                      plugin={plugin}
                      view={view}
                      hostID={pluginHostIDs[tab.id]}
                      active={tabs.activeWorkspaceTab === tab.id}
                      unlock={library.unlockForConnection}
                      onClose={() => tabs.removeTabs([tab.id])}
                    />
                  </div>
                ) : null
              })}
            {settingsOpen && (
              <div style={{ display: isSettings ? 'contents' : 'none' }}>
                <SettingsPage
                  platform={bootstrap.info.platform}
                  onExportBackup={() => integration.backup(true)}
                  onRestoreBackup={() => integration.restore(true)}
                  shortcutSectionRequest={settingsSectionRequest}
                  settings={settings}
                  version={bootstrap.info.version}
                  status={status}
                  openSSHExportStatus={integration.status?.openssh ?? null}
                  syncStatus={integration.status?.sync ?? null}
                  busyAction={saving ? 'settings' : integration.busy ? 'sync' : null}
                  onChange={saveSettings}
                  onExportOpenSSHConfig={integration.exportSSH}
                  onAuthorizeSSHDirectory={() => integration.choose('ssh')}
                  onAuthorizeSyncDirectory={() => integration.choose('icloud')}
                  onUploadSync={() => integration.backup()}
                  onDownloadSync={() => integration.restore()}
                  onAutoUnlockAction={() => integration.autoUnlock()}
                  onChangeEncryptionPassword={integration.changePassword}
                  onDeleteICloudBackup={integration.deleteBackup}
                />
              </div>
            )}
            <div
              className="workspace-welcome-pane h-full min-h-0 grid grid-rows-[minmax(0,_1fr)]"
              style={{
                display: tabs.activeWorkspaceTab === 'home' ? 'grid' : 'none',
              }}
              aria-hidden={tabs.activeWorkspaceTab !== 'home'}
            >
              <ConnectionWelcome
                hosts={library.hosts}
                searchShortcut={settings.commandPaletteShortcut}
                onConnect={library.sidebarProps.onConnectHost}
                onTogglePinned={library.sidebarProps.onToggleFavoriteHost}
                pinBusy={!!library.sidebarProps.busyAction}
                onCreate={() => library.sidebarProps.onCreateHost('')}
                onSearch={() => showSearch()}
                onLocalTerminal={
                  bootstrap.info.localTerminalAvailable
                    ? () => void terminal.open({ kind: 'local' })
                    : undefined
                }
              />
            </div>
          </>
        }
      />
      {library.overlays}
      {terminal.dialogs}
      {searchOpen && (
        <CommandPalette
          open
          request={searchRequest}
          commands={paletteCommands}
          workspaceTabs={paletteTabs}
          onExecute={(run) => closeSearch(run)}
          hosts={library.hosts}
          busy={library.busy}
          connectionFeaturesAvailable
          fileFeaturesAvailable
          onClose={() => closeSearch()}
          onSelectHost={(id) => {
            void connectHost(id)
          }}
          onOpenFiles={(id) => {
            void openFiles(id)
          }}
        />
      )}
    </>
  )
}
