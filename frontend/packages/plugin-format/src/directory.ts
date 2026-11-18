import {
  constants,
  openSync,
  closeSync,
  fstatSync,
  readSync,
  lstatSync,
  realpathSync,
} from 'node:fs'
import { createHash } from 'node:crypto'
import { join } from 'node:path'
import { validateManifest } from './manifest'
import type { PluginManifest, PluginLocalizations } from '../../plugin-sdk'
export interface InstalledPlugin {
  manifest: PluginManifest
  source: string
  binary?: Uint8Array
  builtIn: boolean
  localizations?: PluginLocalizations
  views?: Record<string, string>
}

function readBytes(path: string, limit: number) {
  const descriptor = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const stat = fstatSync(descriptor)
    if (!stat.isFile() || stat.size > limit) throw Error('Invalid plugin file')
    const buffer = Buffer.alloc(limit + 1)
    const length = readSync(descriptor, buffer, 0, buffer.length, 0)
    if (length > limit) throw Error('Invalid plugin file')
    return buffer.subarray(0, length)
  } finally {
    closeSync(descriptor)
  }
}
function readBounded(path: string, limit: number) {
  return readBytes(path, limit).toString('utf8')
}
function localizations(root: string, manifest: PluginManifest): PluginLocalizations | undefined {
  if (!manifest.i18n) return undefined
  const directory = join(root, 'locales'),
    stat = lstatSync(directory)
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw Error('Invalid plugin locales directory')
  const messages: PluginLocalizations['messages'] = {}
  let size = 0
  for (const [locale, path] of Object.entries(manifest.i18n.locales)) {
    const source = readBounded(join(root, path), 64 * 1024)
    size += Buffer.byteLength(source)
    if (size > 512 * 1024) throw Error('Plugin localization size limit')
    const values: unknown = JSON.parse(source)
    if (
      !values ||
      typeof values !== 'object' ||
      Array.isArray(values) ||
      Object.keys(values).length > 512
    )
      throw Error('Invalid plugin locale dictionary')
    for (const [key, value] of Object.entries(values))
      if (
        !/^[A-Za-z][A-Za-z0-9_.-]{0,120}$/.test(key) ||
        ['__proto__', 'constructor', 'prototype'].includes(key) ||
        typeof value !== 'string' ||
        !value.trim() ||
        value.length > 4096
      )
        throw Error('Invalid plugin translation')
    messages[locale] = values as Record<string, string>
  }
  return { defaultLocale: manifest.i18n.defaultLocale, messages }
}
export function inspectDirectory(directory: string): InstalledPlugin {
  if (lstatSync(directory).isSymbolicLink()) throw Error('Plugin directory must not be a symlink')
  const root = realpathSync(directory)
  const manifest = validateManifest(JSON.parse(readBounded(join(root, 'manifest.json'), 32768)))
  if (manifest.id.startsWith('aterm.')) throw Error('Reserved plugin identifier')
  const source =
    manifest.runtime === 'native' ? '' : readBounded(join(root, 'plugin.js'), 256 * 1024)
  let binary: Buffer | undefined
  if (manifest.runtime === 'native') {
    const parent = join(root, 'backend'),
      target = join(parent, manifest.backend!.platform)
    for (const path of [parent, target]) {
      const stat = lstatSync(path)
      if (!stat.isDirectory() || stat.isSymbolicLink()) throw Error('Invalid backend directory')
    }
    binary = readBytes(join(root, manifest.entry), 48 * 1024 * 1024)
    if (createHash('sha256').update(binary).digest('hex') !== manifest.backend!.sha256)
      throw Error('Native backend checksum mismatch')
  }
  const views: Record<string, string> = {}
  if (manifest.contributes.views?.length) {
    const stat = lstatSync(join(root, 'ui'))
    if (!stat.isDirectory() || stat.isSymbolicLink()) throw Error('Invalid UI directory')
    let size = 0
    for (const entry of [...new Set(manifest.contributes.views.map((v) => v.entry))].sort()) {
      const html = readBounded(join(root, entry), 2 * 1024 * 1024)
      size += Buffer.byteLength(html)
      if (size > 8 * 1024 * 1024) throw Error('Plugin UI size limit')
      views[entry] = html
    }
  }
  return {
    manifest,
    source,
    ...(binary ? { binary } : {}),
    builtIn: false,
    localizations: localizations(root, manifest),
    ...(Object.keys(views).length ? { views } : {}),
  }
}
