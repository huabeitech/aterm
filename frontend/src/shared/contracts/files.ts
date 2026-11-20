// Saved hosts remain addressable by ID. Temporary connections are explicitly
// bound to a terminal and a particular SSH lifetime, never stored as fake hosts.
export type FileTarget = string | { kind: 'terminal'; terminalID: string; contextID: string }
export function fileTargetKey(target: FileTarget | undefined): string | undefined {
  return typeof target === 'string'
    ? target
    : target
      ? `terminal:${target.terminalID}:${target.contextID}`
      : undefined
}
export interface FileSession {
  id: string
  hostID?: string
  target: FileTarget
  title: string
  state: 'connecting' | 'ready' | 'error'
  home: string
  initialPath?: string
  error?: string
}
export interface RemoteEntry {
  name: string
  path: string
  kind: 'directory' | 'file' | 'link' | 'other'
  size: number
  modified: number
  mode: number
  uid: number
  gid: number
}
export interface DirectoryListing {
  path: string
  entries: RemoteEntry[]
}
export interface RemoteText {
  path: string
  text: string
  revision: string
}
export type FileMutation =
  | { kind: 'mkdir' | 'touch'; parent: string; name: string }
  | { kind: 'rename'; path: string; destination: string }
  | { kind: 'delete'; paths: string[] }
  | { kind: 'chmod'; path: string; mode: string }
export interface UploadConflict {
  id: string
  path: string
  localKind: 'file' | 'folder'
  remoteKind: 'file' | 'folder' | 'link' | 'special'
  localSize: number
  remoteSize: number
  localTime: string
  remoteTime: string
  multiple: boolean
}
export interface FileTransfer {
  id: string
  sessionID: string
  direction: 'upload' | 'download'
  name: string
  remotePath: string
  localPath: string
  conflict?: UploadConflict
  uploaded?: number
  skipped?: number
  state: 'conflict' | 'skipped' | 'queued' | 'paused' | 'running' | 'completed' | 'cancelled' | 'failed'
  transferred: number
  bytesPerSecond?: number
  total: number
  error?: string
  directory?: boolean
}
export type FileEvent =
  | { type: 'saved'; sessionID: string; path: string }
  | { type: 'session'; session: FileSession }
  | { type: 'transfer'; transfer: FileTransfer }
  | { type: 'transfers-removed'; ids: string[] }
export interface FilesApi {
  open(target: FileTarget, terminalID?: string): Promise<FileSession>
  close(id: string): Promise<void>
  list(id: string, path: string): Promise<DirectoryListing>
  mutate(id: string, mutation: FileMutation): Promise<void>
  readText(id: string, path: string): Promise<RemoteText>
  saveText(id: string, document: RemoteText): Promise<RemoteText>
  upload(id: string, directory: string, folder?: boolean): Promise<void>
  drop(id: string, directory: string, paths: string[]): Promise<void>
  pathsForFiles(files: File[]): string[]
  image(id: string, path: string): Promise<{ dataURL: string; size: number }>
  download(id: string, paths: string[]): Promise<void>
  resolveConflict(id: string, transferID: string, choice: {id: string; action: 'skip' | 'overwrite'; all: boolean}): Promise<void>
  pause(id: string, transferID: string): Promise<void>
  resume(id: string, transferID: string): Promise<void>
  cancel(id: string, transferID: string): Promise<void>
  retry(id: string, transferID: string): Promise<void>
  clearCompleted(id: string): Promise<void>
  removeTransfer(id: string, transferID: string): Promise<void>
  reveal(id: string, transferID: string): Promise<void>
  onEvent(callback: (event: FileEvent) => void): () => void
}
