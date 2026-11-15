import type { domain } from '../../api/types'
import { compareHostsInScope } from '../../features/hosts/model/hostModel'
import { searchHosts } from '../hosts/hostSearch'
import type { DirectoryTreeItem } from '../common/types'

export type ConnectionTreeNode =
  | { kind: 'directory'; id: string; depth: number; directory: domain.Directory; expanded: boolean }
  | { kind: 'host'; id: string; depth: number; host: domain.Host }

// Search retains ancestors and temporarily expands them without changing saved disclosure state.
export function buildConnectionTree(
  directories: DirectoryTreeItem[],
  hosts: domain.Host[],
  collapsed: Set<string>,
  query: string
): ConnectionTreeNode[] {
  const matches = new Set(
    (query.trim() ? searchHosts(hosts, query).map((r) => r.host) : hosts).map((h) => h.id)
  )
  const filtered = !!query.trim()
  // Matching a folder includes its complete subtree, including empty folders.
  const folderMatches = new Set<string>()
  const normalized = query.trim().toLocaleLowerCase()
  if (filtered) {
    for (const { directory } of directories) {
      if (
        directory.name.toLocaleLowerCase().includes(normalized) ||
        folderMatches.has(directory.parentID)
      )
        folderMatches.add(directory.id)
    }
    for (const host of hosts) if (folderMatches.has(host.directoryID)) matches.add(host.id)
  }
  const included = new Set<string>()
  const byID = new Map(directories.map((d) => [d.directory.id, d.directory]))
  for (const id of folderMatches) {
    let parent = id
    while (parent && !included.has(parent)) {
      included.add(parent)
      parent = byID.get(parent)?.parentID ?? ''
    }
  }
  for (const host of hosts) {
    if (!matches.has(host.id)) continue
    let parent = host.directoryID
    while (parent && !included.has(parent)) {
      included.add(parent)
      parent = byID.get(parent)?.parentID ?? ''
    }
  }
  const nodes: ConnectionTreeNode[] = []
  const appendHosts = (directoryID: string, depth: number) => {
    const scope = {
      type: directoryID ? ('directory' as const) : ('uncategorized' as const),
      id: directoryID
    }
    hosts
      .filter((h) => h.directoryID === directoryID && matches.has(h.id))
      .sort((a, b) => compareHostsInScope(a, b, scope))
      .forEach((host) => nodes.push({ kind: 'host', id: host.id, depth, host }))
  }
  const visit = (parent: string, depth: number) => {
    for (const { directory } of directories.filter((d) => d.directory.parentID === parent)) {
      if (filtered && !included.has(directory.id)) continue
      const expanded = filtered || !collapsed.has(directory.id)
      nodes.push({ kind: 'directory', id: directory.id, depth, directory, expanded })
      if (expanded) visit(directory.id, depth + 1)
    }
    appendHosts(parent, depth)
  }
  visit('', 0)
  return nodes
}
