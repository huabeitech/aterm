import { builtinThemePackage } from '../../../shared/contracts/builtinThemes'
import { useSyncExternalStore } from 'react'
import type { Preferences } from '../../../shared/contracts/preferences'
import type { ThemeCatalog, ThemeDefinition } from '../../../shared/contracts/themePackages'
let catalog: ThemeCatalog = { packages: [], warnings: [] }
let preferences: Preferences | undefined
let revision = 0
const listeners = new Set<() => void>()
export function themeDefinition(id: string): ThemeDefinition | undefined {
  if (!id.includes('/')) return builtinThemePackage.themes.find((theme) => theme.id === id)
  const [pkg, theme] = id.split('/')
  if (pkg === builtinThemePackage.id)
    return builtinThemePackage.themes.find((item) => item.id === theme)
  return catalog.packages.find((item) => item.id === pkg)?.themes.find((item) => item.id === theme)
}
export function themeCatalog() {
  return catalog
}
export function themePreferences() {
  return preferences
}
export function effectiveAppTheme(theme: string, appearance: 'light' | 'dark'): string {
  if (theme !== 'system') return theme
  return (appearance === 'light' ? preferences?.lightTheme : preferences?.darkTheme) ?? appearance
}
export function availableThemes() {
  return [builtinThemePackage, ...catalog.packages].flatMap((pkg) =>
    pkg.themes.map((theme) => ({
      ...theme,
      reference: pkg.id === builtinThemePackage.id ? theme.id : `${pkg.id}/${theme.id}`,
      packageID: pkg.id,
      packageName: pkg.name,
      version: pkg.version,
      author: pkg.author,
      builtIn: pkg.id === builtinThemePackage.id
    }))
  )
}
export function useThemeRevision() {
  return useSyncExternalStore(
    (callback) => {
      listeners.add(callback)
      return () => {
        listeners.delete(callback)
      }
    },
    () => revision
  )
}
let request = 0
export async function refreshThemes() {
  const serial = ++request
  const [next, boot] = await Promise.all([window.desktop.themes.list(), window.desktop.bootstrap()])
  if (serial !== request) return
  catalog = next
  preferences = boot.preferences
  revision++
  for (const listener of listeners) listener()
  window.dispatchEvent(new Event('aterm:themes-changed'))
}
export async function initializeThemes() {
  window.desktop.themes.onChanged(() => {
    void refreshThemes().catch(console.error)
  })
  await refreshThemes()
}
