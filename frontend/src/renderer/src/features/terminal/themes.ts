import { themeDefinition } from '../../app/themeRegistry'
import { resolveTheme } from '../../../../shared/contracts/resolvedTheme'
import type { ITheme } from '@xterm/xterm'
/** Receives the same resolved selection as AppAppearance. */
export function terminalTheme(id: string): ITheme {
  const appearance = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'
  return resolveTheme(themeDefinition(id), appearance).terminal
}
