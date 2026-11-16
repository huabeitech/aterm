import assert from 'node:assert/strict'
import { test } from 'node:test'
import { removeSelectedImages } from '../../plugins/docker/src/ui/imageRemoval'

test('batch image deletion reports refusals and continues with the remaining IDs', async () => {
  const calls: string[] = []
  const result = await removeSelectedImages(['first', 'in-use', 'last', 'first'], async id => {
    calls.push(id)
    if (id === 'in-use') throw new Error('Image is in use')
  }, () => true)

  assert.deepEqual(calls, ['first', 'in-use', 'last'])
  assert.deepEqual(result, {
    deleted: ['first', 'last'],
    failed: [{ id: 'in-use', message: 'Image is in use' }]
  })
})

test('batch image deletion stops when the view leaves its engine', async () => {
  let current = true
  const calls: string[] = []
  const result = await removeSelectedImages(['first', 'last'], async id => {
    calls.push(id)
    current = false
  }, () => current)

  assert.deepEqual(calls, ['first'])
  assert.equal(result, undefined)
})
