import { validateExecutionPolicy } from './policy'
import type { PluginManifest } from '../../plugin-sdk'
import { jsonSnapshot } from './json'

export function validateManifest(input: unknown): PluginManifest {
  if (!input || typeof input !== 'object') throw Error('Invalid plugin manifest')
  const m = input as PluginManifest
  if (
    Object.keys(m).some(
      (key) =>
        ![
          'schemaVersion',
          'id',
          'name',
          'version',
          'runtime',
          'entry',
          'permissions',
          'contributes',
          'i18n',
          'engines',
          'execution',
          'backend',
        ].includes(key),
    ) ||
    m.schemaVersion !== 1 ||
    !['sandbox', 'native'].includes(m.runtime) ||
    !/^[a-z][a-z0-9-]*\.[a-z][a-z0-9.-]*$/.test(m.id) ||
    m.id.length > 100 ||
    typeof m.name !== 'string' ||
    !m.name.trim() ||
    m.name.length > 100 ||
    !/^\d+\.\d+\.\d+$/.test(m.version) ||
    (m.runtime === 'sandbox' && m.entry !== 'plugin.js') ||
    !Array.isArray(m.permissions) ||
    m.permissions.length > 6 ||
    new Set(m.permissions).size !== m.permissions.length ||
    m.permissions.some(
      (permission) =>
        ![
          'process.execute',
          'terminal.create',
          ...(m.runtime === 'native'
            ? ['native.execute', 'ssh.connect', 'hosts.read', 'terminal.open']
            : []),
        ].includes(permission),
    ) ||
    !m.contributes ||
    Object.keys(m.contributes).some(
      (key) => !['terminalStatusBar', 'commands', 'views'].includes(key),
    ) ||
    (m.contributes.terminalStatusBar !== undefined &&
      (m.contributes.terminalStatusBar?.id !== `${m.id}.status` ||
        Object.keys(m.contributes.terminalStatusBar).some((key) => key !== 'id')))
  )
    throw Error('Unsupported plugin manifest')
  // Normalize only public fields; reject undeclared executable contributions.
  const normalized: PluginManifest = {
    schemaVersion: 1,
    id: m.id,
    name: m.name,
    version: m.version,
    runtime: m.runtime,
    entry: m.entry,
    permissions: [...m.permissions].sort(),
    contributes: m.contributes.terminalStatusBar
      ? { terminalStatusBar: { id: `${m.id}.status` } }
      : {},
  }
  if (m.runtime === 'native') {
    if (
      !m.backend ||
      !/^(darwin|linux|windows)-(amd64|arm64)$/.test(m.backend.platform) ||
      !/^[a-f0-9]{64}$/.test(m.backend.sha256) ||
      m.entry !==
        `backend/${m.backend.platform}/plugin${m.backend.platform.startsWith('windows-') ? '.exe' : ''}` ||
      !m.permissions.includes('native.execute') ||
      Object.keys(m.backend).some((k) => !['platform', 'sha256'].includes(k))
    )
      throw Error('Invalid native backend')
    normalized.backend = { ...m.backend }
  } else if (m.backend !== undefined) throw Error('Backend requires native runtime')
  const executes = m.permissions.some((p) => p === 'process.execute' || p === 'terminal.create')
  if (executes !== (m.execution !== undefined))
    throw Error('Execution permissions require an execution policy')
  if (m.execution !== undefined) normalized.execution = validateExecutionPolicy(m.execution)
  if (m.i18n !== undefined) {
    const config = m.i18n
    if (
      !config ||
      typeof config.defaultLocale !== 'string' ||
      !config.locales ||
      typeof config.locales !== 'object' ||
      Array.isArray(config.locales)
    )
      throw Error('Invalid plugin localization manifest')
    const entries = Object.entries(config.locales).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    if (
      !entries.length ||
      entries.length > 16 ||
      !Object.hasOwn(config.locales, config.defaultLocale)
    )
      throw Error('Invalid plugin default locale')
    for (const [locale, path] of entries)
      if (
        !/^[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$/.test(locale) ||
        locale.length > 32 ||
        path !== `locales/${locale}.json`
      )
        throw Error('Invalid plugin locale path')
    for (const [locale] of entries)
      if (Intl.getCanonicalLocales(locale)[0] !== locale)
        throw Error('Plugin locale tags must be canonical')
    normalized.i18n = { defaultLocale: config.defaultLocale, locales: Object.fromEntries(entries) }
  }
  if (m.engines !== undefined) {
    if (
      !m.engines ||
      m.engines.hostApi !== 1 ||
      Object.keys(m.engines).some((k) => k !== 'hostApi')
    )
      throw Error('Incompatible plugin Host API')
    normalized.engines = { hostApi: 1 }
  }
  const validId = (id: unknown): id is string =>
    typeof id === 'string' && /^[a-z][a-zA-Z0-9.-]{0,99}$/.test(id)
  const validTitle = (title: unknown): title is string =>
    typeof title === 'string' && !!title.trim() && title.length <= 150
  if (m.contributes.views !== undefined) {
    if (!Array.isArray(m.contributes.views) || m.contributes.views.length > 8)
      throw Error('Invalid views')
    const ids = new Set<string>()
    normalized.contributes.views = m.contributes.views.map((view) => {
      if (
        !view ||
        !validId(view.id) ||
        ids.has(view.id) ||
        !validTitle(view.title) ||
        ![
          'terminal.sidePanel',
          'terminal.workspace',
          ...(m.runtime === 'native' ? ['app.workspace'] : []),
        ].includes(view.placement) ||
        !/^ui\/[a-zA-Z0-9_-]+\.html$/.test(view.entry) ||
        (view.availability !== undefined &&
          (view.availability !== 'dynamic' || view.placement !== 'terminal.workspace')) ||
        Object.keys(view).some(
          (k) => !['id', 'title', 'placement', 'entry', 'availability', 'locations'].includes(k),
        )
      )
        throw Error('Invalid view')
      if (
        view.locations !== undefined &&
        (m.runtime !== 'native' ||
          !Array.isArray(view.locations) ||
          view.locations.some((l) => !['sidebar', 'host.details', 'connection'].includes(l)))
      )
        throw Error('Invalid view location')
      ids.add(view.id)
      return {
        id: view.id,
        title: view.title,
        placement: view.placement,
        entry: view.entry,
        ...(view.locations ? { locations: [...view.locations] } : {}),
        ...(view.availability ? { availability: view.availability } : {}),
      }
    })
  }
  if (m.contributes.commands !== undefined) {
    if (!Array.isArray(m.contributes.commands) || m.contributes.commands.length > 32)
      throw Error('Invalid commands')
    const ids = new Set<string>()
    normalized.contributes.commands = m.contributes.commands.map((command) => {
      if (
        !command ||
        !validId(command.id) ||
        ids.has(command.id) ||
        !validTitle(command.title) ||
        Object.keys(command).some((k) => !['id', 'title', 'action', 'requiresUnlock'].includes(k))
      )
        throw Error('Invalid command')
      if (command.requiresUnlock !== undefined && typeof command.requiresUnlock !== 'boolean')
        throw Error('Invalid unlock requirement')
      ids.add(command.id)
      if (command.action !== undefined) {
        const action = command.action
        if (
          !action ||
          !['view.open', 'view.toggle', 'view.close'].includes(action.type) ||
          !normalized.contributes.views?.some(
            (v) => v.id === action.view && v.placement === 'terminal.sidePanel',
          ) ||
          Object.keys(action).some((k) => !['type', 'view', 'context'].includes(k))
        )
          throw Error('Invalid command action')
        return {
          id: command.id,
          title: command.title,
          ...(command.requiresUnlock ? { requiresUnlock: true } : {}),
          action: {
            type: action.type,
            view: action.view,
            ...(action.context === undefined
              ? {}
              : { context: jsonSnapshot(action.context, 4096) }),
          },
        }
      }
      return {
        id: command.id,
        title: command.title,
        ...(command.requiresUnlock ? { requiresUnlock: true } : {}),
      }
    })
  }
  if (
    !normalized.contributes.terminalStatusBar &&
    !normalized.contributes.views?.some(
      (v) => v.placement === 'terminal.workspace' || v.placement === 'app.workspace',
    )
  )
    throw Error('Plugin must contribute a status bar or workspace view')
  return normalized
}
