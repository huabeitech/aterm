import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Terminal } from '@xterm/xterm'
import {
  composingKey,
  deferLinuxIMEKey,
  installWindowsIMEWorkaround,
  wordNavigationSequence
} from '../src/renderer/src/features/terminal/terminalKeyboard'

const key = (overrides: Partial<KeyboardEvent> = {}) =>
  ({
    type: 'keydown',
    key: 'ArrowLeft',
    code: 'ArrowLeft',
    keyCode: 37,
    altKey: true,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    isComposing: false,
    ...overrides
  }) as KeyboardEvent

test('Word movement respects the platform, alternate screen, native mode and AltGr', () => {
  assert.equal(wordNavigationSequence(key(), 'shell', 'darwin', false), '\x1bb')
  assert.equal(
    wordNavigationSequence(key({ key: 'ArrowRight' }), 'shell', 'darwin', false),
    '\x1bf'
  )
  for (const platform of ['win32', 'linux'])
    assert.equal(wordNavigationSequence(key(), 'shell', platform, false), '\x1b[1;5D')
  assert.equal(wordNavigationSequence(key(), 'terminal', 'darwin', false), null)
  assert.equal(wordNavigationSequence(key(), 'shell', 'darwin', true), null)
  for (const event of [
    key({ ctrlKey: true }),
    key({ isComposing: true }),
    key({ keyCode: 229 }),
    key({ shiftKey: true }),
    key({ type: 'keyup' })
  ])
    assert.equal(wordNavigationSequence(event, 'shell', 'darwin', false), null)
})

test('Fcitx punctuation gets browser input processing without intercepting shortcuts or AltGr', () => {
  const punctuation = key({ key: ';', code: 'Semicolon', keyCode: 186, altKey: false })
  assert.equal(deferLinuxIMEKey(punctuation, 'linux'), true)
  assert.equal(deferLinuxIMEKey(punctuation, 'darwin'), false)
  assert.equal(deferLinuxIMEKey(key({ ...punctuation, ctrlKey: true }), 'linux'), false)
  assert.equal(
    deferLinuxIMEKey(key({ ...punctuation, ctrlKey: true, altKey: true }), 'linux'),
    false
  )
  assert.equal(composingKey(key({ isComposing: true })), true)
  assert.equal(composingKey(key({ keyCode: 229 })), true)
})

test('Windows Shift commit preserves xterm input fallback state and restores the original handler', () => {
  const core = {
    _keyDownSeen: false,
    _keyDown(_event: KeyboardEvent) {
      this._keyDownSeen = true
      return 123
    }
  }
  const original = core._keyDown
  const term = { _core: core } as unknown as Pick<Terminal, 'options'>
  const restore = installWindowsIMEWorkaround(term, 'win32')
  assert.equal(core._keyDown(key({ key: 'Shift', keyCode: 16 })), 123)
  assert.equal(core._keyDownSeen, false)
  // A prior real keydown must remain remembered when Shift commits preedit.
  core._keyDownSeen = true
  core._keyDown(key({ key: 'Shift', keyCode: 16 }))
  assert.equal(core._keyDownSeen, true)
  core._keyDownSeen = false
  core._keyDown(key({ key: 'a', keyCode: 65 }))
  assert.equal(core._keyDownSeen, true)
  restore()
  assert.equal(core._keyDown, original)
})
