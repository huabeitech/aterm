import type { FileTarget } from './files'
export interface ZmodemProgress {
  state: 'running' | 'completed' | 'cancelled' | 'failed'
  direction: 'upload' | 'download'
  name: string
  transferred: number
  total: number
  error?: string
}
export type TerminalRequest = { kind: 'local'; newTab?: boolean } | { kind: 'ssh'; hostID: string }
export interface RemoteContext {
  id: string
  source: 'openssh'
  host: string
  user: string
  port: number
  state: 'ready'
}
export function remoteFileTarget(session: TerminalSession): FileTarget | undefined {
  return session.remote
    ? { kind: 'terminal', terminalID: session.id, contextID: session.remote.id }
    : session.hostID
}
export function hasRemoteConnection(session: Pick<TerminalSession, 'kind' | 'remote'>): boolean {
  return session.kind === 'ssh' || !!session.remote
}
export interface TerminalSession {
  id: string
  kind: 'local' | 'ssh'
  remote?: RemoteContext
  hostID?: string
  alias: string
  title: string
  state: 'connecting' | 'running' | 'exited' | 'error'
  startedAt: string
  exitCode?: number | null
  error?: string
  cols: number
  rows: number
  pid?: number
  localNumber?: number
  cwd?: string
  shell?: string
  customTitle?: string
}
export interface TerminalPrompt {
  id: string
  sessionID: string
  title: string
  message: string
  kind: 'host-key' | 'host-key-changed' | 'credentials'
  fields: { label: string; secret: boolean }[]
}
export type TerminalEvent =
  | { type: 'accessibility'; enabled: boolean }
  | { type: 'zmodem'; id: string; progress: ZmodemProgress }
  | { type: 'data'; id: string; seq: number; data: string }
  | { type: 'state'; session: TerminalSession }
  | { type: 'prompt'; prompt: TerminalPrompt }
  | { type: 'prompt-closed'; id: string }
export interface TerminalApi {
  accessibilityEnabled(): Promise<boolean>
  requiresUnlock(hostID: string): Promise<boolean>
  open(request: TerminalRequest): Promise<TerminalSession>
  attach(
    id: string,
    cols: number,
    rows: number
  ): Promise<{ session: TerminalSession; data: string; seq: number }>
  input(id: string, data: string): void
  inputBytes(id: string, data: Uint8Array): void
  resize(id: string, cols: number, rows: number): void
  acknowledge(id: string, seq: number): void
  close(id: string): Promise<void>
  cancelTransfer(id: string): Promise<void>
  answer(id: string, answers: string[] | null): Promise<void>
  list(): Promise<TerminalSession[]>
  readClipboard(): Promise<string>
  openLink(url: string): Promise<void>
  onEvent(callback: (event: TerminalEvent) => void, sessionID?: string): () => void
}
