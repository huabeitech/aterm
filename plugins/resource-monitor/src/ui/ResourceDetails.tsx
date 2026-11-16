import type { ResourceSnapshot } from '../resources'
import type { PluginTranslate } from './types'
import { SelectField } from './controls'
import { ResourceTrend } from './ResourceTrend'
import { bytes, percentage, pickNetwork, type ResourcePoint } from './resourceModel'
export type ResourceGroup = 'system' | 'cpu' | 'memory' | 'disk' | 'network'
export interface MonitorPreferences {
  mount?: string
  network?: string
}
interface Props {
  t: PluginTranslate
  resources?: ResourceSnapshot
  history: ResourcePoint[]
  interval: number
  sessionLabel: string
  state: string
  section: ResourceGroup
  onSection: (section: ResourceGroup) => void
  warnings: string[]
  error: string
  preference: MonitorPreferences
  select: (patch: MonitorPreferences) => void
}
export function ResourceDetails({
  t,
  resources,
  history,
  interval,
  sessionLabel,
  state,
  section,
  onSection,
  warnings,
  error,
  preference,
  select
}: Props) {
  const cpu = resources?.cpu,
    memory = resources?.memory
  const network = pickNetwork(resources, preference.network)
  const memoryPercent =
    memory?.available != null ? (1 - memory.available / memory.total) * 100 : null
  const rate = (value: number | null | undefined) => (value == null ? '—' : `${bytes(value)}/s`)
  const plot = (key: 'cpu' | 'memory' | 'receive' | 'send', label: string, ceiling?: number) => (
    <ResourceTrend
      points={history.map((p) => ({ at: p.at, value: p[key] }))}
      interval={interval}
      ceiling={ceiling}
      label={label}
      windowLabel={t('monitorChartWindow')}
      nowLabel={t('monitorChartNow')}
      emptyLabel={t('monitorCollecting')}
      formatValue={ceiling === 100 ? percentage : rate}
    />
  )
  return (
    <aside className="resource-details" aria-label={t('monitorDetails')}>
      <header>
        <div className="min-w-0">
          <h2>{t('monitorDetails')}</h2>
          <p className="truncate" title={sessionLabel}>
            {sessionLabel} · {state}
          </p>
        </div>
      </header>
      <nav>
        {(['system', 'cpu', 'memory', 'disk', 'network'] as const).map((group) => (
          <button
            key={group}
            aria-current={section === group ? 'true' : undefined}
            onClick={() => onSection(group)}
          >
            {group === 'system'
              ? t('monitorSystem')
              : group === 'cpu'
                ? 'CPU'
                : group === 'memory'
                  ? t('pluginsMemory')
                  : group === 'disk'
                    ? t('monitorDisks')
                    : t('monitorNetwork')}
          </button>
        ))}
      </nav>
      <div className="resource-details-body">
        <section data-resource-section="system">
          <h3>{t('monitorSystem')}</h3>
          <dl>
            <Row name={t('monitorHostname')} value={resources?.system.hostname || '—'} />
            <Row name={t('monitorOs')} value={resources?.system.os || '—'} />
            <Row name={t('monitorKernel')} value={resources?.system.kernel || '—'} />
            <Row
              name={t('monitorUptime')}
              value={
                resources?.system.uptime == null
                  ? '—'
                  : t('monitorUptimeValue', {
                      days: Math.floor(resources.system.uptime / 86400),
                      hours: Math.floor((resources.system.uptime % 86400) / 3600),
                      minutes: Math.floor((resources.system.uptime % 3600) / 60)
                    })
              }
            />
          </dl>
          {warnings.map((warning) => (
            <p className="resource-warning" key={warning}>
              {warning}
            </p>
          ))}
          {error && <p role="alert">{error}</p>}
        </section>
        <section data-resource-section="cpu">
          <h3 title={t('pluginsCpuHelp')}>CPU · {percentage(cpu?.usage)}</h3>
          {plot('cpu', t('monitorCpuTrend'), 100)}
          <dl>
            <Row name={t('monitorLogicalCpus')} value={cpu?.count ?? '—'} />
            <Row
              help={t('monitorLoadHelp')}
              name={t('monitorLoad')}
              value={cpu?.load.length ? cpu.load.map((v) => v.toFixed(2)).join(' / ') : '—'}
            />
            <Row name={t('monitorIoWait')} value={percentage(cpu?.iowait)} />
          </dl>
          <div className="resource-cores">
            {cpu?.cores.map((value, index) => (
              <span key={index} title={`CPU ${index}: ${percentage(value)}`}>
                #{index} {percentage(value)}
              </span>
            ))}
          </div>
          {cpu && cpu.count !== null && cpu.count > cpu.cores.length && cpu.cores.length > 0 && (
            <p className="resource-help">{t('monitorCoreLimit', { count: cpu.cores.length })}</p>
          )}
        </section>
        <section data-resource-section="memory">
          <h3 title={t('pluginsMemoryHelp')}>
            {t('pluginsMemory')} · {percentage(memoryPercent)}
          </h3>
          {plot('memory', t('monitorMemoryTrend'), 100)}
          <dl>
            <Row
              name={t('monitorUsed')}
              value={bytes(memory?.available != null ? memory.total - memory.available : null)}
            />
            <Row name={t('monitorAvailable')} value={bytes(memory?.available)} />
            <Row name={t('monitorTotal')} value={bytes(memory?.total)} />
            <Row name={t('monitorCache')} value={bytes(memory?.cached)} />
            <Row
              name="Swap"
              value={
                memory?.swapTotal === 0
                  ? t('monitorSwapDisabled')
                  : `${bytes(memory?.swapUsed)} / ${bytes(memory?.swapTotal)}`
              }
            />
            <Row
              name={t('monitorSwapActivity')}
              value={`↓ ${rate(memory?.swapInRate)} ↑ ${rate(memory?.swapOutRate)}`}
            />
          </dl>
        </section>
        <section data-resource-section="disk">
          <h3>{t('monitorDisks')}</h3>
          <SelectField
            aria-label={t('monitorPinDisk')}
            value={
              resources?.disks.some((d) => d.mount === preference.mount)
                ? preference.mount!
                : 'auto'
            }
            disabled={!resources?.disks.length}
            options={[
              { value: 'auto', label: t('monitorAutomatic') },
              ...(resources?.disks.map((d) => ({ value: d.mount, label: d.mount })) ?? [])
            ]}
            onValueChange={(value) => select({ mount: value === 'auto' ? undefined : value })}
          />
          {!resources?.disks.length && <p className="resource-help">{t('monitorMissing')}</p>}
          {resources?.disks.map((d) => (
            <div className="resource-device" key={d.mount}>
              <h4>
                {d.mount} · {percentage(d.percent)}
              </h4>
              <dl>
                <Row name={t('monitorRemaining')} value={bytes(d.available)} />
                <Row name={t('monitorUsed')} value={`${bytes(d.used)} / ${bytes(d.total)}`} />
                <Row help={t('monitorInodeHelp')} name="inode" value={percentage(d.inodePercent)} />
                <Row name={t('monitorDevice')} value={`${d.device} ${d.type}`} />
              </dl>
            </div>
          ))}
          <h4 title={t('monitorDiskIoHelp')}>{t('monitorDiskIo')}</h4>
          {resources?.io.map((d) => (
            <div className="resource-device" key={d.device}>
              <Row name={d.device} value={`↓ ${rate(d.readRate)} ↑ ${rate(d.writeRate)}`} />
            </div>
          ))}
          {!resources?.io.length && <p className="resource-help">{t('monitorMissing')}</p>}
        </section>
        <section data-resource-section="network">
          <h3 title={t('monitorNetworkHelp')}>
            {t('monitorNetwork')} · {network?.name || '—'}
          </h3>
          <SelectField
            aria-label={t('monitorPinNetwork')}
            value={
              resources?.network.some((n) => n.name === preference.network)
                ? preference.network!
                : 'auto'
            }
            disabled={!resources?.network.length}
            options={[
              { value: 'auto', label: t('monitorAutomatic') },
              ...(resources?.network.map((n) => ({ value: n.name, label: n.name })) ?? [])
            ]}
            onValueChange={(value) => select({ network: value === 'auto' ? undefined : value })}
          />
          <p className="resource-help">{t('monitorReceive')}</p>
          {plot('receive', t('monitorReceive'))}
          <p className="resource-help">{t('monitorSend')}</p>
          {plot('send', t('monitorSend'))}
          {resources?.network.map((n) => (
            <div className="resource-device" key={n.name}>
              <h4>{n.name}</h4>
              <dl>
                <Row name={t('monitorReceive')} value={rate(n.receiveRate)} />
                <Row name={t('monitorSend')} value={rate(n.sendRate)} />
                <Row
                  name={t('monitorTransferred')}
                  value={`↓ ${bytes(n.receiveBytes)} ↑ ${bytes(n.sendBytes)}`}
                />
                <Row
                  name={t('monitorErrors')}
                  value={`${n.errors} · ${n.errorRate === null ? '—' : n.errorRate.toFixed(1) + '/s'}`}
                />
                <Row
                  name={t('monitorDrops')}
                  value={`${n.drops} · ${n.dropRate === null ? '—' : n.dropRate.toFixed(1) + '/s'}`}
                />
              </dl>
            </div>
          ))}
          {!resources?.network.length && <p className="resource-help">{t('monitorMissing')}</p>}
        </section>
      </div>
    </aside>
  )
}
function Row({ name, value, help }: { name: string; value: string | number; help?: string }) {
  return (
    <div className="resource-detail-row">
      <dt title={help}>{name}</dt>
      <dd>{value}</dd>
    </div>
  )
}
