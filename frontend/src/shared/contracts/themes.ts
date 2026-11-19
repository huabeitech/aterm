import { isExternalThemeID } from './themePackages'
export const themeIDs = [
  'system',
  'dark',
  'light',
  'light-dark-terminal',
  'solarized-light',
  'solarized-dark',
  'gruvbox-dark',
  'dracula'
] as const
export type ThemeID = (typeof themeIDs)[number] | `${string}/${string}`
export type ResolvedAppearance = 'light' | 'dark'
export type NativeThemeSource = 'system' | 'light' | 'dark'

export function isThemeID(value: unknown): value is ThemeID {
  return (
    isExternalThemeID(value) ||
    (typeof value === 'string' && (themeIDs as readonly string[]).includes(value))
  )
}
