import {getCommandDefinitions, type Command} from '../../../shared/contracts/commands'
import type {Bootstrap} from '../../../shared/contracts/app'
import type {Preferences} from '../../../shared/contracts/preferences'

export type InputMode = 'normal' | 'editable' | 'recorder' | 'terminal'
const globalCommands = new Set<Command>(['search', 'command-palette', 'toggle-sidebar', 'toggle-terminal-files'])
type Key = Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'shiftKey' | 'altKey' | 'isComposing'>
export function workspaceKeyCommand(event: Key, mode: InputMode, preferences: Preferences, platform: string, editor = false): Command | undefined {
  if (event.isComposing || event.key === 'Process' || mode === 'recorder') return
  const key = event.key.toLowerCase()
  // Native text-edit actions take precedence over configurable workspace keys.
  const modifier = platform === 'darwin' ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey
  if (mode === 'editable' && modifier && !event.altKey &&
    (event.shiftKey ? ['z', 'v'] : ['a', 'c', 'x', 'v', 'z', ...(platform === 'darwin' ? [] : ['y'])]).includes(key)) return
  return getCommandDefinitions(preferences, platform).find(command => {
    if (editor ? command.id !== 'find' && command.id !== 'close-tab' : mode === 'editable' && !globalCommands.has(command.id as Command)) return false
    const parts = command.shortcut.split('+')
    return parts.at(-1) === key && event.metaKey === parts.includes('meta') &&
      event.ctrlKey === parts.includes('ctrl') && event.shiftKey === parts.includes('shift') && event.altKey === parts.includes('alt')
  })?.id as Command | undefined
}

export function createWailsKeyboard(dispatch: (command: Command) => void, closeEditor: () => void) {
  const editor = location.hash === '#file-editor'
  let mode: InputMode = editor ? 'editable' : 'normal'
  let bootstrap: Bootstrap | undefined
  window.addEventListener('keydown', event => {
    if (!bootstrap) return
    const command = workspaceKeyCommand(event, mode, bootstrap.preferences, bootstrap.info.platform, editor)
    if (!command) return
    event.preventDefault()
    event.stopImmediatePropagation()
    if (!event.repeat) {
      if (editor && command === 'close-tab') closeEditor()
      else dispatch(command)
    }
  }, true)
  return {
    setMode(value: InputMode) { mode = value },
    bootstrap(value: Bootstrap) { bootstrap = value },
    preferences(value: Preferences) { if (bootstrap) bootstrap = {...bootstrap, preferences: value} }
  }
}
