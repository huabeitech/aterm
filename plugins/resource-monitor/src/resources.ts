/** Structured, text-only resource contribution. Byte values use binary units in the UI. */
export interface ResourceSnapshot {
  version: 1
  system: { hostname: string; os: string; kernel: string; uptime: number | null }
  cpu: {
    usage: number | null
    iowait: number | null
    count: number | null
    cores: (number | null)[]
    load: number[]
  }
  memory: {
    total: number
    available: number | null
    cached: number | null
    swapTotal: number | null
    swapUsed: number | null
    swapInRate: number | null
    swapOutRate: number | null
  } | null
  disks: {
    device: string
    mount: string
    type: string
    total: number
    used: number
    available: number
    percent: number
    inodePercent: number | null
  }[]
  network: {
    name: string
    receiveBytes: number
    sendBytes: number
    receiveRate: number | null
    sendRate: number | null
    errors: number
    drops: number
    errorRate: number | null
    dropRate: number | null
  }[]
  io: { device: string; readRate: number | null; writeRate: number | null }[]
  defaultInterface: string
}
// Never pass arbitrary plugin objects to renderer contributions. Project known bounded fields.
export function validateResourceSnapshot(input: unknown): ResourceSnapshot {
  const obj = (v: unknown): Record<string, unknown> => {
    if (!v || typeof v !== 'object' || Array.isArray(v)) throw Error('Invalid resource snapshot')
    return v as Record<string, unknown>
  }
  const text = (v: unknown) => {
    if (typeof v !== 'string' || v.length > 160) throw Error('Invalid resource text')
    return v
  }
  const number = (v: unknown, max = Number.MAX_SAFE_INTEGER) => {
    if (typeof v !== 'number' || !Number.isFinite(v) || v < 0 || v > max)
      throw Error('Invalid resource number')
    return v
  }
  const nullable = (v: unknown, max?: number) => (v === null ? null : number(v, max))
  const list = (v: unknown, max: number) => {
    if (!Array.isArray(v) || v.length > max) throw Error('Invalid resource list')
    return v
  }
  const m = obj(input),
    system = obj(m.system),
    cpu = obj(m.cpu),
    memory = m.memory === null ? null : obj(m.memory)
  if (m.version !== 1) throw Error('Unsupported resource snapshot')
  if (
    memory &&
    (number(memory.total) <= 0 ||
      (memory.available !== null && number(memory.available) > number(memory.total)))
  )
    throw Error('Invalid memory capacity')
  return {
    version: 1,
    system: {
      hostname: text(system.hostname),
      os: text(system.os),
      kernel: text(system.kernel),
      uptime: nullable(system.uptime)
    },
    cpu: {
      usage: nullable(cpu.usage, 100),
      iowait: nullable(cpu.iowait, 100),
      count: nullable(cpu.count, 1000000),
      cores: list(cpu.cores, 64).map((v) => nullable(v, 100)),
      load: list(cpu.load, 3).map((v) => number(v))
    },
    memory: memory && {
      total: number(memory.total),
      available: nullable(memory.available),
      cached: nullable(memory.cached),
      swapTotal: nullable(memory.swapTotal),
      swapUsed: nullable(memory.swapUsed),
      swapInRate: nullable(memory.swapInRate),
      swapOutRate: nullable(memory.swapOutRate)
    },
    disks: list(m.disks, 32).map((v) => {
      const d = obj(v)
      return {
        device: text(d.device),
        mount: text(d.mount),
        type: text(d.type),
        total: number(d.total),
        used: number(d.used),
        available: number(d.available),
        percent: number(d.percent, 100),
        inodePercent: nullable(d.inodePercent, 100)
      }
    }),
    network: list(m.network, 16).map((v) => {
      const n = obj(v)
      return {
        name: text(n.name),
        receiveBytes: number(n.receiveBytes),
        sendBytes: number(n.sendBytes),
        receiveRate: nullable(n.receiveRate),
        sendRate: nullable(n.sendRate),
        errors: number(n.errors),
        drops: number(n.drops),
        errorRate: nullable(n.errorRate),
        dropRate: nullable(n.dropRate)
      }
    }),
    io: list(m.io, 16).map((v) => {
      const d = obj(v)
      return {
        device: text(d.device),
        readRate: nullable(d.readRate),
        writeRate: nullable(d.writeRate)
      }
    }),
    defaultInterface: text(m.defaultInterface)
  }
}
