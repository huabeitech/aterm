import type { DragEvent } from 'react'
import type { domain } from '../../api/types'
import type {
  BusyAction,
  DirectoryDropTarget,
  DirectoryTreeItem,
  HostSortScope
} from '../common/types'

export interface ConnectionTreeProps {
  currentHostID?: string
  hoverPreviewEnabled?: boolean
  hosts: domain.Host[]
  loading: boolean
  directoryTree: DirectoryTreeItem[]
  selectedHostID?: string | null
  selectedHostIDs: string[]
  activeDirectoryID: string | null
  busyAction: BusyAction
  draggingDirectoryID: string | null
  draggingHostIDs: string[]
  directoryDropTarget: DirectoryDropTarget
  hostDropTargetID: string | null
  onSelectHost: (id: string, ids: string[]) => void
  onSelectDirectory: (id: string) => void
  onCreateHost: (directoryID?: string) => void
  onRenameHost: (id: string, name: string) => Promise<string | null>
  onEditHost: (id: string) => void
  onOpenHost: (id: string) => void
  onConnectHost: (id: string) => void
  onOpenFiles: (id: string) => void
  onDeleteHosts: (ids: string[]) => void
  onToggleFavoriteHost: (id: string) => void
  onCopySSHCommand: (id: string) => void
  onBatchAddHosts: () => void
  onMoveHosts: (ids: string[], directoryID: string, orderedIDs?: string[]) => void
  onHostDragStart: (ids: string[]) => void
  onHostDragEnd: () => void
  onReorderHosts: (scope: HostSortScope, ids: string[]) => void
  onStartCreateDirectory: (parentID?: string) => void
  onStartRenameDirectory: (directory: domain.Directory) => void
  onDeleteDirectory: (directory: domain.Directory) => void
  onDirectoryDragStart: (event: DragEvent<HTMLElement>, directory: domain.Directory) => void
  onDirectoryDragOver: (
    event: DragEvent<HTMLElement>,
    directory: domain.Directory | null,
    depth?: number
  ) => void
  onDirectoryDrop: (
    event: DragEvent<HTMLElement>,
    directory: domain.Directory | null,
    depth?: number
  ) => void
  onDirectoryDragEnd: () => void
  onHostDragOver: (event: DragEvent<HTMLElement>, directoryID: string) => void
  onHostDrop: (event: DragEvent<HTMLElement>, directoryID: string) => void
}
