export type {
  PluginExecutionPolicy,
  PluginProcessRequest,
  PluginProcessResult,
  PluginProcessStreamRequest,
  PluginProcessStreamEvent,
  PluginTerminalRequest
} from '../../../packages/plugin-sdk'
export interface PluginActivity {
  time: string
  pluginId: string
  target: string
  capability: 'process.execute' | 'terminal.create'
  executable: string
  argumentCount: number
  outcome: 'started' | 'succeeded' | 'failed' | 'denied' | 'cancelled'
}
