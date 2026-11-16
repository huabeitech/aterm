import assert from 'node:assert/strict'
import { test } from 'node:test'
import { LogBuffer, logLimits, logSnapshot } from '../../plugins/docker/src/logs'

test('Docker log byte limit keeps complete UTF-8 characters at the cutoff', () => {
  const text = '😀' + '界'.repeat(logLimits.bytes / 3 - 1) + 'a'
  const result = logSnapshot(text)
  assert.equal(result.truncated, true)
  assert.equal(result.text, text.slice(2))
  assert.equal(Buffer.byteLength(result.text, 'utf8'), logLimits.bytes - 2)
})

test('Docker log batches preserve partial lines until a producer gap', () => {
  const logs = new LogBuffer()
  logs.append('first\npar')
  assert.deepEqual(logs.append('tial\n'), { text: 'first\npartial\n', truncated: false })
  assert.deepEqual(logs.append('after gap\n', true), { text: 'after gap\n', truncated: true })
})
