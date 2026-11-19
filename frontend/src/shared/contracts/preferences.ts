import { appError } from '../errors'
import { isLanguagePreference } from './locale'
import type { domain } from './domain'
import { isThemeID, type ThemeID } from './themes'
export type ThemePreference = ThemeID
export type Preferences = domain.AppSettings
export const appFontScaleOptions = [90, 100, 110, 120, 130] as const
export const defaultPreferences: Preferences = {
  language: 'system',
  theme: 'system',
  appFontScale: 100,
  lightTheme: 'light',
  darkTheme: 'dark',
  defaultHome: 'hosts',
  terminalFontSize: 13,
  terminalWordNavigation: 'shell',
  commandPaletteShortcut: 'meta+p',
  commandSearchShortcut: 'meta+shift+p',
  toggleSidebarShortcut: 'meta+b',
  toggleInspectorShortcut: 'meta+shift+b',
  toggleTerminalFilesShortcut: 'meta+shift+e',
  newTerminalShortcut: 'meta+t',
  closeTerminalShortcut: 'meta+w',
  previousTerminalShortcut: 'meta+arrowleft',
  nextTerminalShortcut: 'meta+arrowright',
  moveTabLeftShortcut: 'meta+shift+arrowleft',
  moveTabRightShortcut: 'meta+shift+arrowright',
  splitVerticalShortcut: 'meta+d',
  splitHorizontalShortcut: 'meta+shift+d',
  sshDirectoryPath: '',
  sshDirectoryBookmark: '',
  sshDirectoryBookmarkCreatedAt: '',
  syncDirectoryPath: '',
  syncDirectoryBookmark: '',
  syncDirectoryBookmarkCreatedAt: ''
}
export const shortcutFields = [
  'commandPaletteShortcut',
  'commandSearchShortcut',
  'toggleSidebarShortcut',
  'toggleTerminalFilesShortcut',
  'newTerminalShortcut',
  'closeTerminalShortcut',
  'previousTerminalShortcut',
  'nextTerminalShortcut',
  'moveTabLeftShortcut',
  'moveTabRightShortcut',
  'splitVerticalShortcut',
  'splitHorizontalShortcut',
  'toggleInspectorShortcut'
] as const
export function platformDefaults(platform: string): Preferences {
  const result = { ...defaultPreferences }
  if (platform !== 'darwin') {
    for (const key of shortcutFields) result[key] = result[key].replace('meta+', 'ctrl+')
    result.toggleInspectorShortcut = 'ctrl+alt+b'
    result.toggleSidebarShortcut = 'ctrl+shift+b'
  }
  return result
}
// Older preferences may already assign the new default to another action.
export function defaultSidebarShortcut(input: Partial<Preferences>, platform?: string, field: 'toggleSidebarShortcut' | 'toggleInspectorShortcut' = 'toggleSidebarShortcut'): string {
  const mod = platform
    ? platform === 'darwin'
      ? 'meta'
      : 'ctrl'
    : input.newTerminalShortcut?.includes('ctrl+')
      ? 'ctrl'
      : 'meta'
  const occupied = new Set(
    shortcutFields
      .filter((key) => key !== field)
      .map((key) => input[key])
      .filter((value): value is string => typeof value === 'string')
      .map(normalizeShortcut)
  )
  const candidates = [
    ...(field === 'toggleSidebarShortcut' && mod === 'meta' ? ['meta+b'] : []),
    ...(field === 'toggleSidebarShortcut' || mod === 'meta' ? [`${mod}+shift+b`] : []),
    `${mod}+alt+b`,
    `${mod}+alt+shift+b`,
    ...Array.from({ length: 24 }, (_, index) => `${mod}+alt+f${index + 1}`)
  ]
  return candidates.find((value) => !occupied.has(value))!
}

