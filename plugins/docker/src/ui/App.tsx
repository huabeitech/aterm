import { useDetailResize } from './useDetailResize'
import { useImages } from './useImages'
import { ImagesPane } from './Images'
import { memo, useMemo, useEffect, useRef, useState, type CSSProperties } from 'react'
import { AlertDialog, Tabs } from 'radix-ui'
import {
  AlertCircle,
  Box,
  ChevronDown,
  Folder,
  Info,
  Layers,
  MoreHorizontal,
  Network,
  Pause,
  Play,
  PlugZap,
  RefreshCw,
  RotateCcw,
  Search,
  Server,
  Square,
  Terminal,
  Trash2,
  X
} from 'lucide-react'
import {
  Button,
  DropdownMenu,
  Menu,
  MenuItem,
  Popover,
  Section,
  StatusFilter,
  ScrollArea,
  CopyButton,
  ListSkeleton
} from './components'
import { translator, type Translate, type MessageKey } from './i18n'
import { useDocker, type Container, type ContainerDetail, type Operation } from './useDocker'

type Docker = ReturnType<typeof useDocker>
const stateLabels: Record<string, MessageKey> = {
  running: 'running',
  exited: 'exited',
  created: 'createdState',
  paused: 'paused',
  restarting: 'restarting',
  dead: 'dead',
  removing: 'removing',
  healthy: 'healthy',
  unhealthy: 'unhealthy',
  starting: 'starting'
}
function stateName(value: string, t: Translate) {
  return stateLabels[value] ? t(stateLabels[value]) : value
}
function Status({ value, t }: { value: string; t: Translate }) {
  return (
    <span className="status caption-5" data-state={value}>
      <i />
      {stateName(value, t)}
    </span>
  )
}
function Environment({ model, session, t }: { model: Docker; session: string; t: Translate }) {
  const e = model.engine
  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <Button
          className="environment-trigger"
          aria-label={t('environment')}
          title={`${session} · ${e?.context ?? t('connecting')}`}
        >
          <Server />
          <span className="environment-host title-5">{session}</span>
          {e && (
            <>
              <span className="environment-divider">/</span>
              <span className="environment-context caption-5">{e.context}</span>
            </>
          )}
          <ChevronDown className="chevron" />
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          aria-label={t('environment')}
          className="environment-popover"
          sideOffset={8}
          align="start"
        >
          <h2 className="title-4">{t('environment')}</h2>
          <p className="caption-5 muted">{t('engineHelp')}</p>
          <dl id="engine" className="environment-properties caption-5">
            {[
              [t('connection'), session],
              [t('engine'), e?.name],
              [t('context'), e?.context],
              [t('endpoint'), e?.endpoint],
              [t('version'), e?.version]
            ].map(([key, value]) => (
              <div key={key}>
                <dt>{key}</dt>
                <dd>{value || '—'}</dd>
              </div>
            ))}
          </dl>
          <Popover.Close asChild>
            <Button
              variant="outline"
              disabled={model.busy || !model.active}
              onClick={() => void model.refresh(true)}
            >
              <PlugZap />
              {t('reconnect')}
            </Button>
          </Popover.Close>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
const ContainerList = memo(function ContainerList({
  model,
  rows,
  query,
  t
}: {
  model: Pick<Docker, 'ready' | 'busy' | 'containers' | 'selected' | 'refresh' | 'choose'>
  rows: Container[]
  query: string
  t: Translate
}) {
  if (!model.ready)
    return (
      <div className="list-empty">
        {model.busy ? (
          <ListSkeleton label={t('loading')} />
        ) : (
          <>
            <Box />
            <h2 className="title-4">{t('unavailable')}</h2>
            <Button variant="outline" onClick={() => void model.refresh(true)}>
              {t('retry')}
            </Button>
          </>
        )}
      </div>
    )
  if (!rows.length)
    return (
      <div className="list-empty">
        <Search />
        <h2 className="title-4">{t(model.containers.length || query ? 'noMatch' : 'empty')}</h2>
        <p className="body-6 muted">
          {t(model.containers.length || query ? 'noMatchHelp' : 'emptyHelp')}
        </p>
      </div>
    )
  return (
    <table>
      <thead>
        <tr>
          {(['name', 'image', 'status', 'ports'] as const).map((key) => (
            <th scope="col" className={`${key}-column caption-5`} key={key}>
              {t(key === 'name' ? 'containerName' : key)}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((c) => (
          <tr
            key={c.ID}
            tabIndex={0}
            aria-selected={model.selected?.ID === c.ID}
            onClick={() => model.choose(c.ID)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                model.choose(c.ID)
              }
            }}
          >
            <td className="name-column">
              <div className="container-name title-5">
                <Box />
                <span>{c.Names}</span>
              </div>
              <div className="container-id caption-5 muted">
                {model.selected ? c.Image : c.ID.slice(0, 12)}
              </div>
            </td>
            <td className="image-column caption-5" title={c.Image}>
              {c.Image}
            </td>
            <td className="status-column">
              <Status value={c.State} t={t} />
              <span className="container-status caption-5 muted" title={c.Status}>
                {c.Status}
              </span>
            </td>
            <td className="ports-column caption-5" title={c.Ports}>
              {c.Ports || '—'}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
})
function Properties({
  detail,
  container,
  locale,
  t
}: {
  detail: ContainerDetail
  container: Container
  locale: string
  t: Translate
}) {
  const date =
    detail.Created && !Number.isNaN(Date.parse(detail.Created))
      ? new Date(detail.Created).toLocaleString(locale, {
          year: 'numeric',
          month: 'short',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit'
        })
      : '—'
  const ports = Object.entries(detail.Ports ?? {}).flatMap(([port, bindings]) =>
    (bindings ?? []).map((binding) => ({
      port,
      host: `${binding.HostIp || '*'}:${binding.HostPort}`
    }))
  )
  return (
    <>
      <Section title={t('summary')}>
        <dl className="properties caption-5">
          <div>
            <dt>{t('id')}</dt>
            <dd className="with-copy">
              <code title={detail.Id}>{detail.Id?.slice(0, 12)}</code>
              <CopyButton value={detail.Id} label={t('copyId')} />
            </dd>
          </div>
          <div>
            <dt>{t('state')}</dt>
            <dd>
              <Status value={container.State} t={t} />
            </dd>
          </div>
          {detail.State?.Health?.Status && (
            <div>
              <dt>{t('health')}</dt>
              <dd>
                <Status value={detail.State.Health.Status} t={t} />
              </dd>
            </div>
          )}
          <div>
            <dt>{t('created')}</dt>
            <dd title={detail.Created}>{date}</dd>
          </div>
          <div>
            <dt>{t('restartPolicy')}</dt>
            <dd>{detail.RestartPolicy?.Name || '—'}</dd>
          </div>
        </dl>
      </Section>
      <Section title={t('ports')} count={ports.length}>
        {ports.length ? (
          <div className="mapping-list">
            {ports.map((p, i) => (
              <div className="mapping-row caption-5" key={i}>
                <Network />
                <code>{p.host}</code>
                <span className="mapping-arrow">→</span>
                <code>{p.port}</code>
              </div>
            ))}
          </div>
        ) : (
          <p className="caption-5 muted">{t('portEmpty')}</p>
        )}
      </Section>
      <Section title={t('mounts')} count={detail.Mounts?.length ?? 0}>
        {detail.Mounts?.length ? (
          <div className="mapping-list">
            {detail.Mounts.map((m, i) => (
              <div className="mount-row caption-5" key={i}>
                <Folder />
                <span title={`${m.Source} → ${m.Destination}`}>
                  {m.Source} → {m.Destination} {m.RW ? '(rw)' : '(ro)'}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="caption-5 muted">{t('mountEmpty')}</p>
        )}
      </Section>
      <Section title={t('networks')} count={detail.Networks?.length ?? 0}>
        {detail.Networks?.length ? (
          <div className="network-list">
            {detail.Networks.map((name) => (
              <span className="network-chip caption-5" key={name}>
                <Layers />
                {name}
              </span>
            ))}
          </div>
        ) : (
          <p className="caption-5 muted">{t('networkEmpty')}</p>
        )}
      </Section>
    </>
  )
}
function Logs({ model, t }: { model: Docker; t: Translate }) {
  const [query, setQuery] = useState(''),
    output = useRef<HTMLDivElement>(null)
  const lines = query
    ? model.logs.split('\n').filter((l) => l.toLowerCase().includes(query.toLowerCase()))
    : null
  useEffect(() => {
    if (model.following && output.current) output.current.scrollTop = output.current.scrollHeight
  }, [model.logs, model.following, query])
  useEffect(() => {
    const viewport = output.current
    if (!viewport || !model.following) return
    const observer = new ResizeObserver(() => {
      viewport.scrollTop = viewport.scrollHeight
    })
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [model.following])
  return (
    <div className="logs-panel">
      <div className="logs-toolbar">
        <label className="search-field log-search">
          <Search />
          <input
            type="search"
            placeholder={t('logSearch')}
            aria-label={t('logSearch')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <Button
          icon
          title={t(model.following && !model.logEnded ? 'pause' : 'follow')}
          aria-label={t(model.following && !model.logEnded ? 'pause' : 'follow')}
          aria-pressed={model.following && !model.logEnded}
          onClick={() => {
            if (!model.following || model.logEnded) model.resumeLogs()
            else model.setFollowing(false)
          }}
        >
          {model.following && !model.logEnded ? <Pause /> : <Play />}
        </Button>
        <CopyButton value={lines ? lines.join('\n') : model.logs} label={t('copy')} />
      </div>
      <ScrollArea className="logs-scroll" viewportRef={output}>
        <pre id="log-output" className="log-output" tabIndex={0}>
          {(lines ? lines.join('\n') : model.logs) || t('noLogs')}
        </pre>
      </ScrollArea>
      <footer className="logs-footer caption-5">
        <span className="live-label" data-live={model.following && model.active && !model.logEnded}>
          <i />
          {t(model.logEnded ? 'logEnded' : model.following ? 'live' : 'logPaused')}
        </span>
        <span title={model.logTruncated ? t('logsTruncatedHelp') : undefined}>
          {model.logTruncated
            ? t('logsTruncated')
            : lines
              ? `${lines.length} ${t('logMatches')}`
              : t('tail')}
        </span>
      </footer>
    </div>
  )
}
function ContainerMenu({
  model,
  t,
  onConfirm
}: {
  model: Docker
  t: Translate
  onConfirm: (v: Operation) => void
}) {
  const running = model.selected?.State === 'running',
    disabled = model.busy || !model.active
  return (
    <Menu
      label={t('more')}
      trigger={
        <Button icon aria-label={t('more')} title={t('more')}>
          <MoreHorizontal />
        </Button>
      }
    >
      <DropdownMenu.Label className="menu-label caption-5">
        {model.selected?.Names}
      </DropdownMenu.Label>
      {running && (
        <>
          <MenuItem disabled={disabled} onSelect={() => onConfirm('stop')}>
            <Square />
            {t('stop')}
          </MenuItem>
          <MenuItem disabled={disabled} onSelect={() => onConfirm('restart')}>
            <RotateCcw />
            {t('restart')}
          </MenuItem>
          <DropdownMenu.Separator className="menu-separator" />
          <MenuItem disabled={disabled} onSelect={() => void model.perform('exec', 'bash')}>
            <Terminal />
            {t('bash')}
          </MenuItem>
        </>
      )}
      <DropdownMenu.Separator className="menu-separator" />
      <MenuItem
        destructive
        disabled={disabled || running}
        title={running ? t('dangerHint') : undefined}
        onSelect={() => onConfirm('remove')}
      >
        <Trash2 />
        {t('remove')}
      </MenuItem>
    </Menu>
  )
}
function Detail({
  model,
  locale,
  t,
  confirm,
  style
}: {
  model: Docker
  locale: string
  t: Translate
  confirm: (v: Operation) => void
  style: CSSProperties
}) {
  const c = model.selected
  if (!c) return null
  const running = c.State === 'running'
  return (
    <aside id="detail" style={style} className="detail" aria-label={c.Names}>
      <header className="detail-heading">
        <div className="detail-identity">
          <div className="detail-name">
            <Box />
            <h2 id="selected-name" className="title-4" title={c.Names}>
              {c.Names}
            </h2>
            <Status value={c.State} t={t} />
          </div>
          <p className="caption-5 muted detail-image" title={c.Image}>
            {c.Image}
          </p>
        </div>
        <Button icon aria-label={t('back')} title={t('back')} onClick={() => model.choose(null)}>
          <X />
        </Button>
      </header>
      <Tabs.Root value={model.tab} onValueChange={model.setTab} className="detail-tabs-root">
        <div className="detail-navigation">
          <Tabs.List className="tabs-list" aria-label={t('containerName')}>
            <Tabs.Trigger id="overview-tab" className="tabs-trigger title-5" value="overview">
              {t('overview')}
            </Tabs.Trigger>
            <Tabs.Trigger id="logs-tab" className="tabs-trigger title-5" value="logs">
              {t('logs')}
            </Tabs.Trigger>
          </Tabs.List>
          <div className="detail-commands">
            <Button
              variant="outline"
              className="primary-command"
              disabled={
                model.busy ||
                !model.active ||
                (!running && !['exited', 'created'].includes(c.State))
              }
              onClick={() => (running ? void model.perform('exec', 'sh') : confirm('start'))}
            >
              {running ? <Terminal /> : <Play />}
              <span>{t(running ? 'exec' : 'start')}</span>
            </Button>
            <ContainerMenu model={model} t={t} onConfirm={confirm} />
          </div>
        </div>
        <Tabs.Content forceMount value="overview" className="detail-overview-content">
          <ScrollArea className="detail-area" viewportClassName="detail-scroll">
            {model.detailError ? (
              <div className="detail-error" role="alert">
                <AlertCircle />
                <p>{model.detailError}</p>
                <Button variant="outline" onClick={() => model.choose(c.ID)}>
                  {t('retry')}
                </Button>
              </div>
            ) : model.detail ? (
              <Properties detail={model.detail} container={c} locale={locale} t={t} />
            ) : (
              <div className="detail-skeleton" aria-busy="true">
                {[1, 2, 3, 4, 5].map((i) => (
                  <div key={i} className="skeleton" />
                ))}
              </div>
            )}
          </ScrollArea>
        </Tabs.Content>
        <Tabs.Content forceMount value="logs" className="detail-log-content">
          <Logs key={c.ID} model={model} t={t} />
        </Tabs.Content>
      </Tabs.Root>
    </aside>
  )
}
export function App() {
  const [view, setView] = useState(atermView.state)
  useEffect(() => atermView.onDidChange(setView), [])
  const [resource, setResource] = useState<'containers' | 'images'>('containers')
  const model = useDocker(view, resource),
    locale = view.environment.locale,
    t = useMemo(() => translator(locale), [locale])
  const images = useImages(model, resource === 'images'),
    resize = useDetailResize()
  const hasDetail = resource === 'containers' ? !!model.selected : !!images.selected
  const separator = (
    <div
      className="detail-resizer"
      role="separator"
      tabIndex={0}
      aria-label={t('resizePanel')}
      aria-orientation="vertical"
      aria-valuenow={Math.round(resize.ratio * 100)}
      aria-valuemin={20}
      aria-valuemax={80}
      {...resize.separator}
    />
  )
  const panelStyle = { width: `${resize.ratio * 100}%` }
  const [query, setQuery] = useState(''),
    [filter, setFilter] = useState('all'),
    [imageQuery, setImageQuery] = useState(''),
    [imageFilter, setImageFilter] = useState('all'),
    [confirmation, setConfirmation] = useState<Operation | null>(null)
  useEffect(() => {
    if (!model.active || !model.selected || resource !== 'containers') setConfirmation(null)
  }, [model.active, model.selected?.ID, resource])
  const rows = useMemo(
    () =>
      model.containers.filter(
        (c) =>
          (c.Names + ' ' + c.Image).toLowerCase().includes(query.toLowerCase()) &&
          (filter === 'all' ||
            (filter === 'running' && c.State === 'running') ||
            (filter === 'stopped' && ['exited', 'created', 'dead'].includes(c.State)) ||
            (filter === 'abnormal' &&
              (['dead', 'restarting', 'paused'].includes(c.State) || /unhealthy/.test(c.Status))))
      ),
    [model.containers, query, filter]
  )
  // Log batches must not rebuild every row in the container table.
  const listModel = useMemo(
    () => ({
      ready: model.ready,
      busy: model.busy,
      containers: model.containers,
      selected: model.selected,
      refresh: model.refresh,
      choose: model.choose
    }),
    [model.ready, model.busy, model.containers, model.selected, model.refresh, model.choose]
  )

  const error =
    (model.error || (resource === 'images' ? images.error : '')) &&
    (/not found|ENOENT|not recognized/i.test(model.error)
      ? t('missing')
      : /permission denied|access denied/i.test(model.error)
        ? t('permission')
        : /cannot connect|connection refused/i.test(model.error)
          ? t('daemon')
          : model.error || images.error)
  return (
    <main id="app" data-detail={hasDetail} data-resource={resource}>
      <header className="workspace-toolbar">
        <Environment model={model} session={view.context.sessionLabel} t={t} />
        <span className="toolbar-divider" />
        <Tabs.Root
          value={resource}
          onValueChange={(value) => setResource(value as 'containers' | 'images')}
        >
          <Tabs.List className="tabs-list resource-tabs" aria-label="Docker">
            <Tabs.Trigger className="tabs-trigger title-5" value="containers">
              {t('containersTab')}
            </Tabs.Trigger>
            <Tabs.Trigger className="tabs-trigger title-5" value="images">
              {t('imagesTab')}
            </Tabs.Trigger>
          </Tabs.List>
        </Tabs.Root>
        <label className="search-field container-search">
          <Search />
          <input
            id="search"
            type="search"
            placeholder={t(resource === 'containers' ? 'search' : 'imageSearch')}
            aria-label={t(resource === 'containers' ? 'search' : 'imageSearch')}
            value={resource === 'containers' ? query : imageQuery}
            onChange={(e) => (resource === 'containers' ? setQuery : setImageQuery)(e.target.value)}
          />
        </label>
        <StatusFilter
          value={resource === 'containers' ? filter : imageFilter}
          onChange={resource === 'containers' ? setFilter : setImageFilter}
          label={t(resource === 'containers' ? 'status' : 'imageTags')}
          options={
            resource === 'containers'
              ? (['all', 'running', 'stopped', 'abnormal'] as const).map((key) => ({
                  value: key,
                  label: t(key)
                }))
              : [
                  { value: 'all', label: t('imagesAll') },
                  { value: 'untagged', label: t('imagesDangling') }
                ]
          }
        />
        <Button
          icon
          disabled={model.busy || images.busy || !model.active}
          aria-label={t('refresh')}
          title={t('refresh')}
          onClick={() => void (resource === 'containers' ? model.refresh() : images.refresh())}
        >
          <RefreshCw />
        </Button>
      </header>
      {error && (
        <div className="error-banner caption-5" role="alert">
          <AlertCircle />
          <span>{error}</span>
        </div>
      )}
      <div className="workspace-body" ref={resize.root} data-dragging={resize.dragging}>
        <div className="resource-pane" hidden={resource !== 'containers'}>
          <section id="list" className="container-list" aria-label={t('all')}>
            <ScrollArea className="list-area">
              <ContainerList model={listModel} rows={rows} query={query} t={t} />
            </ScrollArea>
            <footer className="list-footer caption-5">
              <span role="status">
                {model.busy
                  ? t('busy')
                  : `${rows.length} / ${model.containers.length} ${t('containers')}`}
              </span>
              {!model.active && <span>{t('inactive')}</span>}
            </footer>
          </section>
          {model.selected && separator}
          <Detail
            model={model}
            locale={locale}
            t={t}
            confirm={setConfirmation}
            style={panelStyle}
          />
        </div>
        <div className="resource-pane" hidden={resource !== 'images'}>
          <ImagesPane
            model={images}
            t={t}
            locale={locale}
            engine={model.engine}
            session={view.context.sessionLabel}
            style={panelStyle}
            separator={separator}
            visible={resource === 'images'}
            query={imageQuery}
            filter={imageFilter}
          />
        </div>
      </div>
      <AlertDialog.Root
        open={!!confirmation}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null)
        }}
      >
        <AlertDialog.Portal>
          <AlertDialog.Overlay className="dialog-overlay" />
          <AlertDialog.Content className="dialog">
            <div className={`dialog-symbol ${confirmation === 'remove' ? 'danger' : ''}`}>
              {confirmation === 'remove' ? <Trash2 /> : <Info />}
            </div>
            <AlertDialog.Title className="title-4">
              {confirmation && `${t(confirmation)} · ${model.selected?.Names}`}
            </AlertDialog.Title>
            <AlertDialog.Description className="body-6 muted">
              {confirmation &&
                t(confirmation === 'remove' ? 'removeText' : 'confirmText', {
                  action: t(confirmation),
                  name: model.selected?.Names ?? '',
                  host: `${view.context.sessionLabel} / ${model.engine?.name ?? ''}`
                })}
            </AlertDialog.Description>
            <div className="dialog-actions">
              <AlertDialog.Cancel asChild>
                <Button variant="outline">{t('cancel')}</Button>
              </AlertDialog.Cancel>
              <AlertDialog.Action asChild>
                <Button
                  id="confirm-action"
                  variant={confirmation === 'remove' ? 'destructive' : 'primary'}
                  onClick={() => {
                    if (confirmation) void model.perform(confirmation)
                  }}
                >
                  {confirmation && t(confirmation)}
                </Button>
              </AlertDialog.Action>
            </div>
          </AlertDialog.Content>
        </AlertDialog.Portal>
      </AlertDialog.Root>
    </main>
  )
}
