import { resourceProbeCommand } from './probe'
import {
  appendPoint,
  bytes,
  percentage,
  pickDisk,
  pickNetwork,
  sustained
} from './ui/resourceModel'
// Collection, interpretation and presentation belong to this plugin.
;(() => {
  const previous = new Map()
  const api = window.atermPlugin
  const t = (key, options) => api.i18n.t(key, options)
  const sessions = new Map()
  const histories = new Map()
  async function publish(token) {
    const data = sessions.get(token)
    if (!data) return
    const { resources, preference, history, interval } = data
    const disk = pickDisk(resources, preference.mount)
    const network = pickNetwork(resources, preference.network)
    const memory = resources.memory
    const used = memory?.available != null ? (1 - memory.available / memory.total) * 100 : null
    const cpuWarning = sustained(history, (p) => (p.cpu ?? 0) >= 90, interval)
    const memoryWarning = sustained(history, (p) => (p.memory ?? 0) >= 90 && p.swapping, interval)
    const warnings = [
      ...(cpuWarning ? [t('monitorCpuWarning')] : []),
      ...(memoryWarning ? [t('monitorMemoryWarning')] : []),
      ...resources.disks
        .filter((d) => d.percent >= 90)
        .map((d) =>
          t('monitorDiskWarning', {
            mount: d.mount,
            free: bytes(d.available),
            used: percentage(d.percent)
          })
        ),
      ...resources.disks
        .filter((d) => (d.inodePercent ?? 0) >= 90)
        .map((d) => t('monitorInodeWarning', { mount: d.mount }))
    ]
    const rate = (v) => (v == null ? '—' : `${bytes(v)}/s`)
    const items = [
      {
        id: 'cpu',
        label: 'CPU',
        value: percentage(resources.cpu.usage),
        detail: t('pluginsCpuHelp'),
        command: 'monitor.cpu',
        priority: 10,
        level: cpuWarning ? 'warning' : 'normal',
        trend: { values: history.map((p) => p.cpu), ceiling: 100 }
      },
      {
        id: 'memory',
        label: t('pluginsMemory'),
        value:
          used == null
            ? '—'
            : `${percentage(used)} · ${t('monitorAvailable')} ${bytes(memory?.available)}`,
        compactValue: percentage(used),
        detail: t('pluginsMemoryHelp'),
        command: 'monitor.memory',
        priority: 10,
        level: memoryWarning ? 'warning' : 'normal'
      },
      {
        id: 'disk',
        label: disk?.mount || t('monitorDisks'),
        value: disk
          ? `${percentage(disk.percent)}${disk.percent >= 90 ? ' · ' + t('monitorRemaining') + ' ' + bytes(disk.available) : ''}`
          : '—',
        compactValue: percentage(disk?.percent),
        detail: disk
          ? `${disk.mount} · inode ${percentage(disk.inodePercent)} · ${t('monitorRemaining')} ${bytes(disk.available)}`
          : t('pluginsDiskHelp'),
        command: 'monitor.disk',
        level: disk?.percent >= 90 || disk?.inodePercent >= 90 ? 'warning' : 'normal'
      },
      {
        id: 'network',
        label: '',
        value: `↓ ${rate(network?.receiveRate)} ↑ ${rate(network?.sendRate)}`,
        compactValue: `↕ ${rate(network?.receiveRate != null && network?.sendRate != null ? network.receiveRate + network.sendRate : null)}`,
        detail: `${network?.name || ''}\n${t('monitorNetworkHelp')}`,
        command: 'monitor.network'
      },
      {
        id: 'details',
        label: '',
        value: t('monitorDetails'),
        detail: t('name'),
        command: 'monitor.system',
        priority: -10
      }
    ]
    await api.request('status.publish', {
      token,
      state: 'ready',
      items,
      data: { resources, history, interval, preference, warnings }
    })
  }
  api.commands.registerCommand('monitor.select', async ({ token, args }) => {
    const data = sessions.get(token)
    if (!data || !args || typeof args !== 'object') return
    const next = { ...data.preference }
    for (const key of ['mount', 'network'])
      if (Object.hasOwn(args, key)) {
        const value = args[key]
        const available =
          key === 'mount'
            ? data.resources.disks.some((d) => d.mount === value)
            : data.resources.network.some((n) => n.name === value)
        if (value === null) delete next[key]
        else if (typeof value === 'string' && available) next[key] = value
      }
    // Persist before changing UI: a failed write must not report a saved selection.
    await api.storage.set('preferences.' + data.hostKey, next)
    if (next.network !== data.preference.network)
      data.history = data.history.map((p) => ({ ...p, receive: null, send: null }))
    data.preference = next
    await publish(token)
  })
  api.i18n.onDidChangeLocale(() => {
    for (const token of sessions.keys()) void publish(token)
  })
  const number = (v) =>
    v !== undefined && v !== '' && Number.isFinite(Number(v)) && Number(v) >= 0 ? Number(v) : null
  function parseTable(text) {
    const lines = (text || '').trim().split('\n')
    const typed = /\bType\b/.test(lines.shift() || '')
    return lines.flatMap((line) => {
      const cells = line.trim().split(/\s+/)
      const offset = typed ? 2 : 1
      if (cells.length !== offset + 5) return []
      const total = number(cells[offset]),
        used = number(cells[offset + 1]),
        available = number(cells[offset + 2])
      const usage = number(cells[offset + 3].replace('%', ''))
      if (
        total === null ||
        total <= 0 ||
        used === null ||
        available === null ||
        usage === null ||
        usage > 100
      )
        return []
      return [
        {
          device: cells[0].slice(0, 160),
          type: typed ? cells[1].slice(0, 160) : '',
          mount: cells[offset + 4].replace(/\\040/g, ' ').slice(0, 160),
          total,
          used,
          available,
          percent: usage
        }
      ]
    })
  }
  api.onRefresh(async ({ token, interval = 5, hostKey = token, sessionId = token }) => {
    try {
      const response = await api.process.run({
        token, executable: 'sh', args: ['-c', resourceProbeCommand], timeoutMs: 6000
      })
      if (response.exitCode !== 0) throw Error('Resource collection failed')
      const text = response.stdout
      if (text.includes('ATERM_UNSUPPORTED')) {
        await api.request('status.publish', { token, state: 'unsupported', items: [] })
        return
      }
      const blocks = {}
      let section = ''
      for (const line of text.split('\n')) {
        const marker = line.match(/^ATERM_([A-Z]+)$/)
        if (marker) {
          section = marker[1]
          blocks[section] = ''
          continue
        }
        if (section) blocks[section] += line + '\n'
      }
      const uptime = number(blocks.UPTIME?.trim().split(/\s+/)[0])
      const timestamp = uptime ?? Date.now() / 1000
      let last = previous.get(token)
      const elapsed = last ? timestamp - last.timestamp : 0
      if (elapsed <= 0 || elapsed > 90) last = undefined
      const rate = (current, before) =>
        current !== null && before !== undefined && before !== null && last && current >= before
          ? (current - before) / elapsed
          : null
      const cpuTicks = (blocks.CPU || '')
        .trim()
        .split('\n')
        .filter((line) => /^cpu(?:\d+)?\s/.test(line))
        .map((line) => {
          const values = line.trim().split(/\s+/).slice(1, 9).map(Number)
          if (values.length < 4 || values.some((v) => !Number.isFinite(v) || v < 0)) return null
          return {
            total: values.reduce((a, b) => a + b, 0),
            idle: (values[3] || 0) + (values[4] || 0),
            wait: values[4] ?? null
          }
        })
      const cpuUsage = (current, before, field) => {
        if (
          !current ||
          !before ||
          current[field] === null ||
          before[field] === null ||
          current.total <= before.total ||
          current[field] < before[field]
        )
          return null
        const ratio = (current[field] - before[field]) / (current.total - before.total)
        return Math.max(0, Math.min(100, (field === 'idle' ? 1 - ratio : ratio) * 100))
      }
      // CPU percentages use tick deltas, even when legacy samples omit uptime.
      const cpuLast = uptime === null ? previous.get(token)?.cpuTicks : last?.cpuTicks
      const cpu = {
        usage: cpuUsage(cpuTicks[0], cpuLast?.[0], 'idle'),
        iowait: cpuUsage(cpuTicks[0], cpuLast?.[0], 'wait'),
        count:
          number(blocks.CPUCOUNT?.trim()) || (cpuTicks.length > 1 ? cpuTicks.length - 1 : null),
        cores: cpuTicks.slice(1, 65).map((tick, i) => cpuUsage(tick, cpuLast?.[i + 1], 'idle')),
        load: (blocks.LOAD || '')
          .trim()
          .split(/\s+/)
          .slice(0, 3)
          .map(number)
          .filter((v) => v !== null)
      }
      const memoryValues = Object.fromEntries(
        [...(blocks.MEMORY || '').matchAll(/^(\w+):\s+(\d+)\s+kB/gm)].map((m) => [
          m[1],
          Number(m[2]) * 1024
        ])
      )
      const swapCounters = Object.fromEntries(
        [...(blocks.SWAPIO || '').matchAll(/^(pswpin|pswpout)\s+(\d+)/gm)].map((m) => [
          m[1],
          Number(m[2])
        ])
      )
      const pageSize = number(blocks.PAGESIZE?.trim())
      const available = memoryValues.MemAvailable
      const memory =
        memoryValues.MemTotal > 0
          ? {
              total: memoryValues.MemTotal,
              available: available >= 0 && available <= memoryValues.MemTotal ? available : null,
              cached:
                memoryValues.Cached !== undefined ||
                memoryValues.Buffers !== undefined ||
                memoryValues.SReclaimable !== undefined
                  ? (memoryValues.Cached || 0) +
                    (memoryValues.Buffers || 0) +
                    (memoryValues.SReclaimable || 0)
                  : null,
              swapTotal: memoryValues.SwapTotal ?? null,
              swapUsed:
                memoryValues.SwapTotal !== undefined && memoryValues.SwapFree !== undefined
                  ? Math.max(0, memoryValues.SwapTotal - memoryValues.SwapFree)
                  : null,
              swapInRate: pageSize
                ? rate(swapCounters.pswpin ?? null, last?.swapCounters.pswpin) * pageSize
                : null,
              swapOutRate: pageSize
                ? rate(swapCounters.pswpout ?? null, last?.swapCounters.pswpout) * pageSize
                : null
            }
          : null
      // A missing baseline is unknown, never zero. Multiplication above must preserve null.
      if (memory && pageSize) {
        const incoming = rate(swapCounters.pswpin ?? null, last?.swapCounters.pswpin)
        const outgoing = rate(swapCounters.pswpout ?? null, last?.swapCounters.pswpout)
        memory.swapInRate = incoming === null ? null : incoming * pageSize
        memory.swapOutRate = outgoing === null ? null : outgoing * pageSize
      }
      const inodes = parseTable(blocks.INODES)
      const seen = new Set()
      const disks = parseTable(blocks.DISK)
        .sort((a, b) => (a.mount === '/' ? -1 : b.mount === '/' ? 1 : 0))
        .filter((d) => {
          if (
            /^(tmpfs|devtmpfs|squashfs|proc|sysfs|cgroup2?|nsfs|ramfs|fusectl|debugfs|tracefs)$/.test(
              d.type
            ) ||
            (d.type === 'overlay' && d.mount !== '/')
          )
            return false
          if (seen.has(d.device)) return false
          seen.add(d.device)
          return true
        })
        .slice(0, 32)
        .map((d) => ({
          ...d,
          total: d.total * 1024,
          used: d.used * 1024,
          available: d.available * 1024,
          inodePercent: inodes.find((i) => i.mount === d.mount)?.percent ?? null
        }))
      const networkCounters = {}
      const network = (blocks.NETWORK || '')
        .split('\n')
        .flatMap((line) => {
          const match = line.match(/^\s*([^:]+):\s+(.+)$/)
          if (!match || match[1].trim() === 'lo') return []
          const values = match[2].trim().split(/\s+/).map(number)
          if (values.length < 16 || values.some((v) => v === null)) return []
          const name = match[1].trim().slice(0, 160)
          const counters = {
            receiveBytes: values[0],
            sendBytes: values[8],
            errors: values[2] + values[10],
            drops: values[3] + values[11]
          }
          networkCounters[name] = counters
          const before = last?.networkCounters[name]
          return [
            {
              name,
              ...counters,
              receiveRate: rate(counters.receiveBytes, before?.receiveBytes),
              sendRate: rate(counters.sendBytes, before?.sendBytes),
              errorRate: rate(counters.errors, before?.errors),
              dropRate: rate(counters.drops, before?.drops)
            }
          ]
        })
        .slice(0, 16)
      const ioCounters = {}
      const io = (blocks.DISKIO || '')
        .split('\n')
        .flatMap((line) => {
          const cells = line.trim().split(/\s+/)
          const device = cells[2]
          // Individual whole devices only: never sum logical and physical layers.
          if (
            !device ||
            !/^(?:sd[a-z]+|vd[a-z]+|xvd[a-z]+|nvme\d+n\d+|mmcblk\d+|dm-\d+|md\d+)$/.test(device) ||
            cells.length < 10
          )
            return []
          const read = number(cells[5]),
            written = number(cells[9])
          if (read === null || written === null) return []
          ioCounters[device] = { read, written }
          const before = last?.ioCounters[device]
          const r = rate(read, before?.read),
            w = rate(written, before?.written)
          return [
            {
              device,
              readRate: r === null ? null : r * 512,
              writeRate: w === null ? null : w * 512
            }
          ]
        })
        .slice(0, 16)
      const systemLines = (blocks.SYSTEM || '').trim().split('\n')
      const resources = {
        version: 1,
        system: {
          hostname: (systemLines[0] || '').slice(0, 160),
          kernel: (systemLines[1] || '').slice(0, 160),
          os: (systemLines.find((line) => line.startsWith('PRETTY_NAME=')) || '')
            .replace(/^PRETTY_NAME=["']?|["']$/g, '')
            .slice(0, 160),
          uptime
        },
        cpu,
        memory,
        disks,
        network,
        io,
        defaultInterface: (blocks.ROUTE?.match(/\bdev\s+(\S+)/)?.[1] || '').slice(0, 160)
      }
      previous.set(token, { timestamp, cpuTicks, swapCounters, networkCounters, ioCounters })
      if (previous.size > 128) previous.delete(previous.keys().next().value)
      const before = histories.get(sessionId)
      const saved = before?.preference ?? (await api.storage.get('preferences.' + hostKey))
      const preference = saved && typeof saved === 'object' && !Array.isArray(saved) ? saved : {}
      const net = pickNetwork(resources, preference.network)
      const now = Date.now()
      const previousHistory =
        before && before.token !== token && before.history.length
          ? appendPoint(before.history, {
              at: now - 1,
              cpu: null,
              memory: null,
              receive: null,
              send: null,
              swapping: false
            })
          : (before?.history ?? [])
      const history = appendPoint(previousHistory, {
        at: now,
        cpu: cpu.usage,
        memory: memory?.available != null ? (1 - memory.available / memory.total) * 100 : null,
        receive: net?.receiveRate ?? null,
        send: net?.sendRate ?? null,
        swapping: (memory?.swapOutRate ?? 0) > 0
      })
      const data = {
        resources,
        preference,
        history,
        interval,
        hostKey,
        sessionId,
        token
      }
      for (const [oldToken, entry] of sessions)
        if (entry.sessionId === sessionId) sessions.delete(oldToken)
      histories.set(sessionId, data)
      sessions.set(token, data)
      if (histories.size > 128) histories.delete(histories.keys().next().value)
      if (sessions.size > 128) sessions.delete(sessions.keys().next().value)
      await publish(token)
    } catch {
      await api.request('status.publish', { token, state: 'delayed', items: [] })
    }
  })
  void api.request('runtime.ready', {})
})()
