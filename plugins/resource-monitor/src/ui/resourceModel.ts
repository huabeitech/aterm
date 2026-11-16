import type { ResourceSnapshot } from '../resources'
export interface ResourcePoint {
  at: number
  cpu: number | null
  memory: number | null
  receive: number | null
  send: number | null
  swapping: boolean
}
export const bytes = (value: number | null | undefined) => {
  if (value == null || !Number.isFinite(value)) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(unit > 0 && value < 100 ? 1 : 0)} ${units[unit]}`
}
export const percentage = (value: number | null | undefined) =>
  value == null ? '—' : `${Math.round(value)}%`
export function pickDisk(resources?: ResourceSnapshot, mount?: string) {
  return (
    resources?.disks.find((d) => d.mount === mount) ??
    [...(resources?.disks ?? [])].sort((a, b) => b.percent - a.percent)[0]
  )
}
export function pickNetwork(resources?: ResourceSnapshot, name?: string) {
  return (
    resources?.network.find((n) => n.name === name) ??
    resources?.network.find((n) => n.name === resources.defaultInterface) ??
    resources?.network.find((n) => !/^(veth|docker|br-|virbr)/.test(n.name)) ??
    resources?.network[0]
  )
}
export function appendPoint(history: ResourcePoint[], point: ResourcePoint) {
  return [...history.filter((p) => p.at > point.at - 300000 && p.at < point.at), point].slice(-120)
}
export function sustained(
  history: ResourcePoint[],
  test: (point: ResourcePoint) => boolean,
  interval: number
) {
  const last = history.at(-1)
  if (!last || !test(last)) return false
  let start = last.at
  for (let i = history.length - 2; i >= 0; i--) {
    if (!test(history[i]) || start - history[i].at > interval * 2000 + 1000) break
    start = history[i].at
  }
  return last.at - start >= 30000
}
export interface TrendPoint {
  at: number
  value: number
  x: number
  y: number
}
/** Five minutes of actual sample time; missing samples split the line. */
export function trendSegments(
  history: { at: number; value: number | null }[],
  interval: number,
  ceiling?: number,
  width = 300,
  height = 96
): TrendPoint[][] {
  const points = history.filter((p) => Number.isFinite(p.at)).sort((a, b) => a.at - b.at)
  if (!points.length) return []
  const end = points.at(-1)!.at,
    start = end - 300000
  const max = ceiling ?? trendCeiling(points)
  const segments: TrendPoint[][] = []
  let segment: TrendPoint[] = []
  let previous: number | undefined
  for (const point of points) {
    if (point.at < start) continue
    if (
      point.value == null ||
      !Number.isFinite(point.value) ||
      (previous !== undefined && point.at - previous > interval * 2000 + 1000)
    ) {
      if (segment.length) segments.push(segment)
      segment = []
    }
    if (point.value != null && Number.isFinite(point.value))
      segment.push({
        at: point.at,
        value: point.value,
        x: 4 + ((point.at - start) / 300000) * Math.max(0, width - 8),
        y: 4 + (1 - Math.max(0, Math.min(1, point.value / max))) * (height - 8)
      })
    previous = point.at
  }
  if (segment.length) segments.push(segment)
  return segments
}
export function trendCeiling(points: { value: number | null }[]) {
  const peak = Math.max(1, ...points.map((p) => (Number.isFinite(p.value) ? p.value! : 0)))
  const step = 10 ** Math.floor(Math.log10(peak))
  return Math.ceil(peak / step) * step
}
