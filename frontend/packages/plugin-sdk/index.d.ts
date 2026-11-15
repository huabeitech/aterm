export interface PluginProcessRequest {
  /** Per-process overrides; null removes an inherited variable. Names require execution.env permission. */
  env?: Record<string, string | null>
  executable: string
  args: string[]
  timeoutMs?: number
  /** Default: error on overflow. Tail retains the newest combined stdout/stderr bytes. */
  output?: 'error' | 'tail'
  /** Optional lower budget, 1024–131072 bytes; never raises the host limit. */
  maxOutputBytes?: number
}
export interface PluginProcessResult {
  stdout: string
  stderr: string
  exitCode: number | null
  truncated?: boolean
}
/** A persistent process inherits its source session/grant lifetime. */
export interface PluginProcessStreamRequest {
  /** Per-process overrides; null removes an inherited variable. Names require execution.env permission. */
  env?: Record<string, string | null>
  executable: string
  args: string[]
  /** Per-batch tail budget: 1024 to 24576 bytes, default 24576. */
  maxOutputBytes?: number
}
export interface PluginProcessStreamEvent {
  id: string
  /** stdout/stderr merged in arrival order, batched with UTF-8 decoding. */
  text: string
  truncated: boolean
  done: boolean
  exitCode?: number | null
  error?: string
}
export interface PluginTerminalRequest extends Omit<
  PluginProcessRequest,
  'output' | 'maxOutputBytes'
> {
  title?: string
  checks?: { args: string[]; stdout: string }[]
}
/** Executable access is high privilege. These declarations are not read-only policies. */
export interface PluginExecutionPolicy {
  programs: string[]
  targets: ('local' | 'ssh')[]
  env?: string[]
  unsetEnv?: string[]
}
/** Public plugin protocol. No application implementation types cross this boundary. */
export interface PluginManifest {
  schemaVersion: 1
  id: string
  name: string
  version: string
  runtime: 'sandbox'
  entry: string
  permissions: ('process.execute' | 'terminal.create')[]
  execution?: PluginExecutionPolicy
  engines?: { hostApi: 1 }
  contributes: {
    terminalStatusBar?: { id: string }
    commands?: PluginCommand[]
    views?: PluginView[]
  }
  i18n?: { defaultLocale: string; locales: Record<string, string> }
}
export interface PluginCommand {
  id: string
  title: string
  action?: PluginViewAction
}
export interface PluginViewAction {
  type: 'view.open' | 'view.toggle' | 'view.close'
  view: string
  context?: JsonValue
}
export interface PluginView {
  id: string
  title: string
  entry: string
  placement: 'terminal.sidePanel' | 'terminal.workspace'
  /** Opt in to session-scoped availability published by the plugin. */
  availability?: 'dynamic'
}
export type JsonValue =
  null | boolean | number | string | JsonValue[] | { [key: string]: JsonValue }
export interface PluginViewDocument {
  url: string
  instanceId: string
}
export interface PluginCommandResult {
  data?: JsonValue
  action?: PluginViewAction
}
export interface PluginLocalizations {
  defaultLocale: string
  messages: Record<string, Record<string, string>>
}
export interface PluginInfo {
  localizations?: PluginLocalizations
  manifest: PluginManifest
  builtIn: boolean
  enabled: boolean
  state: 'inactive' | 'active' | 'failed'
  interval: number
  grants: { id: string; label: string }[]
  canRollback?: boolean
  error?: string
}
export interface StatusItem {
  id: string
  label: string
  value: string
  detail?: string
  level?: 'normal' | 'warning' | 'error'
  command?: string
  visibility?: 'always' | 'expanded'
  compactValue?: string
  priority?: number
  trend?: { values: (number | null)[]; ceiling?: number }
}
export interface PluginViewAvailability {
  visible: boolean
  available: boolean
  message?: string
}
export interface PluginStatus {
  pluginId: string
  sessionId: string
  state: 'permission-required' | 'loading' | 'ready' | 'delayed' | 'unsupported' | 'unavailable'
  items: StatusItem[]
  views?: Record<string, PluginViewAvailability>
  data?: JsonValue
  updatedAt?: number
}
export interface RefreshContext {
  readonly sessionId: string
  readonly token: string
  readonly interval: number
  readonly sessionLabel: string
  readonly hostKey: string
}
export type RpcResult<T> = { ok: true; value: T } | { ok: false }
export interface PluginApi {
  readonly capabilities: {
    readonly hostApi: 1
    readonly statusBar: boolean
    readonly views: boolean
    readonly workspaceViews: boolean
    readonly processExecution: boolean
    readonly processOutputTail?: boolean
    readonly processStreaming?: boolean
    readonly interactiveTerminals: boolean
    readonly commands: boolean
    readonly storage: boolean
  }
  readonly i18n: {
    readonly locale: string
    t(key: string, options?: Record<string, string | number>): string
    onDidChangeLocale(callback: (locale: string) => void): () => void
  }
  readonly commands: {
    registerCommand(
      id: string,
      callback: (context: {
        token: string
        command: string
        args: JsonValue
      }) => void | JsonValue | Promise<void | JsonValue>
    ): () => void
  }
  readonly storage: {
    get(key: string): Promise<JsonValue>
    set(key: string, value: JsonValue): Promise<void>
  }
  onRefresh(
    callback: (context: RefreshContext) => void | JsonValue | Promise<void | JsonValue>
  ): void
  readonly process: {
    run(request: PluginProcessRequest & { token: string }): Promise<PluginProcessResult>
    subscribe(
      request: PluginProcessStreamRequest & { token: string },
      callback: (event: PluginProcessStreamEvent) => void | Promise<void>
    ): Promise<{ dispose(): Promise<void> }>
  }
  readonly terminals: {
    open(request: PluginTerminalRequest & { token: string }): Promise<{ terminalSessionId: string }>
  }
  request(
    method: 'process.execute' | 'terminal.create',
    params: { token: string; request: PluginProcessRequest | PluginTerminalRequest }
  ): Promise<RpcResult<PluginProcessResult | { terminalSessionId: string } | { error: string }>>
  request(method: 'runtime.ready', params: Record<string, never>): Promise<RpcResult<void>>
  request(
    method: 'status.publish',
    params: Pick<RefreshContext, 'token'> & {
      state: 'ready' | 'delayed' | 'unsupported' | 'unavailable'
      items: StatusItem[]
      views?: Record<string, PluginViewAvailability>
      data?: JsonValue
    }
  ): Promise<RpcResult<void>>
}
export interface PluginViewState {
  status?: Pick<PluginStatus, 'state' | 'items' | 'data' | 'updatedAt'>
  context: {
    active?: boolean
    invocationId: string
    sessionId: string
    sessionLabel: string
    view: JsonValue
  }
  environment: { locale: string; appearance: 'dark' | 'light'; tokens: Record<string, string> }
}
export interface PluginViewApi {
  readonly ready: Promise<void>
  readonly state: PluginViewState
  t(key: string, options?: Record<string, string | number>): string
  onDidChange(callback: (state: PluginViewState) => void): () => void
  executeCommand(command: string, args?: JsonValue): Promise<JsonValue>
  close(): Promise<void>
}
declare global {
  const atermPlugin: PluginApi
  const atermView: PluginViewApi
  interface Window {
    readonly atermPlugin: PluginApi
    readonly atermView: PluginViewApi
  }
}
