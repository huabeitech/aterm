import { DesktopRequest } from '../../../../bindings/ssh-connect/internal/app/app'
import { Events } from '@wailsio/runtime'

export interface AgentConfig {
  enabled: boolean
}
export function agentRequest<T>(method: string, ...args: unknown[]): Promise<T> {
  return DesktopRequest('agent:request', [method, ...args]) as Promise<T>
}
export function onAgentUnlock(handler: (request: {id: string}) => void) {
  return Events.On('agent:unlock', event => handler(event.data))
}
