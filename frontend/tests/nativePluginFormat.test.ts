import assert from 'node:assert/strict'
import test from 'node:test'
import { createHash } from 'node:crypto'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { validateManifest } from '../packages/plugin-format/src/manifest'
import { inspectDirectory } from '../packages/plugin-format/src/directory'
const binary = Buffer.from('fixture-native-binary')
const manifest = {
  schemaVersion: 1,
  id: 'example.native',
  name: 'Native fixture',
  version: '1.0.0',
  runtime: 'native',
  entry: 'backend/windows-amd64/plugin.exe',
  backend: { platform: 'windows-amd64', sha256: createHash('sha256').update(binary).digest('hex') },
  permissions: ['native.execute', 'hosts.read'],
  contributes: {
    views: [
      {
        id: 'native.workspace',
        title: 'view.title',
        placement: 'app.workspace',
        entry: 'ui/workspace.html',
        locations: ['sidebar', 'host.details', 'connection'],
        icon: 'M8,8H248V248H8Z',
      },
    ],
    commands: [{ id: 'native.request', title: 'command.title', requiresUnlock: true }],
  },
}
test('native package declarations retain platform, entry points and unlock requirements', () => {
  const m = validateManifest(manifest)
  assert.equal(m.runtime, 'native')
  assert.equal(m.contributes.views?.[0].icon, manifest.contributes.views[0].icon)
  assert.throws(()=>validateManifest({...manifest,contributes:{...manifest.contributes,views:[{...manifest.contributes.views[0],icon:'<svg onload="alert(1)"/>'}]}}),/icon/i)
  assert.equal(m.entry, 'backend/windows-amd64/plugin.exe')
  assert.deepEqual(m.backend, manifest.backend)
  assert.deepEqual(m.contributes.views?.[0].locations, manifest.contributes.views[0].locations)
  assert.equal(m.contributes.commands?.[0].requiresUnlock, true)
  assert.throws(() => validateManifest({ ...manifest, entry: '../plugin.exe' }))
  assert.throws(() => validateManifest({ ...manifest, permissions: ['hosts.read'] }))
  assert.throws(() =>
    validateManifest({ ...manifest, backend: { ...manifest.backend, unknown: true } }),
  )
})
test('native directory inspection verifies binary bytes before installation', () => {
  const root = mkdtempSync(join(tmpdir(), 'aterm-native-format-'))
  try {
    mkdirSync(join(root, 'backend/windows-amd64'), { recursive: true })
    mkdirSync(join(root, 'ui'))
    writeFileSync(join(root, 'manifest.json'), JSON.stringify(manifest))
    writeFileSync(join(root, manifest.entry), binary)
    writeFileSync(join(root, 'ui/workspace.html'), '<p>fixture</p>')
    const inspected = inspectDirectory(root)
    assert.deepEqual(inspected.binary, binary)
    assert.equal(inspected.source, '')
    writeFileSync(join(root, manifest.entry), 'modified')
    assert.throws(() => inspectDirectory(root), /checksum/i)
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})
