import type { PluginManifest, StatusItem } from '../contracts/plugins'

export { jsonSnapshot } from '../../../packages/plugin-format/src/json'
import { jsonSnapshot } from '../../../packages/plugin-format/src/json'

export function validateStatusItems(input: unknown, manifest: PluginManifest): StatusItem[] {
  if (!Array.isArray(input) || input.length > 16) throw Error('Invalid status items')
  const ids = new Set<string>()
  return input.map((item) => {
    if (!item || typeof item !== 'object') throw Error('Invalid status item')
    for (const key of ['id', 'label', 'value'])
      if (typeof item[key] !== 'string' || item[key].length > 150)
        throw Error('Invalid status item')
    if (!item.id || ids.has(item.id)) throw Error('Duplicate or empty status item')
    ids.add(item.id)
    if (
      item.command !== undefined &&
      !manifest.contributes.commands?.some((c) => c.id === item.command)
    )
      throw Error('Command not declared')
    const result: StatusItem = {
      id: item.id,
      label: item.label,
      value: item.value,
      level: ['warning', 'error'].includes(item.level) ? item.level : 'normal'
    }
    if (item.command) result.command = item.command
    if (item.visibility !== undefined) {
      if (!['always', 'expanded'].includes(item.visibility))
        throw Error('Invalid status visibility')
      result.visibility = item.visibility
    }
    if (typeof item.detail === 'string') result.detail = item.detail.slice(0, 2048)
    if (typeof item.compactValue === 'string') result.compactValue = item.compactValue.slice(0, 150)
    if (Number.isFinite(item.priority))
      result.priority = Math.max(-100, Math.min(100, item.priority))
    if (item.trend !== undefined) {
      const trend = item.trend
      if (
        !Array.isArray(trend?.values) ||
        trend.values.length > 120 ||
        trend.values.some(
          (v: unknown) => v !== null && (typeof v !== 'number' || !Number.isFinite(v) || v < 0)
        ) ||
        (trend.ceiling !== undefined && (!Number.isFinite(trend.ceiling) || trend.ceiling <= 0))
      )
        throw Error('Invalid status trend')
      result.trend = {
        values: [...trend.values],
        ...(trend.ceiling ? { ceiling: trend.ceiling } : {})
      }
    }
    return result
  })
}

/** Availability can only target views explicitly opting into dynamic discovery. */
export function validateViewAvailability(input: unknown, manifest: PluginManifest) {
  const value = jsonSnapshot(input, 8192)
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw Error('Invalid view availability')
  const result: Record<string, import('../../../packages/plugin-sdk').PluginViewAvailability> = {}
  for (const [id, state] of Object.entries(value)) {
    if (
      !manifest.contributes.views?.some(
        (view) => view.id === id && view.availability === 'dynamic'
      ) ||
      !state ||
      typeof state !== 'object' ||
      Array.isArray(state) ||
      Object.keys(state).some((key) => !['visible', 'available', 'message'].includes(key)) ||
      typeof state.visible !== 'boolean' ||
      typeof state.available !== 'boolean' ||
      (state.message !== undefined &&
        (typeof state.message !== 'string' || state.message.length > 500))
    )
      throw Error('Invalid view availability')
    result[id] = state as unknown as import('../../../packages/plugin-sdk').PluginViewAvailability
  }
  return result
}
