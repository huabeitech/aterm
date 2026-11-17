export type WorkspaceTabID = 'home' | 'tunnels' | 'settings' | `files:${string}` | `terminal:${string}`
export type WorkspaceTabKind = 'tunnels' | 'settings' | 'files' | 'terminal'
export type WorkspaceTabDropPosition = 'before' | 'after'

export type WorkspaceTab = {
  id: Exclude<WorkspaceTabID, 'home'>
  kind: WorkspaceTabKind
  closable: boolean
  hostID?: string
}

let fileWorkspaceTabSequence = 0

export function createWorkspaceTab(id: Exclude<WorkspaceTabID, 'home'>): WorkspaceTab {
  if (id === 'tunnels') return { id, kind: 'tunnels', closable: true }
  if (id === 'settings') return { id, kind: 'settings', closable: true }
  return { id, kind: id.startsWith('files:') ? 'files' : 'terminal', closable: true }
}

export function createFilesWorkspaceTab(hostID: string): WorkspaceTab {
  fileWorkspaceTabSequence += 1
  return {
    id: `files:${hostID}:${fileWorkspaceTabSequence}`,
    kind: 'files',
    closable: true,
    hostID
  }
}

export function addWorkspaceTab(tabs: WorkspaceTab[], tab: WorkspaceTab): WorkspaceTab[] {
  return tabs.some((item) => item.id === tab.id) ? tabs : [...tabs, tab]
}

export function removeWorkspaceTabs(tabs: WorkspaceTab[], ids: WorkspaceTabID[]): WorkspaceTab[] {
  const removed = new Set(ids)
  return tabs.filter((tab) => !removed.has(tab.id))
}

export function nextWorkspaceTabAfterClose(
  tabs: WorkspaceTab[],
  active: WorkspaceTabID,
  history: WorkspaceTabID[],
  ids: WorkspaceTabID[]
): WorkspaceTabID {
  const removed = new Set(ids)
  const remaining = removeWorkspaceTabs(tabs, ids)
  const previousTab = history.find((tab) => remaining.some((item) => item.id === tab))
  if (previousTab) return previousTab
  const closingIndex = tabs.findIndex((tab) => tab.id === active)
  const leftTab = tabs
    .slice(0, closingIndex)
    .reverse()
    .find((tab) => !removed.has(tab.id) && remaining.some((item) => item.id === tab.id))
  return leftTab?.id ?? remaining[0]?.id ?? 'home'
}

export function reorderWorkspaceTabs(
  tabs: WorkspaceTab[],
  source: WorkspaceTabID,
  target: WorkspaceTabID,
  insertAfter: boolean
): WorkspaceTab[] {
  if (source === target) return tabs
  const sourceIndex = tabs.findIndex((tab) => tab.id === source)
  const targetIndex = tabs.findIndex((tab) => tab.id === target)
  if (sourceIndex < 0 || targetIndex < 0) return tabs
  const next = [...tabs]
  const [moved] = next.splice(sourceIndex, 1)
  const insertionIndex = next.findIndex((tab) => tab.id === target) + (insertAfter ? 1 : 0)
  next.splice(insertionIndex, 0, moved)
  return next
}

export function adjacentWorkspaceTab(
  tabs: WorkspaceTab[],
  active: WorkspaceTabID,
  direction: -1 | 1
): WorkspaceTabID | null {
  if (!tabs.length) return null
  const activeIndex = Math.max(
    0,
    tabs.findIndex((tab) => tab.id === active)
  )
  return tabs[(activeIndex + direction + tabs.length) % tabs.length]?.id ?? null
}
