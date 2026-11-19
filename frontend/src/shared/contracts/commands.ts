import { t } from '../i18n'
import type { Preferences } from './preferences'
export type Command =
  | 'search'
  | 'command-palette'
  | 'find'
  | 'new-terminal'
  | 'close-tab'
  | 'previous-tab'
  | 'next-tab'
  | 'move-left'
  | 'move-right'
  | 'toggle-sidebar'
  | 'toggle-inspector'
  | 'next-connection-view'
  | 'hosts'
  | 'settings'
  | 'split-vertical'
  | 'split-horizontal'
  | `tab-${1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9}`
export function getCommandDefinitions(settings: Preferences, platform: string) {
  const mod = platform === 'darwin' ? 'meta' : 'ctrl'
  return [
    {
      id: 'search',
      labelKey: 'native:searchHosts' as const,
      shortcut: settings.commandPaletteShortcut
    },
    {
      id: 'command-palette',
      labelKey: 'native:commandPalette' as const,
      shortcut: settings.commandSearchShortcut
    },
    { id: 'find', labelKey: 'commands:find' as const, shortcut: `${mod}+f` },
    {
      id: 'new-terminal',
      labelKey: 'commands:newLocalShell' as const,
      shortcut: settings.newTerminalShortcut
    },
    {
      id: 'close-tab',
      labelKey: 'commands:closeWorkspaceTab' as const,
      shortcut: settings.closeTerminalShortcut
    },
    {
      id: 'previous-tab',
      labelKey: 'commands:previousWorkspaceTab' as const,
      shortcut: settings.previousTerminalShortcut
    },
    {
      id: 'next-tab',
      labelKey: 'commands:nextWorkspaceTab' as const,
      shortcut: settings.nextTerminalShortcut
    },
    {
      id: 'move-left',
      labelKey: 'commands:moveCurrentTabLeft' as const,
      shortcut: settings.moveTabLeftShortcut
    },
    {
      id: 'move-right',
      labelKey: 'commands:moveCurrentTabRight' as const,
      shortcut: settings.moveTabRightShortcut
    },
    {
      id: 'toggle-sidebar',
      labelKey: 'hosts:toggleSidebar' as const,
      shortcut: settings.toggleSidebarShortcut
    },
    {
      id: 'toggle-inspector',
      labelKey: 'hosts:toggleInspector' as const,
      shortcut: settings.toggleInspectorShortcut
    },
    {
      id: 'next-connection-view',
      labelKey: 'commands:nextConnectionView' as const,
      shortcut: settings.toggleTerminalFilesShortcut
    },
    { id: 'hosts', labelKey: 'hosts:connections' as const, shortcut: `${mod}+shift+h` },
    { id: 'settings', labelKey: 'native:settings' as const, shortcut: `${mod}+,` },
    {
      id: 'split-vertical',
      labelKey: 'commands:splitTerminalVertically' as const,
      shortcut: settings.splitVerticalShortcut
    },
    {
      id: 'split-horizontal',
      labelKey: 'commands:splitTerminalHorizontally' as const,
      shortcut: settings.splitHorizontalShortcut
    },
    ...Array.from({ length: 9 }, (_, index) => ({
      id: `tab-${index + 1}` as Command,
      labelKey: 'commands:workspaceTab' as const,
      params: { index: index + 1 },
      shortcut: `${mod}+${index + 1}`
    }))
  ]
}
export function getCommands(settings: Preferences, platform: string) {
  return getCommandDefinitions(settings, platform).map((command) => ({
    ...command,
    id: command.id as Command,
    label: t(command.labelKey, { index: 'params' in command ? command.params.index : undefined }),
    englishLabel: t(command.labelKey, {
      lng: 'en',
      index: 'params' in command ? command.params.index : undefined
    })
  }))
}
export function accelerator(shortcut: string): string {
  const map: Record<string, string> = {
    meta: 'Command',
    ctrl: 'Control',
    arrowleft: 'Left',
    arrowright: 'Right'
  }
  return shortcut
    .split('+')
    .map((key) => map[key] ?? key)
    .join('+')
}
