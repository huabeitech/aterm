import { t } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useRef } from 'react'
import { compareHostsInScope } from '../../hosts/model/hostModel'
import type { Dispatch, DragEvent, SetStateAction } from 'react'
import type { ConfirmationService } from '@/components/ui/confirm-provider'
import { domain } from '../../../api/types'
import type {
  BusyAction,
  DirectoryDropTarget,
  DirectoryEditorState,
  HostSortScope,
  ViewID
} from '../../../components/common/types'
import { validateDirectoryName } from '../../../components/common/directoryValidation'
import {
  calculateDirectoryDropTarget,
  isDirectoryDescendant,
  resolveDirectoryMove
} from '../model/directoryModel'

interface DirectoryActionsOptions {
  modal: ConfirmationService
  directories: domain.Directory[]
  hosts: domain.Host[]
  directoryEditor: DirectoryEditorState
  activeDirectoryID: string | null
  selectedID: string | null
  draggingDirectoryID: string | null
  draggingHostIDs: string[]
  setDirectoryEditor: Dispatch<SetStateAction<DirectoryEditorState>>
  setActiveView: Dispatch<SetStateAction<ViewID>>
  setActiveDirectoryID: Dispatch<SetStateAction<string | null>>
  setDraggingDirectoryID: Dispatch<SetStateAction<string | null>>
  setDirectoryDropTarget: Dispatch<SetStateAction<DirectoryDropTarget>>
  setDraggingHostIDs: Dispatch<SetStateAction<string[]>>
  setHostDropTargetID: Dispatch<SetStateAction<string | null>>
  setSelectedHostIDs: Dispatch<SetStateAction<string[]>>
  setSelectionAnchorID: Dispatch<SetStateAction<string | null>>
  setBusyAction: Dispatch<SetStateAction<BusyAction>>
  createDirectory: (input: domain.DirectoryInput) => Promise<domain.Directory>
  updateDirectory: (id: string, input: domain.DirectoryInput) => Promise<domain.Directory>
  moveDirectory: (id: string, input: domain.DirectoryMoveInput) => Promise<domain.Directory>
  deleteDirectory: (id: string) => Promise<void>
  moveHosts: (ids: string[], directoryID: string) => Promise<domain.Host[]>
  reorderHosts: (scope: HostSortScope, ids: string[]) => Promise<domain.Host[]>
  refreshAll: (selectedID?: string | null) => Promise<void>
  reportSuccess: (text: string) => void
  reportError: (text: string, title?: string) => void
}

