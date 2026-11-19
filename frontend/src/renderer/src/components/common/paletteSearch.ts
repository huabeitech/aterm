import type { ThemePreference } from '../../../../shared/contracts/preferences'

export type PaletteCommand = {
  id: string
  label: string
  keywords?: string
  shortcut?: string
  detail?: string
  previewTheme?: ThemePreference
  choices?: { title: string; items: PaletteCommand[] }
  run: () => void
}

export function paletteMode(query: string): 'commands' | 'hosts' {
  return query.startsWith('>') ? 'commands' : 'hosts'
}

export function searchPaletteCommands(
  commands: PaletteCommand[],
  query: string,
  recent: string[]
): PaletteCommand[] {
  const tokens = query.toLowerCase().trim().split(/\s+/).filter(Boolean)
  return commands
    .map((command, index) => {
      const label = command.label.toLowerCase()
      const text = `${label} ${command.keywords ?? ''}`.toLowerCase()
      let score = 0
      for (const token of tokens) {
        if (label === token) score += 1000
        else if (label.includes(token)) score += 500
        else if (text.includes(token)) score += 200
        else {
          let from = 0
          for (const char of token) {
            const match = text.indexOf(char, from)
            if (match < 0) return null
            from = match + 1
          }
          score += 50
        }
      }
      const recentIndex = recent.indexOf(command.id)
      if (recentIndex >= 0) score += 20 / (recentIndex + 1)
      return { command, score, index }
    })
    .filter((item): item is NonNullable<typeof item> => item !== null)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map(({ command }) => command)
}
