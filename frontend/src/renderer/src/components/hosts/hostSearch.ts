import { domain as service } from '../../api/types'

export type HostSearchResult = {
  host: service.Host
  score: number
  matches: HostSearchMatches
}

export type HostSearchMatches = {
  name: number[]
  alias: number[]
  hostName: number[]
  remark: number[]
}

const fieldWeights = {
  name: 1000,
  alias: 1000,
  hostName: 700,
  remark: 300
}

export function searchHosts(hosts: service.Host[], query: string): HostSearchResult[] {
  const normalizedQuery = normalize(query)
  if (!normalizedQuery) {
    return recentFirstHosts(hosts).map((host, index) => ({
      host,
      score: -index,
      matches: emptyMatches()
    }))
  }

  const tokens = normalizedQuery.split(/\s+/).filter(Boolean)
  const compactQuery = tokens.join('')
  return hosts
    .map((host, index) => {
      const ranked = rankHost(host, compactQuery, tokens)
      return ranked === null
        ? null
        : { host, score: ranked.score - index / 10000, matches: ranked.matches }
    })
    .filter((result): result is HostSearchResult => Boolean(result))
    .sort((a, b) => b.score - a.score || a.host.name.localeCompare(b.host.name))
}

function recentFirstHosts(hosts: service.Host[]): service.Host[] {
  const seenIDs = new Set<string>()
  const uniqueHosts = hosts.filter((host) => {
    if (seenIDs.has(host.id)) return false
    seenIDs.add(host.id)
    return true
  })

  return uniqueHosts.sort((a, b) => timestampValue(b.lastUsedAt) - timestampValue(a.lastUsedAt))
}

function timestampValue(value: string) {
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : 0
}

export function rankHost(host: service.Host, compactQuery: string, tokens: string[]) {
  const fields = [
    { name: 'alias' as const, value: normalize(host.alias) },
    { name: 'name' as const, value: normalize(host.name) },
    { name: 'hostName' as const, value: normalize(host.hostName) },
    { name: 'remark' as const, value: normalize(host.remark) }
  ]

  const matches = emptyMatches()
  let score = 0
  let matched = false

  const compactMatch = bestFieldMatch(fields, compactQuery)
  if (compactMatch) {
    matched = true
    score += compactMatch.score
    mergeMatches(matches, compactMatch.name, compactMatch.indices)
  }

  for (const token of tokens) {
    const tokenMatch = bestFieldMatch(fields, token)
    if (!tokenMatch) {
      if (!compactMatch) return null
      continue
    }
    matched = true
    score += tokenMatch.score / 3
    mergeMatches(matches, tokenMatch.name, tokenMatch.indices)
  }

  if (!matched) {
    return null
  }

  return { score, matches }
}

type SearchField = { name: keyof HostSearchMatches; value: string }
type FieldMatch = { name: keyof HostSearchMatches; score: number; indices: number[] }

function bestFieldMatch(fields: SearchField[], query: string): FieldMatch | null {
  let best: FieldMatch | null = null
  for (const field of fields) {
    const match = matchField(field, query)
    if (match && (!best || match.score > best.score)) best = match
  }
  return best
}

function matchField(field: SearchField, query: string): FieldMatch | null {
  if (!query || !field.value) return null

  const fieldWeight = fieldWeights[field.name]
  if (field.value === query) {
    return { name: field.name, score: fieldWeight + 4000, indices: range(0, query.length) }
  }
  if (field.value.startsWith(query)) {
    return {
      name: field.name,
      score: fieldWeight + 3000 - field.value.length,
      indices: range(0, query.length)
    }
  }

  const containsIndex = field.value.indexOf(query)
  if (containsIndex >= 0) {
    return {
      name: field.name,
      score: fieldWeight + 2200 - containsIndex * 8,
      indices: range(containsIndex, containsIndex + query.length)
    }
  }

  const fuzzy = fuzzyMatch(field.value, query)
  if (!fuzzy) return null
  return { name: field.name, score: fieldWeight + fuzzy.score, indices: fuzzy.indices }
}

function fuzzyMatch(value: string, query: string): { score: number; indices: number[] } | null {
  const indices: number[] = []
  let searchFrom = 0
  for (const char of query) {
    const index = value.indexOf(char, searchFrom)
    if (index < 0) return null
    indices.push(index)
    searchFrom = index + 1
  }

  let consecutive = 0
  let gapPenalty = 0
  for (let index = 1; index < indices.length; index += 1) {
    const gap = indices[index] - indices[index - 1] - 1
    if (gap === 0) consecutive += 1
    gapPenalty += gap
  }

  const startBonus = indices[0] === 0 ? 220 : Math.max(0, 80 - indices[0] * 8)
  return {
    indices,
    score: 900 + startBonus + consecutive * 80 - gapPenalty * 12 - value.length
  }
}

function emptyMatches(): HostSearchMatches {
  return { name: [], alias: [], hostName: [], remark: [] }
}

function mergeMatches(
  matches: HostSearchMatches,
  field: keyof HostSearchMatches,
  indices: number[]
) {
  matches[field] = Array.from(new Set([...matches[field], ...indices])).sort((a, b) => a - b)
}

function range(start: number, end: number) {
  return Array.from({ length: Math.max(end - start, 0) }, (_, index) => start + index)
}

function normalize(value: string | null | undefined) {
  return (value ?? '').trim().toLowerCase()
}
