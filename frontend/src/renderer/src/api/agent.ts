import { DesktopRequest } from '../../../../bindings/ssh-connect/internal/app/app'
import { Events } from '@wailsio/runtime'

export interface AgentConfig {
  enabled: boolean
  hosts: Record<string, boolean> | null
}
export function agentRequest<T>(method: string, ...args: unknown[]): Promise<T> {
  return DesktopRequest('agent:request', [method, ...args]) as Promise<T>
}
export function onAgentOpen(handler: (request: {id: string; locked: boolean}) => void) {
  return Events.On('agent:open', event => handler(event.data))
}
