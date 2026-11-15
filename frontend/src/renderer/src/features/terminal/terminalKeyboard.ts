import type { Terminal } from '@xterm/xterm'

type Key = Pick<
  KeyboardEvent,
  | 'type'
  | 'key'
  | 'code'
  | 'ctrlKey'
  | 'altKey'
  | 'metaKey'
  | 'shiftKey'
  | 'isComposing'
  | 'keyCode'
>
export const composingKey = (event: Pick<Key, 'isComposing' | 'keyCode'>) =>
  event.isComposing || event.keyCode === 229

export function wordNavigationSequence(
  event: Key,
  mode: 'shell' | 'terminal',
  platform: string,
  alternate: boolean
) {
  if (
    mode !== 'shell' ||
    alternate ||
    event.type !== 'keydown' ||
    composingKey(event) ||
    !event.altKey ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey ||
    (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight')
  )
    return null
  const left = event.key === 'ArrowLeft'
  return platform === 'darwin' ? (left ? '\x1bb' : '\x1bf') : left ? '\x1b[1;5D' : '\x1b[1;5C'
}

const linuxTextKeys = new Set([
  'Backquote',
  'Backslash',
  'BracketLeft',
  'BracketRight',
  'Comma',
  'Period',
  'Quote',
  'Semicolon',
  'Slash'
])
export function deferLinuxIMEKey(event: Key, platform: string) {
  return (
    platform === 'linux' &&
    event.type === 'keydown' &&
    !event.ctrlKey &&
    !event.altKey &&
    !event.metaKey &&
    (linuxTextKeys.has(event.code) || (event.code === 'Space' && event.shiftKey))
  )
}

// xterm 6.0 arms its input fallback guard for Shift, which can discard a
// Sogou commit when Shift switches to English (upstream xterm.js#6054).
// Keep this version-specific workaround at the keydown boundary, not keyup.
export function installWindowsIMEWorkaround(term: Pick<Terminal, 'options'>, platform: string) {
  if (platform !== 'win32') return () => {}
  const core = (
    term as unknown as {
      _core?: {
        _keyDownSeen: boolean
        _keyDown(event: KeyboardEvent): unknown
      }
    }
  )._core
  if (!core || typeof core._keyDown !== 'function' || typeof core._keyDownSeen !== 'boolean')
    return () => {}
  const original = core._keyDown
  const wrapped = (event: KeyboardEvent) => {
    const seen = core._keyDownSeen
    try {
      return original.call(core, event)
    } finally {
      if (event.key === 'Shift' || event.keyCode === 16) core._keyDownSeen = seen
    }
  }
  core._keyDown = wrapped
  return () => {
    if (core._keyDown === wrapped) core._keyDown = original
  }
}
