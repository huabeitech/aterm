import type { ConfirmationChoice, ConfirmationEvent } from './confirmation'
import type { FileEditorApi } from './fileEditor'
import type { IntegrationApi } from './integration'
import type { FilesApi } from './files'
import type { TerminalApi } from './terminal'
import type { LibraryApi } from './library'
import type { Preferences } from './preferences'
import type { Command } from './commands'
export const channels = {
  tunnels: 'tunnels:request',
  tunnelsChanged: 'tunnels:changed',
  plugins: 'plugins:manage',
  pluginsChanged: 'plugins:changed',
  pluginStatus: 'plugins:status',
  pluginTerminalOpened: 'plugins:terminal-opened',
  themes: 'themes:request',
  themesChanged: 'themes:changed',
  locale: 'locale:changed',
  integration: 'integration:request',
  confirmationEvent: 'confirmation:event',
  confirmationAnswer: 'confirmation:answer',
  fileEditor: 'file-editor:request',
  fileEditorState: 'file-editor:state',
  fileEditorClose: 'file-editor:close-requested',
  filesRequest: 'files:request',
  filesEvent: 'files:event',
  terminalRequest: 'terminal:request',
  terminalInput: 'terminal:input',
  terminalBinaryInput: 'terminal:binary-input',
  terminalResize: 'terminal:resize',
  terminalAck: 'terminal:ack',
  terminalEvent: 'terminal:event',
  info: 'app:info',
  library: 'library:request',
  bootstrap: 'app:bootstrap',
  preferences: 'preferences:save',
  theme: 'theme:changed',
  command: 'workspace:command',
  window: 'window:action',
  inputMode: 'workspace:input-mode',
  external: 'app:external'
} as const
export interface AppInfo {
  name: string
  version: string
  platform: string
  macAppStoreBuild: boolean
  localTerminalAvailable: boolean
}
export interface Bootstrap {
  locale: import('./locale').LocaleState
  info: AppInfo
  preferences: Preferences
  resolvedTheme: 'light' | 'dark'
  recoveryNotice: string | null
}
export interface DesktopApi {
  tunnels: import('./tunnels').TunnelsApi
  plugins: import('./plugins').PluginsApi
  themes: import('./themePackages').ThemesApi
  onLocaleChanged: (callback: (state: import('./locale').LocaleState) => void) => () => void
  integration: IntegrationApi
  confirmations: {
    onEvent: (callback: (event: ConfirmationEvent) => void) => () => void
    answer: (id: string, choice: ConfirmationChoice) => Promise<void>
  }
  fileEditor: FileEditorApi
  files: FilesApi
  terminal: TerminalApi
  library: LibraryApi
  getAppInfo: () => Promise<AppInfo>
  bootstrap: () => Promise<Bootstrap>
  savePreferences: (value: Preferences) => Promise<Preferences>
  setInputMode: (mode: 'normal' | 'editable' | 'recorder' | 'terminal') => void
  openExternal: (url: string) => Promise<void>
  windowAction: (action: 'minimize' | 'maximize' | 'close' | 'fullscreen') => Promise<void>
  onThemeChanged: (callback: (theme: 'light' | 'dark') => void) => () => void
  onCommand: (callback: (command: Command) => void) => () => void
}
