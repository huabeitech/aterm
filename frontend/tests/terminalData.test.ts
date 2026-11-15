import { test } from 'node:test'
import assert from 'node:assert/strict'
import { terminalBinaryInput, terminalSize, terminalTextChunks } from '../src/shared/terminalData'

test('Independent UTF-8 encoding preserves supplementary characters at every input boundary', () => {
  for (const prefix of [16382, 16383, 16384, 32767]) {
    const text = 'a'.repeat(prefix) + '😀𠮷e\u0301' + 'b'.repeat(20000)
    const chunks = [...terminalTextChunks(text)]
    assert.ok(chunks.every((chunk) => chunk.length <= 16384))
    assert.equal(Buffer.concat(chunks.map((chunk) => Buffer.from(chunk))).toString('utf8'), text)
  }
})

test('PTY geometry is clamped identically at both grid limits', () => {
  assert.deepEqual(terminalSize(900, 450), { cols: 500, rows: 300 })
  assert.deepEqual(terminalSize(1, 0), { cols: 2, rows: 1 })
  assert.deepEqual(terminalSize(132, 43), { cols: 132, rows: 43 })
})

test('Binary input preserves high bytes and rejects malformed or oversized payloads', () => {
  const bytes = Uint8Array.from([0x1b, 0x5b, 0x4d, 0x20, 0x80, 0xff])
  assert.equal(Buffer.from(terminalBinaryInput(bytes)).toString('hex'), '1b5b4d2080ff')
  for (const value of ['\x80', [128], new Uint8Array(65537), new Uint16Array(6), null])
    assert.throws(() => terminalBinaryInput(value), /Invalid terminal binary input/)
})
