import { t } from '../../../../../shared/i18n'
import { directoryScopeIDs } from '../../../../../shared/directoryScope'
import { domain } from '../../../api/types'
import type { DirectoryDropTarget, DirectoryTreeItem } from '../../../components/common/types'

export function buildDirectoryTree(directories: domain.Directory[]): DirectoryTreeItem[] {
  const children = new Map<string, domain.Directory[]>()
  directories.forEach((directory) => {
    const parentID = directory.parentID || ''
    children.set(parentID, [...(children.get(parentID) ?? []), directory])
  })
  children.forEach((items) => items.sort(compareDirectories))
  const result: DirectoryTreeItem[] = []
  const visit = (parentID: string, depth: number) => {
    for (const directory of children.get(parentID) ?? []) {
      result.push({ directory, depth })
      visit(directory.id, depth + 1)
    }
  }
  visit('', 0)
  return result
}

export function buildDirectoryHostCounts(directories: domain.Directory[], hosts: domain.Host[]) {
  const counts = new Map<string, number>()
  const directCounts = new Map<string, number>()
  hosts.forEach((host) =>
    directCounts.set(host.directoryID, (directCounts.get(host.directoryID) ?? 0) + 1)
  )
  directories.forEach((directory) => {
    let count = 0
    for (const id of directoryScopeIDs(directories, directory.id))
      count += directCounts.get(id) ?? 0
    counts.set(directory.id, count)
  })
  return counts
}

export function buildDirectoryOptions(directories: domain.Directory[]) {
  const byID = new Map(directories.map((directory) => [directory.id, directory]))
  return buildDirectoryTree(directories).map(({ directory }) => ({
    id: directory.id,
    label: directoryPath(directory.id, byID) || directory.name
  }))
}

export function calculateDirectoryDropTarget(
  pointer: { clientX: number; clientY: number },
  bounds: { left: number; top: number; width: number; height: number },
  directory: domain.Directory | null,
  _depth: number,
  draggingID: string,
  directories: domain.Directory[]
): DirectoryDropTarget {
  if (!directory) return { id: null, position: 'root' }
  if (directory.id === draggingID || isDirectoryDescendant(directory.id, draggingID, directories))
    return null
  const y = pointer.clientY - bounds.top
  const edge = Math.min(8, bounds.height / 4)
  return {
    id: directory.id,
    position: y < edge ? 'before' : y > bounds.height - edge ? 'after' : 'inside'
  }
}

export function resolveDirectoryMove(
  draggingID: string,
  target: NonNullable<DirectoryDropTarget>,
  directories: domain.Directory[]
): domain.DirectoryMoveInput | null {
  if (target.position === 'root')
    return {
      parentID: '',
      sortOrder: directories.filter(
        (directory) => (directory.parentID || '') === '' && directory.id !== draggingID
      ).length
    }
  const targetDirectory = directories.find((directory) => directory.id === target.id)
  if (!targetDirectory) return null
  if (target.position === 'inside')
    return {
      parentID: targetDirectory.id,
      sortOrder: directories.filter(
        (directory) =>
          (directory.parentID || '') === targetDirectory.id && directory.id !== draggingID
      ).length
    }
  const parentID = targetDirectory.parentID || ''
  const siblings = directories
    .filter((directory) => (directory.parentID || '') === parentID && directory.id !== draggingID)
    .sort(compareDirectories)
  const targetIndex = Math.max(
    0,
    siblings.findIndex((directory) => directory.id === targetDirectory.id)
  )
  return { parentID, sortOrder: target.position === 'after' ? targetIndex + 1 : targetIndex }
}

export function isDirectoryDescendant(
  id: string,
  ancestorID: string,
  directories: domain.Directory[]
) {
  let current = directories.find((directory) => directory.id === id)
  while (current?.parentID) {
    if (current.parentID === ancestorID) return true
    current = directories.find((directory) => directory.id === current?.parentID)
  }
  return false
}

export function compareDirectories(a: domain.Directory, b: domain.Directory) {
  return (
    a.sortOrder - b.sortOrder ||
    a.createdAt.localeCompare(b.createdAt) ||
    a.name.localeCompare(b.name)
  )
}

export function directoryPath(directoryID: string, directoryByID: Map<string, domain.Directory>) {
  if (!directoryID) return ''
  const names: string[] = []
  const seen = new Set<string>()
  let current = directoryByID.get(directoryID)
  while (current && !seen.has(current.id)) {
    names.unshift(current.name)
    seen.add(current.id)
    current = current.parentID ? directoryByID.get(current.parentID) : undefined
  }
  return names.join(' / ')
}

export function directoryLabel(directoryID: string, directories: domain.Directory[]) {
  if (!directoryID) return t('hosts:rootDirectory')
  return (
    directoryPath(
      directoryID,
      new Map(directories.map((directory) => [directory.id, directory]))
    ) || t('hosts:missingDirectory')
  )
}
