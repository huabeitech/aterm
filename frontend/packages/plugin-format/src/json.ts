import type { JsonValue } from '../../plugin-sdk'
/** JSON is the only value allowed across the plugin boundary. */
export function jsonSnapshot(input: unknown, maxBytes = 196608): JsonValue {
  let nodes = 0
  function visit(value: unknown, depth: number): JsonValue {
    if (++nodes > 20000 || depth > 16) throw Error('Plugin data limit')
    if (value === null || typeof value === 'boolean' || typeof value === 'string') return value
    if (typeof value === 'number' && Number.isFinite(value)) return value
    if (Array.isArray(value)) return value.map((v) => visit(v, depth + 1))
    if (value && typeof value === 'object' && Object.getPrototypeOf(value) === Object.prototype) {
      const result: Record<string, JsonValue> = {}
      for (const [key, child] of Object.entries(value)) {
        if (['__proto__', 'constructor', 'prototype'].includes(key)) throw Error('Invalid data key')
        result[key] = visit(child, depth + 1)
      }
      return result
    }
    throw Error('Plugin data must be JSON')
  }
  const result = visit(input, 0)
  if (new TextEncoder().encode(JSON.stringify(result)).byteLength > maxBytes)
    throw Error('Plugin data limit')
  return result
}
