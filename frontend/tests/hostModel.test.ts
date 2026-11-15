import assert from 'node:assert/strict'
import test from 'node:test'
import { validateForm } from '../src/renderer/src/features/hosts/model/hostModel'
import type { FormState } from '../src/renderer/src/components/common/types'

const form = (overrides: Partial<FormState> = {}): FormState => ({
  directoryID: '',
  alias: 'production',
  hostName: 'server.example',
  user: 'deploy',
  port: '22',
  identityFile: '',
  keySource: 'system',
  passwordAuth: false,
  password: '',
  passwordState: 'none',
  proxyJump: '',
  jumpHostID: '',
  forwardAgent: false,
  compression: false,
  serverAliveInterval: '',
  serverAliveCountMax: '',
  connectTimeout: '',
  remark: '',
  favorite: false,
  ...overrides
})

test('requires a managed SSH key when that source is selected', () => {
  assert.equal(
    validateForm(form({ keySource: 'managed' })).identityFile,
    'Managed SSH Key is required.'
  )
  assert.equal(
    validateForm(form({ keySource: 'managed', identityFile: 'aterm-key://managed-key-id' }))
      .identityFile,
    undefined
  )
})

test('directory scope and counts include all descendants without leaking siblings or uncategorized hosts', async () => {
  const { directoryScopeIDs } = await import('../src/shared/directoryScope')
  const { buildDirectoryHostCounts } =
    await import('../src/renderer/src/features/directories/model/directoryModel')
  const directories = [
    { id: 'leaf', parentID: 'child' },
    { id: 'other', parentID: '' },
    { id: 'parent', parentID: '' },
    { id: 'child', parentID: 'parent' }
  ]
  assert.deepEqual([...directoryScopeIDs(directories, 'parent')].sort(), [
    'child',
    'leaf',
    'parent'
  ])
  const counts = buildDirectoryHostCounts(
    directories as import('../src/shared/contracts/domain').domain.Directory[],
    ['parent', 'child', 'leaf', 'leaf', 'other', ''].map((directoryID) => ({
      directoryID
    })) as import('../src/shared/contracts/domain').domain.Host[]
  )
  assert.equal(counts.get('parent'), 4)
  assert.equal(counts.get('child'), 3)
  assert.equal(counts.get('leaf'), 2)
  assert.equal(counts.get('other'), 1)
  assert.equal(
    directoryScopeIDs(
      [
        { id: 'a', parentID: 'b' },
        { id: 'b', parentID: 'a' }
      ],
      'a'
    ).size,
    2
  )
})

test('host aliases are required and use the exportable character set', () => {
  assert.equal(validateForm(form({ alias: '  ' })).alias, 'Host Alias is required.')
  for (const alias of ['prod-api', 'dev_01', 'api.example', 'Prod2', ' prod-api ']) {
    assert.equal(validateForm(form({ alias })).alias, undefined, alias)
  }
  for (const alias of ['prod api', 'prod\tapi', '生产', '*', 'api?', '!api', 'a#b', 'a"b', 'a/b']) {
    assert.equal(
      validateForm(form({ alias })).alias,
      'Host Alias can only contain letters, numbers, dots, underscores, or hyphens.',
      alias
    )
  }
})
