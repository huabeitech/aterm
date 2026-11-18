import { crc32 } from 'node:zlib'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fromBuffer } from 'yauzl'
import { zipSync } from 'fflate'
import { inspectDirectory, type InstalledPlugin } from './directory'

const archiveLimit = 64 * 1024 * 1024
const fileLimit = (name: string) =>
  name.startsWith('backend/') ? 48 * 1024 * 1024 : 2 * 1024 * 1024
const countLimit = 128

export function packageFiles(plugin: InstalledPlugin): Record<string, Uint8Array> {
  const files: Record<string, Uint8Array> = Object.create(null)
  const add = (path: string, value: string) => {
    files[path] = Buffer.from(value)
  }
  add('manifest.json', JSON.stringify(plugin.manifest, null, 2) + '\n')
  if (plugin.manifest.runtime === 'native') {
    if (!plugin.binary) throw Error('Native backend missing')
    files[plugin.manifest.entry] = plugin.binary
  } else add('plugin.js', plugin.source)
  for (const [path, html] of Object.entries(plugin.views ?? {})) add(path, html)
  if (plugin.localizations && plugin.manifest.i18n)
    for (const [locale, path] of Object.entries(plugin.manifest.i18n.locales))
      add(path, JSON.stringify(plugin.localizations.messages[locale], null, 2) + '\n')
  return files
}

export function createArchive(plugin: InstalledPlugin): Uint8Array {
  const files = packageFiles(plugin)
  // Fixed timestamps and sorted entries make identical builds reproducible.
  const entries = Object.fromEntries(
    Object.keys(files)
      .sort()
      .map((path) => [path, [files[path], { mtime: new Date(2020, 0, 1) }]]),
  )
  return zipSync(entries as Parameters<typeof zipSync>[0], { level: 9 })
}

/** Bound and validate all entries before materializing allowlisted paths for inspection. */
export async function inspectArchive(path: string): Promise<InstalledPlugin> {
  const { openSync, closeSync, fstatSync, readSync } = await import('node:fs')
  const descriptor = openSync(path, 'r')
  let bytes: Buffer
  try {
    if (fstatSync(descriptor).size > archiveLimit) throw Error('Plugin archive size limit')
    const buffer = Buffer.alloc(archiveLimit + 1)
    let length = 0
    while (length < buffer.length) {
      const read = readSync(descriptor, buffer, length, buffer.length - length, null)
      if (!read) break
      length += read
    }
    bytes = buffer.subarray(0, length)
    if (bytes.length > archiveLimit) throw Error('Plugin archive size limit')
  } finally {
    closeSync(descriptor)
  }
  const files = await new Promise<Map<string, Buffer>>((resolve, reject) => {
    fromBuffer(
      bytes,
      { lazyEntries: true, decodeStrings: true, validateEntrySizes: true, strictFileNames: true },
      (error, zip) => {
        if (error || !zip) return reject(error ?? Error('Invalid plugin archive'))
        const result = new Map<string, Buffer>()
        let size = 0
        let count = 0
        let failed = false
        const fail = (error: Error) => {
          if (failed) return
          failed = true
          zip.close()
          reject(error)
        }
        zip.on('error', fail)
        zip.on('end', () => {
          if (!failed) resolve(result)
        })
        zip.on('entry', (entry) => {
          const name = entry.fileName
          const mode = (entry.externalFileAttributes >>> 16) & 0xf000
          if (
            ++count > countLimit ||
            result.has(name) ||
            !/^(manifest\.json|plugin\.js|ui\/[A-Za-z0-9._-]+\.html|locales\/[A-Za-z0-9._-]+\.json|backend\/(darwin|linux|windows)-(amd64|arm64)\/plugin(\.exe)?)$/.test(
              name,
            ) ||
            (mode !== 0 && mode !== 0x8000) ||
            entry.generalPurposeBitFlag & 1 ||
            entry.uncompressedSize > fileLimit(name) ||
            (size += entry.uncompressedSize) > archiveLimit
          ) {
            return fail(Error('Invalid plugin archive entry'))
          }
          zip.openReadStream(entry, (error, stream) => {
            if (error || !stream) return fail(error ?? Error('Invalid plugin archive stream'))
            const chunks: Buffer[] = []
            let length = 0
            stream.on('error', fail)
            stream.on('data', (chunk: Buffer) => {
              length += chunk.length
              if (length > entry.uncompressedSize || length > fileLimit(name)) {
                stream.destroy()
                fail(Error('Plugin archive expansion limit'))
                return
              }
              chunks.push(chunk)
            })
            stream.on('end', () => {
              if (failed) return
              const bytes = Buffer.concat(chunks)
              if (crc32(bytes) !== entry.crc32)
                return fail(Error('Plugin archive checksum mismatch'))
              result.set(name, bytes)
              zip.readEntry()
            })
          })
        })
        zip.readEntry()
      },
    )
  })
  const temporary = mkdtempSync(join(tmpdir(), 'aterm-plugin-'))
  try {
    for (const [name, bytes] of files) {
      const target = join(temporary, name)
      mkdirSync(dirname(target), { recursive: true })
      writeFileSync(target, bytes, { flag: 'wx', mode: 0o600 })
    }
    const plugin = inspectDirectory(temporary)
    const expected = Object.keys(packageFiles(plugin))
    if (expected.length !== files.size || expected.some((name) => !files.has(name)))
      throw Error('Undeclared plugin archive files')
    return plugin
  } finally {
    rmSync(temporary, { recursive: true, force: true })
  }
}