export function useDirectoryActions(options: DirectoryActionsOptions) {
  useTranslation()
  const locked = useRef(false)
  async function applyDirectoryMove(id: string, move: domain.DirectoryMoveInput) {
    if (locked.current) return
    const previous = options.directories.find((d) => d.id === id)
    if (!previous) return
    locked.current = true
    options.setBusyAction('directory')
    try {
      await options.moveDirectory(id, move)
      await options.refreshAll(options.selectedID)
      options.reportSuccess(t('hosts:directoryMoved'))
    } catch (error) {
      options.reportError(readableDirectoryError(error), t('hosts:directoryError'))
      await options.refreshAll(options.selectedID)
    } finally {
      locked.current = false
      options.setBusyAction(null)
    }
  }
  function hostOrder(directoryID: string, hosts = options.hosts) {
    const scope = {
      type: directoryID ? ('directory' as const) : ('uncategorized' as const),
      id: directoryID
    }
    return hosts
      .filter((h) => h.directoryID === directoryID)
      .sort((a, b) => compareHostsInScope(a, b, scope))
      .map((h) => h.id)
  }
  async function restoreHosts(previous: domain.Host[], affected: Set<string>, moving: string[]) {
    for (const directoryID of affected) {
      const ids = previous
        .filter((h) => moving.includes(h.id) && h.directoryID === directoryID)
        .map((h) => h.id)
      if (ids.length) await options.moveHosts(ids, directoryID)
    }
    for (const directoryID of affected) {
      const ids = hostOrder(directoryID, previous)
      if (ids.length)
        await options.reorderHosts(
          { type: directoryID ? 'directory' : 'uncategorized', id: directoryID },
          ids
        )
    }
  }
  function startCreateDirectory(parentID = '') {
    options.setDirectoryEditor({ mode: 'create', parentID, name: '' })
  }

  function startRenameDirectory(directory: domain.Directory) {
    options.setDirectoryEditor({ mode: 'rename', id: directory.id, name: directory.name })
  }

  async function saveDirectory() {
    const editor = options.directoryEditor
    if (!editor) return
    const name = editor.name.trim()
    const validationError = validateDirectoryName(name)
    if (validationError) {
      options.reportError(validationError, t('hosts:directoryError'))
      return
    }
    options.setBusyAction('directory')
    try {
      if (editor.mode === 'create') {
        const created = await options.createDirectory({ parentID: editor.parentID, name })
        options.setActiveView('directory')
        options.setActiveDirectoryID(created.id)
        await options.refreshAll(options.selectedID)
        options.reportSuccess(t('hosts:directoryCreated'))
      } else {
        const current = options.directories.find((directory) => directory.id === editor.id)
        await options.updateDirectory(editor.id, { parentID: current?.parentID ?? '', name })
        await options.refreshAll(options.selectedID)
        options.reportSuccess(t('hosts:directoryRenamed'))
      }
      options.setDirectoryEditor(null)
    } catch (error) {
      const text = readableDirectoryError(error)
      options.reportError(text, t('hosts:directoryError'))
    } finally {
      options.setBusyAction(null)
    }
  }

  function deleteDirectory(directory: domain.Directory) {
    options.modal.confirm({
      title: t('hosts:deleteDirectoryConfirm', { name: directory.name }),
      content: t('hosts:onlyEmptyDirectoriesCanBeDeleted'),
      okText: t('hosts:delete'),
      okButtonProps: { danger: true },
      onOk: async () => {
        options.setBusyAction('directory')
        try {
          await options.deleteDirectory(directory.id)
          if (options.activeDirectoryID === directory.id) {
            options.setActiveView('all')
            options.setActiveDirectoryID(null)
          }
          await options.refreshAll(options.selectedID)
          options.reportSuccess(t('hosts:directoryDeleted'))
        } catch (error) {
          options.reportError(readableDirectoryError(error), t('hosts:directoryError'))
        } finally {
          options.setBusyAction(null)
        }
      }
    })
  }

  function startDirectoryDrag(event: DragEvent<HTMLElement>, directory: domain.Directory) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', directory.id)
    options.setDraggingDirectoryID(directory.id)
    options.setDirectoryDropTarget(null)
  }

  function updateDirectoryDrop(
    event: DragEvent<HTMLElement>,
    directory: domain.Directory | null,
    depth = 0
  ) {
    const draggingID = options.draggingDirectoryID
    if (!draggingID) return
    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    if (!directory) {
      options.setDirectoryDropTarget({ id: null, position: 'root' })
      return
    }
    if (
      directory.id === draggingID ||
      isDirectoryDescendant(directory.id, draggingID, options.directories)
    ) {
      options.setDirectoryDropTarget(null)
      return
    }
    options.setDirectoryDropTarget(
      calculateDirectoryDropTarget(
        event,
        event.currentTarget.getBoundingClientRect(),
        directory,
        depth,
        draggingID,
        options.directories
      )
    )
  }

  async function dropDirectory(
    event: DragEvent<HTMLElement>,
    directory: domain.Directory | null,
    depth = 0
  ) {
    const draggingID = options.draggingDirectoryID
    if (!draggingID) return
    updateDirectoryDrop(event, directory, depth)
    const target = calculateDirectoryDropTarget(
      event,
      event.currentTarget.getBoundingClientRect(),
      directory,
      depth,
      draggingID,
      options.directories
    )
    finishDirectoryDrag()
    if (!target || target.id === draggingID) return
    const move = resolveDirectoryMove(draggingID, target, options.directories)
    if (!move) return
    await applyDirectoryMove(draggingID, move)
  }

  function finishDirectoryDrag() {
    options.setDraggingDirectoryID(null)
    options.setDirectoryDropTarget(null)
  }
  function startHostDrag(ids: string[]) {
    options.setDraggingHostIDs(ids)
    options.setHostDropTargetID(null)
  }
  function updateHostDrop(event: DragEvent<HTMLElement>, directoryID: string) {
    if (!options.draggingHostIDs.length) return
    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    options.setHostDropTargetID(directoryID)
  }
  async function dropHosts(event: DragEvent<HTMLElement>, directoryID: string) {
    if (!options.draggingHostIDs.length) return
    event.preventDefault()
    event.stopPropagation()
    const ids = options.draggingHostIDs
    finishHostDrag()
    await moveHostsToDirectory(ids, directoryID)
  }
  function finishHostDrag() {
    options.setDraggingHostIDs([])
    options.setHostDropTargetID(null)
  }

  async function moveHostsToDirectory(
    hostIDs: string[],
    directoryID: string,
    orderedIDs?: string[]
  ) {
    if (locked.current) return
    const movingIDs = new Set(hostIDs)
    if (!options.hosts.some((h) => movingIDs.has(h.id) && h.directoryID !== directoryID)) return
    const previous = options.hosts
    const affected = new Set([
      directoryID,
      ...previous.filter((h) => movingIDs.has(h.id)).map((h) => h.directoryID)
    ])
    const order = orderedIDs ?? [
      ...hostOrder(directoryID).filter((id) => !movingIDs.has(id)),
      ...hostIDs
    ]
    locked.current = true
    options.setBusyAction('save')
    try {
      const movingHosts = await options.moveHosts(hostIDs, directoryID)
      await options.reorderHosts(
        { type: directoryID ? 'directory' : 'uncategorized', id: directoryID },
        order
      )
      await options.refreshAll(
        movingHosts.some((h) => h.id === options.selectedID) ? options.selectedID : hostIDs[0]
      )
      options.setSelectedHostIDs(hostIDs)
      options.setSelectionAnchorID(hostIDs[0])
      options.reportSuccess(t('hosts:hostsMoved', { count: hostIDs.length }))
    } catch (error) {
      options.reportError(errorText(error))
      try {
        await restoreHosts(previous, affected, hostIDs)
      } catch (restoreError) {
        options.reportError(errorText(restoreError))
      }
      await options.refreshAll(options.selectedID)
    } finally {
      locked.current = false
      options.setBusyAction(null)
    }
  }

  async function reorderHostsInScope(scope: HostSortScope, hostIDs: string[]) {
    if (locked.current) return
    const previous = hostOrder(scope.id)
    if (previous.join(',') === hostIDs.join(',')) return
    locked.current = true
    options.setBusyAction('save')
    try {
      await options.reorderHosts(scope, hostIDs)
    } catch (error) {
      options.reportError(errorText(error))
      await options.refreshAll(options.selectedID)
    } finally {
      locked.current = false
      options.setBusyAction(null)
    }
  }

  return {
    startCreateDirectory,
    startRenameDirectory,
    saveDirectory,
    deleteDirectory,
    startDirectoryDrag,
    updateDirectoryDrop,
    dropDirectory,
    finishDirectoryDrag,
    startHostDrag,
    updateHostDrop,
    dropHosts,
    finishHostDrag,
    moveHostsToDirectory,
    reorderHostsInScope
  }
}

const errorText = (error: unknown) =>
  error instanceof Error ? error.message : String(error || t('hosts:unexpectedError'))
function readableDirectoryError(error: unknown) {
  const text = errorText(error)
  if (
    error &&
    typeof error === 'object' &&
    'code' in error &&
    error.code === 'directoryContainsHosts'
  )
    return t('hosts:moveOrDeleteTheHostsInThisDirectoryBeforeDeletingIt')

  return text || t('hosts:unableToUpdateDirectory')
}
