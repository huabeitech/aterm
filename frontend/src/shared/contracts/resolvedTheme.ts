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
  const background =
    colors['surface.window'] ??
    definition.terminal?.background ??
    cssVariables['--app-window-background']
  const foreground =
    colors['text.primary'] ?? definition.terminal?.foreground ?? cssVariables['--mac-text']
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
    '--terminal-title-background': mix(4),
    '--terminal-title-text': foreground,
    '--terminal-title-secondary-text': mix(72),
    '--terminal-title-hover': mix(8),
    '--terminal-title-active': mix(12),
    '--terminal-border': mix(12),
    '--terminal-find-background': mix(4),
    '--terminal-find-text': foreground,
    '--terminal-find-muted-text': mix(72),
    '--terminal-find-hover': mix(8)
  })
  cssVariables['--app-window-background'] = background
  for (const key of [
    '--mac-content',
    '--workspace-background',
    '--hosts-background',
    '--inspector-background',
    '--terminal-background'
  ])
    cssVariables[key as `--${string}`] = background
  for (const [key, value] of Object.entries(colors))
    cssVariables[colorTokens[key as keyof typeof colorTokens]] = value!
  // A legacy component token must not reintroduce an independently themed terminal.
  cssVariables['--terminal-background'] = background
  const fallback = builtinThemePackage.themes.find((theme) => theme.id === appearance)!.terminal!
  const palette = definition.terminal
  const terminal = {
    background,
    foreground,
    cursor: palette?.cursor ?? foreground,
    cursorAccent: background,
    selectionBackground: palette?.selectionBackground ?? fallback.selectionBackground,
    ...Object.fromEntries(
      ansiNames.map((name, index) => [name, palette?.ansi?.[index] ?? fallback.ansi![index]])
    ),
    overviewRulerBorder: '#00000000'
  }
  return { id: selected, label: definition.name, appearance, cssVariables, terminal }
}
