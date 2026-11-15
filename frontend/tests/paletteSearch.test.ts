import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  paletteMode,
  searchPaletteCommands
} from '../src/renderer/src/components/common/paletteSearch'
const commands = [
  { id: 'ssh', label: 'SSH: Connect to Host…', keywords: '连接 主机', run() {} },
  { id: 'settings', label: 'Preferences: Settings', keywords: '设置', run() {} },
  { id: 'split', label: 'Terminal: Split Right', keywords: '分屏', run() {} }
]
test('only a leading > switches into command mode', () => {
  assert.equal(paletteMode('>'), 'commands')
  assert.equal(paletteMode('> settings'), 'commands')
  assert.equal(paletteMode('prod>'), 'hosts')
  assert.equal(paletteMode(''), 'hosts')
})
test('command search supports aliases, fuzzy matching and recent ranking below relevance', () => {
  assert.equal(searchPaletteCommands(commands, '设置', [])[0]?.id, 'settings')
  assert.equal(searchPaletteCommands(commands, 'ssh host', [])[0]?.id, 'ssh')
  assert.equal(searchPaletteCommands(commands, 'splt', [])[0]?.id, 'split')
  assert.equal(searchPaletteCommands(commands, '', ['split'])[0]?.id, 'split')
  assert.equal(searchPaletteCommands(commands, 'settings', ['ssh'])[0]?.id, 'settings')
  assert.deepEqual(searchPaletteCommands(commands, 'unmatched', []), [])
})
