import { PluginView } from './PluginView'
import { usePluginViewController } from './usePluginViewController'
import { isViewToggleSelected } from './viewActions'
import { usePluginSession } from './usePluginSession'
import { pluginPresentation, pluginPresentationKeys } from './presentation'
import './plugins.css'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { t, i18n } from '../../../../shared/i18n'
import { pluginDisplayName } from '../../../../shared/plugins/localization'
import type { PluginInfo } from '../../../../shared/contracts/plugins'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import { desktop } from '../../api/app'
import { ActionButton, ActionMenu } from '../../components/ui/workbench-controls'
import { MoreHorizontal, ChevronDown, ChevronRight } from 'lucide-react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../components/ui/dialog'
import { SelectField } from '../../components/ui/workbench-controls'

export function PluginStatusBar({
  plugin,
  session,
  visible,
  panel
}: {
  plugin: PluginInfo
  session: TerminalSession
  visible: boolean
  panel: HTMLElement | null
}) {
  useTranslation()
  const { view, close, execute } = usePluginViewController(plugin.manifest.id, session.id)
  const [collapsed, setCollapsed] = useState(false)
  const [controls, setControls] = useState<Record<string, { paused: boolean; hidden: boolean }>>({})
  const { paused, hidden } = controls[session.id] ?? { paused: false, hidden: false }
  const updateControl = (
    key: 'paused' | 'hidden',
    value: boolean | ((before: boolean) => boolean)
  ) => {
    setControls((all) => {
      const before = all[session.id] ?? { paused: false, hidden: false }
      return {
        ...all,
        [session.id]: { ...before, [key]: typeof value === 'function' ? value(before[key]) : value }
      }
    })
  }
  const setPaused = (value: boolean | ((before: boolean) => boolean)) =>
    updateControl('paused', value)
  const setHidden = (value: boolean) => updateControl('hidden', value)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    const show = (event: Event) => {
      if ((event as CustomEvent<string>).detail === session.id) {
        setHidden(false)
        setPaused(false)
      }
    }
    window.addEventListener('aterm:show-plugin-bars', show)
    return () => window.removeEventListener('aterm:show-plugin-bars', show)
  }, [session.id])
  const running = session.state === 'running'
  const id = plugin.manifest.id
  const name = pluginDisplayName(
    plugin.manifest,
    plugin.localizations,
    i18n.resolvedLanguage ?? i18n.language
  )
  const { status, error: sessionError } = usePluginSession(
    id,
    session.id,
    visible && running && !paused && !hidden && plugin.state !== 'failed'
  )
  useEffect(() => {
    close()
  }, [
    session.id,
    plugin.manifest.version,
    visible,
    hidden,
    plugin.state === 'failed',
    status?.state === 'permission-required',
    close
  ])
  const presentation = pluginPresentation(session.state, plugin.state, status?.state, paused)
  const loading = presentation === 'loading'
  const failure = error || sessionError
  const state = failure || t(pluginPresentationKeys[presentation])
  const tooltip = (item: { id: string; detail?: string }) => {
    const detail = item.detail
    return `${name}\n${detail || ''}\n${status?.updatedAt ? new Date(status.updatedAt).toLocaleString() : state}`
  }
  async function action(key: string) {
    setError('')
    try {
      if (key.startsWith('item:')) {
        const item = status?.items.find((i) => i.id === key.slice(5))
        if (item?.command) await execute(item.command)
      }
      if (key === 'refresh') await desktop.plugins.refresh(id, session.id)
      if (key === 'pause') setPaused((value) => !value)
      if (key === 'hide') setHidden(true)
      if (key === 'settings') setSettingsOpen(true)
    } catch (err) {
      setError(String(err))
    }
  }
  if (hidden) return null
  return (
    <>
      <div
        className="plugin-status-bar"
        data-collapsed={collapsed}
        data-plugin-session={session.id}
        data-testid="terminal-plugin-status"
        aria-label={name}
      >
        <button
          className="plugin-status-toggle"
          aria-label={name}
          aria-expanded={!collapsed}
          onClick={() => setCollapsed(!collapsed)}
        >
          {collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
        </button>
        <div className="plugin-status-items" tabIndex={0} aria-label={name} title={name}>
          {[...(status?.items ?? [])]
            .sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0))
            .map((item) => (
              <button
                key={item.id}
                className="plugin-status-item"
                data-item-id={item.id}
                data-level={item.level}
                data-secondary={(item.priority ?? 0) < 0}
                data-visibility={item.visibility}
                title={tooltip(item)}
                disabled={!item.command}
                aria-pressed={isViewToggleSelected(
                  plugin.manifest.contributes.commands?.find(
                    (command) => command.id === item.command
                  ),
                  view
                )}
                onClick={() => {
                  setError('')
                  if (item.command) void execute(item.command).catch((err) => setError(String(err)))
                }}
              >
                <span className="plugin-status-label">{item.label}</span>
                <span className="plugin-status-value">{item.value}</span>
                <span className="plugin-status-compact">{item.compactValue ?? item.value}</span>
                {!collapsed && item.trend && (
                  <StatusTrend values={item.trend.values} ceiling={item.trend.ceiling} />
                )}
              </button>
            ))}
          {(failure || presentation !== 'ready') && (
            <span
              className="plugin-status-state"
              aria-busy={loading && !failure}
              title={error || sessionError || plugin.error || state}
              aria-label={state}
            >
              <span>{state}</span>
            </span>
          )}
        </div>
        {running && status?.state === 'permission-required' && (
          <ActionButton
            size="sm"
            variant="ghost"
            disabled={paused}
            onClick={() =>
              void desktop.plugins.authorize(id, session.id).catch((err) => setError(String(err)))
            }
          >
            {t('settings:pluginsAllowHost')}
          </ActionButton>
        )}
        <ActionMenu
          actions={[
            ...(status?.items ?? []).map((item) => ({
              key: 'item:' + item.id,
              label: `${item.label} ${item.value}`.trim(),
              disabled: !item.command
            })),
            {
              key: 'refresh',
              label: t('settings:pluginsRefresh'),
              disabled: !running || paused || status?.state === 'unsupported'
            },
            {
              key: 'pause',
              label: paused ? t('settings:pluginsResume') : t('settings:pluginsPause'),
              disabled: !running
            },
            { key: 'settings', label: t('settings:pluginsSettings') },
            { key: 'hide', label: t('settings:pluginsHide') }
          ]}
          onAction={({ key }) => void action(key)}
        >
          <ActionButton
            variant="ghost"
            size="icon-xs"
            aria-label={t('settings:pluginsActions')}
            icon={<MoreHorizontal size={14} />}
          />
        </ActionMenu>
      </div>
      {view && panel && visible && (
        <PluginView
          key={`${session.id}:${view.id}`}
          plugin={plugin}
          session={session}
          status={status && (!running || paused) ? { ...status, state: 'delayed' } : status}
          view={view}
          panel={panel}
          onClose={close}
          onCommand={execute}
        />
      )}
      <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{name}</DialogTitle>
          </DialogHeader>
          <label className="flex items-center justify-between gap-4 text-sm">
            {t('settings:pluginsInterval')}
            <SelectField
              aria-label={t('settings:pluginsInterval')}
              value={String(plugin.interval)}
              options={[5, 10, 30].map((n) => ({
                value: String(n),
                label: t('settings:pluginsSeconds', { count: n })
              }))}
              onValueChange={(value) => {
                void desktop.plugins
                  .configure(id, true, Number(value))
                  .catch((err) => setError(String(err)))
              }}
            />
          </label>
          <p className="text-sm text-muted-foreground">{name}</p>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}

function StatusTrend({ values, ceiling }: { values: (number | null)[]; ceiling?: number }) {
  const max = ceiling ?? Math.max(1, ...values.map((v) => v ?? 0))
  const segments: string[][] = [[]]
  values.forEach((v, index) => {
    if (v === null) {
      if (segments.at(-1)!.length) segments.push([])
    } else
      segments
        .at(-1)!
        .push(`${(index / Math.max(1, values.length - 1)) * 60},${19 - Math.min(1, v / max) * 18}`)
  })
  return (
    <svg className="plugin-status-trend" viewBox="0 0 60 20" aria-hidden="true">
      {segments
        .filter((s) => s.length)
        .map((s, i) => (
          <polyline
            key={i}
            points={s.join(' ')}
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          />
        ))}
    </svg>
  )
}
