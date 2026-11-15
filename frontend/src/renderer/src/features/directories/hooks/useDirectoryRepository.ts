import { useState } from 'react'
import { domain } from '../../../api/types'
import type {
  DirectoryDropTarget,
  DirectoryEditorState,
  ViewID
} from '../../../components/common/types'
import {
  CreateDirectory,
  DeleteDirectory,
  ListDirectories,
  MoveDirectory,
  UpdateDirectory
} from '../../../api/directories'

export function useDirectoryRepository() {
  const [directories, setDirectories] = useState<domain.Directory[]>([])
  const [activeView, setActiveView] = useState<ViewID>('all')
  const [activeDirectoryID, setActiveDirectoryID] = useState<string | null>(null)
  const [directoryEditor, setDirectoryEditor] = useState<DirectoryEditorState>(null)
  const [draggingDirectoryID, setDraggingDirectoryID] = useState<string | null>(null)
  const [directoryDropTarget, setDirectoryDropTarget] = useState<DirectoryDropTarget>(null)
  const [hostDropTargetID, setHostDropTargetID] = useState<string | null>(null)

  async function loadDirectories() {
    const next = await ListDirectories()
    setDirectories(next)
    return next
  }

  const createDirectory = (input: domain.DirectoryInput) => CreateDirectory(input)
  const updateDirectory = (id: string, input: domain.DirectoryInput) => UpdateDirectory(id, input)
  async function moveDirectory(id: string, input: domain.DirectoryMoveInput) {
    const previous = directories
    setDirectories((current) => {
      const moving = current.find((d) => d.id === id)
      if (!moving) return current
      const siblings = current
        .filter((d) => d.id !== id && d.parentID === input.parentID)
        .sort((a, b) => a.sortOrder - b.sortOrder)
      siblings.splice(input.sortOrder, 0, { ...moving, parentID: input.parentID })
      const positions = new Map(siblings.map((d, index) => [d.id, index]))
      return current.map((d) =>
        d.id === id
          ? { ...d, parentID: input.parentID, sortOrder: positions.get(id)! }
          : positions.has(d.id)
            ? { ...d, sortOrder: positions.get(d.id)! }
            : d
      )
    })
    try {
      return await MoveDirectory(id, input)
    } catch (error) {
      setDirectories(previous)
      throw error
    }
  }
  const deleteDirectory = (id: string) => DeleteDirectory(id)

  return {
    directories,
    setDirectories,
    activeView,
    setActiveView,
    activeDirectoryID,
    setActiveDirectoryID,
    directoryEditor,
    setDirectoryEditor,
    draggingDirectoryID,
    setDraggingDirectoryID,
    directoryDropTarget,
    setDirectoryDropTarget,
    hostDropTargetID,
    setHostDropTargetID,
    loadDirectories,
    createDirectory,
    updateDirectory,
    moveDirectory,
    deleteDirectory
  }
}
