/** Wails implements the same DesktopApi as the Electron preload.
 * UI components remain runtime-independent; all Wails transport stays here.
 */
import * as bindings from '../../../../bindings/ssh-connect/internal/app/app'
import {Events, Window as WailsWindow} from '@wailsio/runtime'
const windowName = WailsWindow.Name()
import {createWailsKeyboard, type InputMode} from './wailsKeyboard'
import type {Command} from '../../../shared/contracts/commands'
import {channels, type DesktopApi} from '../../../shared/contracts/app'
import {terminalEvents} from './terminalEvents'
import {installNativeFileDrop} from './wailsFileDrop'
import {subscribeTerminalOutput, beginTerminalOutputAttachment, releaseTerminalOutput} from './wailsTerminalOutput'
const keyboardCommands = new Set<(command: Command) => void>()
const keyboard = createWailsKeyboard(
  command => {for (const callback of keyboardCommands) callback(command)},
  () => {void invokeRequest(channels.window, 'close').catch(console.error)}
)
async function invokeRequest(channel: string, ...args: unknown[]): Promise<any> {
  // Register before invoking Go: live events may arrive before its snapshot
  // response. A close invalidates this ticket so a late response cannot revive it.
  const attachment = channel === channels.terminalRequest && args[0] === 'attach'
    ? beginTerminalOutputAttachment(args[1] as string) : undefined
  let result: any
  try {
    result = await bindings.DesktopRequest(channel, args)
  } catch (error) {
    attachment?.cancel()
    throw error
  }
  if (channel === channels.bootstrap) {
    const bootstrap = {...(result as import('../../../shared/contracts/app').Bootstrap), resolvedTheme: window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' as const : 'light' as const}
    keyboard.bootstrap(bootstrap)
    if (location.hash === '#file-editor') sendInputMode('editable')
    return bootstrap
  }
  if (channel === channels.preferences) keyboard.preferences(result as import('../../../shared/contracts/preferences').Preferences)
  if (attachment) {
    try {return attachment.complete(result as {data: string; seq: number})}
    catch (error) {attachment.cancel(); throw error}
  }
  if (channel === channels.terminalRequest && args[0] === 'close') releaseTerminalOutput(args[1] as string)
  return result
}
function subscribe<T>(channel: string, callback: (value: T) => void): () => void {
  if (channel === channels.terminalEvent) return subscribeTerminalOutput(callback as (event: import('../../../shared/contracts/terminal').TerminalEvent) => void)
  return Events.On(channel, (event) => {
    void windowName.then((name) => {if (!event.sender || event.sender === name) callback(event.data as T)})
  })
}
const send = (channel: string, ...args: unknown[]) => {
  void invokeRequest(channel, ...args.map((value) => value instanceof Uint8Array ? Array.from(value) : value)).catch((error) => console.error('Desktop operation failed', channel, error))
}
let inputModeWrite = Promise.resolve()
function sendInputMode(mode: InputMode) {
  keyboard.setMode(mode)
  // Wails invokes bindings concurrently. Serialize focus changes so a slow old
  // request cannot overwrite the mode of the newly focused control.
  inputModeWrite = inputModeWrite.then(() => invokeRequest(channels.inputMode, mode))
    .catch(error => console.error('Unable to update input mode', error))
}
async function invokeLibrary(operation: string, ...args: unknown[]) {
  try {
    return await invokeRequest(channels.library, operation, ...args)
  } catch (error) {
    const text = error instanceof Error ? error.message : 'Unable to update the host library.'
    throw new Error(text.replace(/^Error invoking remote method '[^']+': (?:Error: )?/, ''))
  }
}
async function invokeIntegration(operation: string, ...args: unknown[]) {
  try {
    return await invokeRequest(channels.integration, operation, ...args)
  } catch (error) {
    throw new Error(
      (error instanceof Error ? error.message : 'Operation failed.').replace(
        /^Error invoking remote method '[^']+': (?:Error: )?/,
        ''
      )
    )
  }
}
async function invokeFiles(operation: string, ...args: unknown[]) {
  try {
    return await invokeRequest(channels.filesRequest, operation, ...args)
  } catch (error) {
    throw new Error(
      (error instanceof Error ? error.message : 'File operation failed.').replace(
        /^Error invoking remote method '[^']+': (?:Error: )?/,
        ''
      )
    )
  }
}
async function invokeFileEditor(operation: string, ...args: unknown[]) {
  try {
    return await invokeRequest(channels.fileEditor, operation, ...args)
  } catch (error) {
    throw new Error(
      (error instanceof Error ? error.message : 'File operation failed.').replace(
        /^Error invoking remote method '[^']+': (?:Error: )?/,
        ''
      )
    )
  }
}
const subscribeTerminal = terminalEvents((dispatch) => subscribe(channels.terminalEvent, dispatch))
const desktop: DesktopApi = {
  tunnels: {
    test: (id, input) => invokeRequest(channels.tunnels, 'test', id, input),
    list: () => invokeRequest(channels.tunnels, 'list'),
    save: (id, input) => invokeRequest(channels.tunnels, 'save', id, input),
    start: (id) => invokeRequest(channels.tunnels, 'start', id),
    stop: (id) => invokeRequest(channels.tunnels, 'stop', id),
    delete: (id) => invokeRequest(channels.tunnels, 'delete', id),
    onChanged: (callback) => subscribe(channels.tunnelsChanged, callback)
  },
  confirmations: {
    onEvent: (callback) => subscribe(channels.confirmationEvent, callback),
    answer: (id, choice) => invokeRequest(channels.confirmationAnswer, id, choice)
  },
  plugins: {
    activity: (id) => invokeRequest(channels.plugins, 'activity', id),
    list: () => invokeRequest(channels.plugins, 'list'),
    executeCommand: (...args) => invokeRequest(channels.plugins, 'executeCommand', ...args),
    openView: (...args) => invokeRequest(channels.plugins, 'openView', ...args),
    closeView: (...args) => invokeRequest(channels.plugins, 'closeView', ...args),
    configure: (id, enabled, interval) =>
      invokeRequest(channels.plugins, 'configure', id, enabled, interval),
    revoke: (id, grantId) => invokeRequest(channels.plugins, 'revoke', id, grantId),
    install: () => invokeRequest(channels.plugins, 'install'),
    rollback: (id) => invokeRequest(channels.plugins, 'rollback', id),
    authorize: (id, sessionId) => invokeRequest(channels.plugins, 'authorize', id, sessionId),
    uninstall: (id) => invokeRequest(channels.plugins, 'uninstall', id),
    visibility: (pluginId, sessionId, consumer, visible, passive = false) =>
      invokeRequest(
        channels.plugins,
        'visibility',
        pluginId,
        sessionId,
        consumer,
        visible,
        passive
      ),
    refresh: (pluginId, sessionId) =>
      invokeRequest(channels.plugins, 'refresh', pluginId, sessionId),
    onChanged: (callback) => subscribe(channels.pluginsChanged, callback),
    onTerminalOpened: (callback) => subscribe(channels.pluginTerminalOpened, callback),
    onStatus: (callback) => subscribe(channels.pluginStatus, callback)
  },
  themes: {
    list: () => invokeRequest(channels.themes, 'list'),
    import: () => invokeRequest(channels.themes, 'import'),
    duplicate: (theme) => invokeRequest(channels.themes, 'duplicate', theme),
    remove: (id) => invokeRequest(channels.themes, 'remove', id),
    export: (id) => invokeRequest(channels.themes, 'export', id),
    onChanged: (callback) => subscribe(channels.themesChanged, callback)
  },
  onLocaleChanged: (callback) => subscribe(channels.locale, callback),
  integration: {
    status: (...args) => invokeIntegration('status', ...args),
    chooseDirectory: (...args) => invokeIntegration('chooseDirectory', ...args),
    autoUnlock: (...args) => invokeIntegration('autoUnlock', ...args),
    changePassword: (...args) => invokeIntegration('changePassword', ...args),
    backup: (...args) => invokeIntegration('backup', ...args),
    restore: (...args) => invokeIntegration('restore', ...args),
    deleteBackup: (...args) => invokeIntegration('deleteBackup', ...args),
    exportSSH: (...args) => invokeIntegration('exportSSH', ...args)
  },
  fileEditor: {
    open: (...args) => invokeFileEditor('open', ...args),
    state: () => invokeFileEditor('state'),
    change: (text) => invokeFileEditor('change', text),
    save: () => invokeFileEditor('save'),
    close: () => invokeFileEditor('close'),
    quit: () => invokeFileEditor('quit'),
    onCloseRequested: (callback) => subscribe(channels.fileEditorClose, callback),
    onState: (callback) => subscribe(channels.fileEditorState, callback)
  },
  files: {
    pathsForFiles: (files) => files.map((file) => (file as File & {path?: string}).path ?? '').filter(Boolean),
    image: (...args) => invokeFiles('image', ...args),
    drop: (...args) => invokeFiles('drop', ...args),
    open: (...args) => invokeFiles('open', ...args),
    close: (...args) => invokeFiles('close', ...args),
    list: (...args) => invokeFiles('list', ...args),
    mutate: (...args) => invokeFiles('mutate', ...args),
    readText: (...args) => invokeFiles('readText', ...args),
    saveText: (...args) => invokeFiles('saveText', ...args),
    upload: (...args) => invokeFiles('upload', ...args),
    download: (...args) => invokeFiles('download', ...args),
    cancel: (...args) => invokeFiles('cancel', ...args),
    retry: (...args) => invokeFiles('retry', ...args),
    clearCompleted: (...args) => invokeFiles('clearCompleted', ...args),
    removeTransfer: (...args) => invokeFiles('removeTransfer', ...args),
    reveal: (...args) => invokeFiles('reveal', ...args),
    onEvent: (callback) => subscribe(channels.filesEvent, callback)
  },

  terminal: {
    accessibilityEnabled: () => invokeRequest(channels.terminalRequest, 'accessibilityEnabled'),
    cancelTransfer: (...args) => invokeRequest(channels.terminalRequest, 'cancelTransfer', ...args),
    open: (...args) => invokeRequest(channels.terminalRequest, 'open', ...args),
    attach: (...args) => invokeRequest(channels.terminalRequest, 'attach', ...args),
    close: (...args) => invokeRequest(channels.terminalRequest, 'close', ...args),
    answer: (...args) => invokeRequest(channels.terminalRequest, 'answer', ...args),
    list: (...args) => invokeRequest(channels.terminalRequest, 'list', ...args),
    requiresUnlock: (...args) => invokeRequest(channels.terminalRequest, 'requiresUnlock', ...args),
    readClipboard: (...args) => invokeRequest(channels.terminalRequest, 'readClipboard', ...args),
    openLink: (...args) => invokeRequest(channels.terminalRequest, 'openLink', ...args),
    input: (...args) => send(channels.terminalInput, ...args),
    inputBytes: (...args) => send(channels.terminalBinaryInput, ...args),
    resize: (...args) => send(channels.terminalResize, ...args),
    acknowledge: (...args) => send(channels.terminalAck, ...args),
    onEvent: subscribeTerminal
  },
  library: {
    listHosts: (...args) => invokeLibrary('listHosts', ...args),
    createHost: (...args) => invokeLibrary('createHost', ...args),
    updateHost: (...args) => invokeLibrary('updateHost', ...args),
    deleteHosts: (...args) => invokeLibrary('deleteHosts', ...args),
    moveHosts: (...args) => invokeLibrary('moveHosts', ...args),
    reorderHosts: (...args) => invokeLibrary('reorderHosts', ...args),
    listDirectories: (...args) => invokeLibrary('listDirectories', ...args),
    createDirectory: (...args) => invokeLibrary('createDirectory', ...args),
    updateDirectory: (...args) => invokeLibrary('updateDirectory', ...args),
    moveDirectory: (...args) => invokeLibrary('moveDirectory', ...args),
    deleteDirectory: (...args) => invokeLibrary('deleteDirectory', ...args),
    previewSnippet: (...args) => invokeLibrary('previewSnippet', ...args),
    importSnippet: (...args) => invokeLibrary('importSnippet', ...args),
    vaultStatus: (...args) => invokeLibrary('vaultStatus', ...args),
    unlockVault: (...args) => invokeLibrary('unlockVault', ...args),
    getHostPassword: (...args) => invokeLibrary('getHostPassword', ...args),
    copyText: (...args) => invokeLibrary('copyText', ...args),
    listManagedKeys: (...args) => invokeLibrary('listManagedKeys', ...args),
    createManagedKey: (...args) => invokeLibrary('createManagedKey', ...args),
    chooseManagedKeyImport: (...args) => invokeLibrary('chooseManagedKeyImport', ...args),
    importManagedKey: (...args) => invokeLibrary('importManagedKey', ...args),
    exportManagedKey: (...args) => invokeLibrary('exportManagedKey', ...args),
    addManagedKeyToAgent: (...args) => invokeLibrary('addManagedKeyToAgent', ...args),
    deleteManagedKey: (...args) => invokeLibrary('deleteManagedKey', ...args)
  },
  getAppInfo: () => invokeRequest(channels.info),
  bootstrap: () => invokeRequest(channels.bootstrap),
  savePreferences: (value) => invokeRequest(channels.preferences, value),
  setInputMode: sendInputMode,
  openExternal: (url) => invokeRequest(channels.external, url),
  windowAction: (action) => invokeRequest(channels.window, action),
  onThemeChanged: (callback) => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const changed = () => callback(media.matches ? 'dark' : 'light')
    media.addEventListener('change', changed)
    const off = subscribe<'light' | 'dark'>(channels.theme, callback)
    return () => {media.removeEventListener('change', changed); off()}
  },
  onCommand: (callback) => {
    keyboardCommands.add(callback)
    const off = subscribe<Command>(channels.command, callback)
    return () => {keyboardCommands.delete(callback); off()}
  }
}
window.desktop = desktop
installNativeFileDrop()
