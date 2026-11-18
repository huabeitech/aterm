export interface TunnelInput {
  name: string
  hostID: string
  localAddress: string
  localPort: number
  targetAddress: string
  targetPort: number
}
export interface Tunnel extends TunnelInput {
  id: string
  createdAt: string
  updatedAt: string
}
export interface TunnelStatus extends Tunnel {
  state: 'stopped' | 'starting' | 'running' | 'stopping' | 'failed'
  error: string
  lastError: string
  activeConfig: Tunnel | null
}

export interface Host {
  id: string
  alias: string
  hostName: string
  user: string
}
