import test from 'node:test'
import assert from 'node:assert/strict'
import { buildConnectionTree } from '../src/renderer/src/components/sidebar/connectionTreeModel'
import type { domain } from '../src/shared/contracts/domain'
const directory = (id: string, parentID = '') => ({
  directory: { id, parentID, name: id, sortOrder: 0, createdAt: '', updatedAt: '' },
  depth: parentID ? 1 : 0
})
const host = (id: string, directoryID = '', extra: Partial<domain.Host> = {}): domain.Host => ({
  id,
  directoryID,
  name: id,
  alias: id,
  hostName: `${id}.example.com`,
  user: 'root',
  port: 22,
  identityFile: '',
  passwordAuth: false,
  proxyJump: '',
  jumpHostID: '',
  forwardAgent: false,
  compression: false,
  serverAliveInterval: 0,
  serverAliveCountMax: 0,
  connectTimeout: 0,
  remark: '',
  lastUsedAt: '',
  sortOrders: {},
  createdAt: '',
  updatedAt: '',
  ...extra
})
const directories = [directory('prod'), directory('child', 'prod'), directory('empty')]
const hosts = [host('root'), host('nested', 'child'), host('direct', 'prod')]
test('connections and nested folders share a tree, including root connections and empty folders', () => {
  assert.deepEqual(
    buildConnectionTree(directories, hosts, new Set(), '').map((n) => [n.id, n.depth]),
    [
      ['prod', 0],
      ['child', 1],
      ['nested', 2],
      ['direct', 1],
      ['empty', 0],
      ['root', 0]
    ]
  )
  assert.deepEqual(
    buildConnectionTree(directories, hosts, new Set(['prod']), '').map((n) => n.id),
    ['prod', 'empty', 'root']
  )
})
test('search reveals matching descendants and ancestors without changing disclosure state', () => {
  const collapsed = new Set(['prod'])
  assert.deepEqual(
    buildConnectionTree(directories, hosts, collapsed, 'nested').map((n) => n.id),
    ['prod', 'child', 'nested']
  )
  assert.deepEqual([...collapsed], ['prod'])
  assert.deepEqual(buildConnectionTree(directories, hosts, collapsed, 'missing'), [])
})
test('the complete tree includes every connection and honors directory sort order', () => {
  const items = [
    host('a', 'prod', { sortOrders: { 'directory:prod': 2 } }),
    host('z', 'prod', { sortOrders: { 'directory:prod': 1 } }),
    host('recent', 'child', { lastUsedAt: '2026-01-01T00:00:00Z' })
  ]
  assert.deepEqual(
    buildConnectionTree(directories, items, new Set(), '').map((n) => n.id),
    ['prod', 'child', 'recent', 'z', 'a', 'empty']
  )
})

test('folder search includes matching empty folders and the whole matching subtree', () => {
  const collapsed = new Set(['prod'])
  assert.deepEqual(
    buildConnectionTree(directories, hosts, collapsed, 'empty').map((n) => n.id),
    ['empty']
  )
  assert.deepEqual(
    buildConnectionTree(directories, hosts, collapsed, 'PROD').map((n) => n.id),
    ['prod', 'child', 'nested', 'direct']
  )
  assert.deepEqual(
    buildConnectionTree(directories, hosts, collapsed, 'child').map((n) => n.id),
    ['prod', 'child', 'nested']
  )
  assert.deepEqual([...collapsed], ['prod'])
})
