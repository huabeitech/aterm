import { builtinThemePackage } from './builtinThemes'
import { colorTokens, ansiNames, type ThemeDefinition } from './themePackages'
import type { ResolvedAppearance } from './themes'
export interface ResolvedTheme {
  id: string
  label: string
  appearance: ResolvedAppearance
  cssVariables: Record<`--${string}`, string>
  terminal: Record<string, string | undefined>
}
export const appThemes = Object.fromEntries(
  (['light', 'dark'] as const).map((appearance) => {
    const definition = builtinThemePackage.themes.find((theme) => theme.id === appearance)!
    return [
      appearance,
      {
        id: appearance,
        label: definition.name,
        appearance,
        cssVariables: Object.fromEntries(
          Object.entries(definition.colors ?? {}).map(([token, value]) => [
            colorTokens[token as keyof typeof colorTokens],
            value
          ])
        )
      }
    ]
  })
) as Record<ResolvedAppearance, Pick<ResolvedTheme, 'id' | 'label' | 'appearance' | 'cssVariables'>>

export function resolveTheme(
  definition: ThemeDefinition | undefined,
  systemAppearance: ResolvedAppearance
): ResolvedTheme {
  definition ??= builtinThemePackage.themes.find((theme) => theme.id === systemAppearance)!
  const selected = definition.id
  const appearance = definition.appearance
  const base = appThemes[appearance]
  const cssVariables = { ...base.cssVariables }
  const colors = definition.colors ?? {}
  const palette = definition.terminal
  const fallback = builtinThemePackage.themes.find((theme) => theme.id === appearance)!.terminal!
  const background = colors['surface.window'] ?? cssVariables['--app-window-background']
  const foreground = colors['text.primary'] ?? cssVariables['--mac-text']
  const terminalBackground = palette?.background ?? colors['terminal.background'] ?? fallback.background
  const terminalForeground = palette?.foreground ?? fallback.foreground
  const terminalMix = (amount: number) =>
    `color-mix(in srgb, ${terminalForeground} ${amount}%, ${terminalBackground})`
  const mix = (amount: number) => `color-mix(in srgb, ${foreground} ${amount}%, ${background})`
  Object.assign(cssVariables, {
    '--mac-text': foreground,
    '--mac-secondary-text': mix(72),
    '--mac-tertiary-text': mix(60),
    '--mac-placeholder': mix(72),
    '--mac-disabled-text': mix(35),
    '--mac-panel': mix(4),
    '--mac-sidebar': mix(3),
    '--mac-toolbar': mix(4),
    '--mac-control': mix(6),
    '--mac-control-border': mix(18),
    '--mac-separator': mix(12),
    '--mac-hover-background': mix(8),
    '--mac-selection-background': mix(18),
    '--workspace-tab-active-background': mix(12),
    '--workspace-tab-hover-background': mix(8),
    '--terminal-title-background': terminalMix(4),
    '--terminal-title-text': terminalForeground,
    '--terminal-title-secondary-text': terminalMix(72),
    '--terminal-title-hover': terminalMix(8),
    '--terminal-title-active': terminalMix(12),
    '--terminal-border': terminalMix(12),
    '--terminal-find-background': terminalMix(4),
    '--terminal-find-text': terminalForeground,
    '--terminal-find-muted-text': terminalMix(72),
    '--terminal-find-hover': terminalMix(8)
  })
  cssVariables['--app-window-background'] = background
  for (const key of [
    '--mac-content',
    '--workspace-background',
    '--hosts-background',
    '--inspector-background'
  ])
    cssVariables[key as `--${string}`] = background
  for (const [key, value] of Object.entries(colors))
    cssVariables[colorTokens[key as keyof typeof colorTokens]] = value!
  cssVariables['--terminal-background'] = terminalBackground
  const terminal = {
    background: terminalBackground,
    foreground: terminalForeground,
    cursor: palette?.cursor ?? terminalForeground,
    cursorAccent: terminalBackground,
    selectionBackground: palette?.selectionBackground ?? fallback.selectionBackground,
    ...Object.fromEntries(
      ansiNames.map((name, index) => [name, palette?.ansi?.[index] ?? fallback.ansi![index]])
    ),
    overviewRulerBorder: '#00000000'
  }
  return { id: selected, label: definition.name, appearance, cssVariables, terminal }
}
