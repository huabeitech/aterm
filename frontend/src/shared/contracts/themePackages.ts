/** Stable, public color tokens. Internal CSS names are deliberately not part of the format. */
export const colorTokens = {
  'surface.window': '--app-window-background',
  'surface.content': '--mac-content',
  'surface.sidebar': '--mac-sidebar',
  'surface.panel': '--mac-panel',
  'surface.toolbar': '--mac-toolbar',
  'surface.control': '--mac-control',
  'text.primary': '--mac-text',
  'text.secondary': '--mac-secondary-text',
  'text.muted': '--mac-tertiary-text',
  'text.disabled': '--mac-disabled-text',
  'text.placeholder': '--mac-placeholder',
  'border.default': '--mac-separator',
  'border.control': '--mac-control-border',
  'interaction.accent': '--primary',
  'interaction.hover': '--mac-hover-background',
  'interaction.selection': '--mac-selection-background',
  'status.error': '--theme-files-error',
  'status.success': '--theme-files-success',
  'status.info': '--theme-files-info',
  'files.folder': '--theme-files-folder',
  'tabs.active': '--workspace-tab-active-background',
  'tabs.hover': '--workspace-tab-hover-background',
  'terminal.title': '--terminal-title-background',
  'terminal.border': '--terminal-border',
  'scrollbar.thumb': '--mac-scrollbar-thumb',
  'scrollbar.hover': '--mac-scrollbar-thumb-hover',
  'scrollbar.track': '--mac-scrollbar-track',
  'surface.workspace': '--workspace-background',
  'surface.hosts': '--hosts-background',
  'surface.inspector': '--inspector-background',
  'terminal.background': '--terminal-background',
  'terminal.titleHover': '--terminal-title-hover',
  'terminal.titleActive': '--terminal-title-active',
  'terminal.titleText': '--terminal-title-text',
  'terminal.titleSecondaryText': '--terminal-title-secondary-text',
  'terminal.findBackground': '--terminal-find-background',
  'terminal.findText': '--terminal-find-text',
  'terminal.findMutedText': '--terminal-find-muted-text',
  'terminal.findHover': '--terminal-find-hover'
} as const
export type ColorToken = keyof typeof colorTokens
export const ansiNames = [
  'black',
  'red',
  'green',
  'yellow',
  'blue',
  'magenta',
  'cyan',
  'white',
  'brightBlack',
  'brightRed',
  'brightGreen',
  'brightYellow',
  'brightBlue',
  'brightMagenta',
  'brightCyan',
  'brightWhite'
] as const
export interface ThemeDefinition {
  id: string
  name: string
  appearance: 'light' | 'dark'
  colors?: Partial<Record<ColorToken, string>>
  terminal?: {
    background: string
    foreground: string
    cursor?: string
    selectionBackground?: string
    ansi?: string[]
  }
}
export interface ThemePackage {
  schemaVersion: 1
  themeApiVersion: 1
  id: string
  name: string
  version: string
  author: string
  license?: string
  themes: ThemeDefinition[]
}
export interface ThemeCatalog {
  packages: ThemePackage[]
  warnings: string[]
}
export interface ThemesApi {
  list(): Promise<ThemeCatalog>
  import(): Promise<ThemeCatalog | null>
  duplicate(theme: string): Promise<ThemeCatalog>
  remove(id: string): Promise<ThemeCatalog>
  export(id: string): Promise<void>
  onChanged(callback: () => void): () => void
}
export function isExternalThemeID(value: unknown): value is `${string}/${string}` {
  return (
    typeof value === 'string' && /^[a-z0-9][a-z0-9.-]{0,95}\/[a-z0-9][a-z0-9-]{0,63}$/.test(value)
  )
}
const color = (value: unknown): value is string =>
  typeof value === 'string' && /^#(?:[0-9a-f]{6}|[0-9a-f]{8})$/i.test(value)
const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === 'object' && !Array.isArray(value)
export function validateThemePackage(value: unknown, builtIn = false): ThemePackage {
  const fail = (path: string): never => {
    throw new Error(`Invalid theme package: ${path}`)
  }
  if (!record(value)) return fail('expected an object')
  if (value.schemaVersion !== 1 || value.themeApiVersion !== 1)
    return fail('unsupported schemaVersion or themeApiVersion')
  if (
    typeof value.id !== 'string' ||
    !/^[a-z0-9][a-z0-9.-]{0,95}$/.test(value.id) ||
    (!builtIn && value.id.startsWith('aterm.'))
  )
    return fail('id')
  for (const key of ['name', 'author', 'version'])
    if (typeof value[key] !== 'string' || !value[key] || (value[key] as string).length > 200)
      fail(key)
  if (!/^\d+\.\d+\.\d+$/.test(value.version as string)) fail('version')
  if (
    value.license !== undefined &&
    (typeof value.license !== 'string' || value.license.length > 200)
  )
    fail('license')
  if (!Array.isArray(value.themes) || !value.themes.length || value.themes.length > 64)
    return fail('themes')
  const ids = new Set<string>()
  const themes = value.themes.map((entry, index): ThemeDefinition => {
    const path = `themes[${index}]`
    if (!record(entry)) return fail(path)
    if (
      typeof entry.id !== 'string' ||
      !/^[a-z0-9][a-z0-9-]{0,63}$/.test(entry.id) ||
      ids.has(entry.id)
    )
      return fail(`${path}.id`)
    ids.add(entry.id)
    if (typeof entry.name !== 'string' || !entry.name || entry.name.length > 200)
      return fail(`${path}.name`)
    if (entry.appearance !== 'dark' && entry.appearance !== 'light')
      return fail(`${path}.appearance`)
    const colors: ThemeDefinition['colors'] = {}
    if (entry.colors !== undefined) {
      if (!record(entry.colors)) return fail(`${path}.colors`)
      for (const [key, value] of Object.entries(entry.colors)) {
        if (!Object.hasOwn(colorTokens, key) || !color(value)) fail(`${path}.colors.${key}`)
        colors[key as ColorToken] = value as string
      }
    }
    let terminal: ThemeDefinition['terminal']
    if (entry.terminal !== undefined) {
      const input = entry.terminal
      if (!record(input)) return fail(`${path}.terminal`)
      for (const key of ['background', 'foreground'])
        if (!color(input[key]) || (input[key] as string).length !== 7)
          fail(`${path}.terminal.${key}`)
      for (const key of ['cursor', 'selectionBackground'])
        if (input[key] !== undefined && !color(input[key])) fail(`${path}.terminal.${key}`)
      if (
        input.ansi !== undefined &&
        (!Array.isArray(input.ansi) || input.ansi.length !== 16 || !input.ansi.every(color))
      )
        fail(`${path}.terminal.ansi`)
      terminal = {
        background: input.background as string,
        foreground: input.foreground as string,
        ...(input.cursor ? { cursor: input.cursor as string } : {}),
        ...(input.selectionBackground
          ? { selectionBackground: input.selectionBackground as string }
          : {}),
        ...(input.ansi ? { ansi: input.ansi as string[] } : {})
      }
    }
    if (!Object.keys(colors).length && !terminal)
      return fail(`${path}: colors or terminal required`)
    return {
      id: entry.id,
      name: entry.name,
      appearance: entry.appearance,
      colors,
      ...(terminal ? { terminal } : {})
    }
  })
  return {
    schemaVersion: 1,
    themeApiVersion: 1,
    id: value.id,
    name: value.name as string,
    version: value.version as string,
    author: value.author as string,
    ...(value.license ? { license: value.license as string } : {}),
    themes
  }
}
