import type { PluginInfo, PluginStatus } from '../../../../shared/contracts/plugins'
import type { TerminalSession } from '../../../../shared/contracts/terminal'

type Presentation =
  | 'loading'
  | 'ready'
  | 'paused'
  | 'failed'
  | 'disconnected'
  | 'permission-required'
  | 'unsupported'
  | 'delayed'

export function pluginPresentation(
  session: TerminalSession['state'],
  runtime: PluginInfo['state'],
  status?: PluginStatus['state'],
  paused = false
): Presentation {
  if (runtime === 'failed') return 'failed'
  if (session === 'connecting') return 'loading'
  if (session !== 'running') return 'disconnected'
  if (paused) return 'paused'
  if (!status || status === 'loading') return 'loading'
  if (status === 'unavailable') return 'delayed'
  return status
}

export const pluginPresentationKeys = {
  loading: 'common:loading',
  ready: 'settings:pluginsActive',
  paused: 'settings:pluginsPaused',
  failed: 'settings:pluginsFailed',
  disconnected: 'settings:pluginsDisconnected',
  'permission-required': 'settings:pluginsPermissionRequired',
  unsupported: 'settings:pluginsUnsupported',
  delayed: 'settings:pluginsDelayed'
} as const
