import { domain as service } from '../../api/types'

export type ViewID = 'all' | 'recent' | 'uncategorized' | 'directory' | 'settings'
export type EditorMode = 'view' | 'create' | 'edit'
export type BusyAction =
  | 'export'
  | 'connect'
  | 'save'
  | 'delete'
  | 'directory'
  | 'settings'
  | 'sync'
  | 'sshDirectory'
  | 'syncDirectory'
  | null
export type HostSortScope = {
  type: 'all' | 'directory' | 'uncategorized'
  id: string
}
export type DirectoryEditorState =
  | { mode: 'create'; parentID: string; name: string }
  | { mode: 'rename'; id: string; name: string }
  | null
export type DirectoryTreeItem = { directory: service.Directory; depth: number }
export type DirectoryDropTarget = {
  id: string | null
  position: 'before' | 'after' | 'inside' | 'root'
} | null

export type FormState = {
  directoryID: string
  name: string
  alias: string
  hostName: string
  user: string
  port: string
  identityFile: string
  keySource: 'system' | 'managed'
  passwordAuth: boolean
  password: string
  passwordState: 'none' | 'saved' | 'editing'
  proxyJump: string
  jumpHostID: string
  forwardAgent: boolean
  compression: boolean
  serverAliveInterval: string
  serverAliveCountMax: string
  connectTimeout: string
  remark: string
}
