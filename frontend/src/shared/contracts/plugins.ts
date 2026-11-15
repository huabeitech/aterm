import type { PluginActivity } from './pluginExecution'
import type {
  JsonValue,
  PluginInfo,
  PluginStatus,
  PluginCommandResult,
  PluginViewDocument
} from '../../../packages/plugin-sdk'
export type {
  JsonValue,
  PluginInfo,
  PluginStatus,
  PluginCommandResult,
  PluginViewDocument,
  PluginManifest,
  PluginCommand,
  PluginView,
  PluginViewAction,
  PluginLocalizations,
  StatusItem
} from '../../../packages/plugin-sdk'
export interface PluginsApi {
  activity(pluginId: string): Promise<PluginActivity[]>
  list(): Promise<PluginInfo[]>
  executeCommand(
    pluginId: string,
    sessionId: string,
    command: string,
    args?: JsonValue
  ): Promise<PluginCommandResult>
  openView(pluginId: string, sessionId: string, view: string): Promise<PluginViewDocument>
  closeView(instanceId: string): Promise<void>
  configure(id: string, enabled: boolean, interval: number): Promise<void>
  revoke(id: string, grantId: string): Promise<void>
  install(): Promise<void>
  rollback(id: string): Promise<void>
  uninstall(id: string): Promise<void>
  authorize(pluginId: string, sessionId: string): Promise<void>
  visibility(
    pluginId: string,
    sessionId: string,
    consumer: string,
    visible: boolean,
    passive?: boolean
  ): Promise<void>
  refresh(pluginId: string, sessionId: string): Promise<void>
  onTerminalOpened(callback: (sessionId: string) => void): () => void
  onChanged(callback: () => void): () => void
  onStatus(callback: (status: PluginStatus) => void): () => void
}