// Keep existing custom bindings when introducing the terminal/files command.
export function defaultTerminalFilesShortcut(
  input: Partial<Preferences>,
  platform?: string
): string {
  const mod = platform
    ? platform === 'darwin'
      ? 'meta'
      : 'ctrl'
    : input.newTerminalShortcut?.includes('ctrl+')
      ? 'ctrl'
      : 'meta'
  const occupied = new Set(
    shortcutFields.filter((key) => key !== 'toggleTerminalFilesShortcut').map((key) => input[key])
  )
  return [
    `${mod}+shift+e`,
    `${mod}+alt+e`,
    `${mod}+alt+shift+e`,
    ...Array.from({ length: 24 }, (_, i) => `${mod}+alt+f${i + 1}`)
  ].find((key) => !occupied.has(key))!
}

export function normalizeShortcut(value: unknown): string {
  if (typeof value !== 'string') throw appError('invalidShortcut')
  const parts = value
    .toLowerCase()
    .trim()
    .split('+')
    .map((part) => part.trim())
  const modifiers = ['meta', 'ctrl', 'alt', 'shift']
  const keys = parts.filter((part) => !modifiers.includes(part))
  if (
    new Set(parts).size !== parts.length ||
    keys.length !== 1 ||
    !(
      /^[a-z0-9]$/.test(keys[0]) ||
      /^f([1-9]|1\d|2[0-4])$/.test(keys[0]) ||
      ['arrowleft', 'arrowright'].includes(keys[0])
    ) ||
    (!parts.includes('meta') && !parts.includes('ctrl'))
  )
    throw appError('shortcutMustIncludeCommandOrControlAndASupportedKey')
  const normalized = [...modifiers.filter((modifier) => parts.includes(modifier)), keys[0]].join(
    '+'
  )
  if (['meta+q', 'ctrl+q', 'meta+shift+h', 'ctrl+shift+h'].includes(normalized))
    throw appError('thisShortcutIsReservedByTheApplication')
  return normalized
}
export function validatePreferences(value: unknown): Preferences {
  if (!value || typeof value !== 'object') throw appError('invalidPreferences')
  const input = value as Preferences
  const language = input.language ?? 'system'
  if (!isLanguagePreference(language)) throw appError('invalidLanguage')
  const appFontScale = input.appFontScale === undefined ? 100 : input.appFontScale
  if (!(appFontScaleOptions as readonly number[]).includes(appFontScale))
    throw appError('invalidAppTextSize')
  if (!isThemeID(input.theme)) throw appError('invalidTheme')
  if (!['hosts', 'local-terminal'].includes(input.defaultHome)) throw appError('invalidDefaultHome')
  const terminalWordNavigation = input.terminalWordNavigation ?? 'shell'
  if (!['shell', 'terminal'].includes(terminalWordNavigation))
    throw appError('invalidTerminalWordNavigation')
  if (
    !Number.isInteger(input.terminalFontSize) ||
    input.terminalFontSize < 10 ||
    input.terminalFontSize > 24
  )
    throw appError('terminalFontSizeMustBeBetween10And24')
  if (
    input.lightTheme !== undefined &&
    (!isThemeID(input.lightTheme) || input.lightTheme === 'system')
  )
    throw appError('invalidTheme')
  if (
    input.darkTheme !== undefined &&
    (!isThemeID(input.darkTheme) || input.darkTheme === 'system')
  )
    throw appError('invalidTheme')
  const result = {
    ...defaultPreferences,
    language,
    theme: input.theme,
    lightTheme: input.lightTheme ?? 'light',
    darkTheme: input.darkTheme ?? 'dark',
    appFontScale,
    defaultHome: input.defaultHome,
    terminalFontSize: input.terminalFontSize,
    terminalWordNavigation
  }
  const seen = new Set<string>()
  for (const key of shortcutFields) {
    result[key] = normalizeShortcut(
      input[key] ??
        (key === 'commandSearchShortcut'
          ? input.commandPaletteShortcut?.includes('ctrl+')
            ? 'ctrl+shift+p'
            : 'meta+shift+p'
          : key === 'toggleTerminalFilesShortcut'
            ? defaultTerminalFilesShortcut(input)
            : key === 'toggleSidebarShortcut'
              ? defaultSidebarShortcut(input)
              : key === 'toggleInspectorShortcut'
                ? defaultSidebarShortcut(result, undefined, key)
                : undefined)
    )
    if (seen.has(result[key])) throw appError('keyboardShortcutsMustBeUnique')
    seen.add(result[key])
  }
  return result
}
