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
export interface TunnelsApi {
  list(): Promise<TunnelStatus[]>
  save(id: string, input: TunnelInput): Promise<Tunnel>
  start(id: string): Promise<void>
  stop(id: string): Promise<void>
  delete(id: string): Promise<void>
  onChanged(callback: () => void): () => void
}
