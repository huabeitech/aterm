import { useThemeRevision, effectiveAppTheme } from './themeRegistry'
import { colorTokens } from '../../../shared/contracts/themePackages'
import { useLayoutEffect } from 'react'
import type { ResolvedAppearance, ThemeID } from '../../../shared/contracts/themes'
import { getAppTheme } from './themes'

const aliases: Record<string, string> = {
  '--foreground': '--mac-text',
  '--muted-foreground': '--mac-secondary-text',
  '--background': '--mac-content',
  '--muted': '--mac-panel',
  '--secondary': '--mac-control',
  '--input': '--mac-control-border',
  '--ring': '--primary',
  '--border': '--mac-separator',
  '--accent': '--mac-hover-background'
}

const terminalAliases: Record<string, string> = {
  '--secondary-foreground': '--mac-text',
  '--accent-foreground': '--mac-text',
  '--card': '--mac-panel',
  '--card-foreground': '--mac-text',
  '--popover': '--mac-panel',
  '--popover-foreground': '--mac-text'
}

// One owner applies the effective theme at document scope so menus, dialogs and
// tooltips in portals share the workspace appearance. Saved preferences stay intact.
export function AppAppearance({
  theme,
  appearance
}: {
  theme: ThemeID
  appearance: ResolvedAppearance
}) {
  const revision = useThemeRevision()
  useLayoutEffect(() => {
    const resolved = getAppTheme(theme, appearance)
    const root = document.documentElement
    root.classList.toggle('dark', resolved.appearance === 'dark')
    root.dataset.theme = resolved.appearance
    root.dataset.customTheme = 'true'
    root.dataset.themeId = effectiveAppTheme(theme, appearance)
    root.dataset.workspaceTheme = 'unified'
    for (const key of Object.values(colorTokens)) root.style.removeProperty(key)
    for (const [key, value] of Object.entries(resolved.cssVariables))
      root.style.setProperty(key, value)
    for (const [key, source] of Object.entries({
      '--theme-tab-text': '--mac-text',
      '--theme-tab-secondary': '--mac-secondary-text',
      '--theme-tab-active': '--workspace-tab-active-background',
      '--theme-tab-hover': '--workspace-tab-hover-background',
      '--theme-tab-border': '--mac-separator'
    }))
      root.style.setProperty(key, resolved.cssVariables[source as `--${string}`])
    for (const [key, token] of Object.entries(aliases)) root.style.setProperty(key, `var(${token})`)
    for (const [key, token] of Object.entries(terminalAliases)) {
      root.style.setProperty(key, `var(${token})`)
    }
    root.dataset.themeRevision = String(revision)
  }, [theme, appearance, revision])
  return null
}
