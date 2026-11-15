const modifierOrder = ['meta', 'ctrl', 'alt', 'shift'] as const
const modifierKeys = new Set(['meta', 'control', 'alt', 'shift'])

export const defaultCommandPaletteShortcut = 'meta+p'

export function shortcutFromKeyboardEvent(
  event: KeyboardEvent | ReactKeyboardEvent<HTMLInputElement>
): string | null {
  const key = event.key.toLowerCase()
  if (modifierKeys.has(key) || !isShortcutKey(key) || (!event.metaKey && !event.ctrlKey))
    return null

  const modifiers = {
    meta: event.metaKey,
    ctrl: event.ctrlKey,
    alt: event.altKey,
    shift: event.shiftKey
  }
  return [...modifierOrder.filter((modifier) => modifiers[modifier]), key].join('+')
}

export function shortcutMatchesEvent(shortcut: string | undefined, event: KeyboardEvent): boolean {
  const current = shortcutFromKeyboardEvent(event)
  return current !== null && current === (shortcut || defaultCommandPaletteShortcut)
}

export function formatCommandPaletteShortcut(shortcut: string | undefined): string {
  const parts = (shortcut || defaultCommandPaletteShortcut).split('+')
  const labels: Record<string, string> = {
    meta: '⌘',
    ctrl: '⌃',
    alt: '⌥',
    shift: '⇧'
  }
  const keys: Record<string, string> = {
    arrowleft: '←',
    arrowright: '→'
  }
  return parts.map((part) => labels[part] ?? keys[part] ?? part.toUpperCase()).join('')
}

function isShortcutKey(key: string) {
  return /^[a-z0-9]$/.test(key) || /^f\d+$/.test(key) || key === 'arrowleft' || key === 'arrowright'
}
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
