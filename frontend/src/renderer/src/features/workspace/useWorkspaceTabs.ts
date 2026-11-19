import {
  type DragEvent,
  type SetStateAction,
  type WheelEvent as ReactWheelEvent,
  useCallback,
  useEffect,
  useRef,
  useState
} from 'react'
import {
  addWorkspaceTab,
  adjacentWorkspaceTab,
  createFilesWorkspaceTab,
  createWorkspaceTab,
  nextWorkspaceTabAfterClose,
  openHostWorkspaceTab,
  removeWorkspaceTabs,
  reorderWorkspaceTabs,
  type WorkspaceTab,
  type WorkspaceTabDropPosition,
  type WorkspaceTabID
} from './workspaceModel'

export function useWorkspaceTabs() {
  const [activeWorkspaceTab, setActiveWorkspaceTabState] = useState<WorkspaceTabID>('home')
  const [workspaceTabs, setWorkspaceTabs] = useState<WorkspaceTab[]>([])
  const [draggingWorkspaceTabID, setDraggingWorkspaceTabID] = useState<WorkspaceTabID | null>(null)
  const [workspaceTabDropTarget, setWorkspaceTabDropTarget] = useState<{
    id: WorkspaceTabID
    position: WorkspaceTabDropPosition
  } | null>(null)
  const [tabScrollState, setTabScrollState] = useState({
    canScrollBack: false,
    canScrollForward: false
  })
  const workspaceTabsRef = useRef<HTMLDivElement>(null)
  const workspaceTabsStateRef = useRef(workspaceTabs)
  const activeWorkspaceTabRef = useRef(activeWorkspaceTab)
  const tabHistoryRef = useRef<WorkspaceTabID[]>([])

  const setActiveWorkspaceTab = useCallback((tab: SetStateAction<WorkspaceTabID>) => {
    const current = activeWorkspaceTabRef.current
    const next = typeof tab === 'function' ? tab(current) : tab
    if (next === current) return
    tabHistoryRef.current = [
      current,
      ...tabHistoryRef.current.filter((item) => item !== current && item !== next)
    ]
    activeWorkspaceTabRef.current = next
    setActiveWorkspaceTabState(next)
  }, [])

  const setTabs = useCallback((next: WorkspaceTab[]) => {
    workspaceTabsStateRef.current = next
    setWorkspaceTabs(next)
  }, [])

  function openWorkspaceTab(tab: WorkspaceTabID) {
    if (tab === 'home') return
    setTabs(addWorkspaceTab(workspaceTabsStateRef.current, createWorkspaceTab(tab)))
  }

  function openHostTab(hostID: string) {
    const current = workspaceTabsStateRef.current
    const { tabs, tab } = openHostWorkspaceTab(current, hostID)
    setTabs(tabs)
    setActiveWorkspaceTab(tab.id)
    tabHistoryRef.current = tabHistoryRef.current.filter((id) => tabs.some((item) => item.id === id))
    return tab
  }

  function findRecentTab(predicate: (tab: WorkspaceTab) => boolean) {
    const candidates = workspaceTabsStateRef.current.filter(predicate)
    for (const id of [activeWorkspaceTabRef.current, ...tabHistoryRef.current]) {
      const match = candidates.find((tab) => tab.id === id)
      if (match) return match
    }
    return candidates[0]
  }

  function openFilesWorkspaceTab(hostID: string, afterTab?: WorkspaceTabID) {
    const tab = createFilesWorkspaceTab(hostID)
    const current = workspaceTabsStateRef.current
    const index = afterTab ? current.findIndex((item) => item.id === afterTab) : -1
    setTabs(
      index < 0
        ? [...current, tab]
        : [...current.slice(0, index + 1), tab, ...current.slice(index + 1)]
    )
    setActiveWorkspaceTab(tab.id)
  }

  function removeTabs(tabs: WorkspaceTabID[]) {
    const current = workspaceTabsStateRef.current
    const next = removeWorkspaceTabs(current, tabs)
    const removed = new Set(tabs)
    setTabs(next)
    tabHistoryRef.current = tabHistoryRef.current.filter((tab) => !removed.has(tab))
    if (!removed.has(activeWorkspaceTabRef.current)) return

    setActiveWorkspaceTab(
      nextWorkspaceTabAfterClose(
        current,
        activeWorkspaceTabRef.current,
        tabHistoryRef.current,
        tabs
      )
    )
  }

  function updateTabScrollState() {
    const element = workspaceTabsRef.current
    if (!element) return
    const maxScrollLeft = element.scrollWidth - element.clientWidth
    setTabScrollState({
      canScrollBack: element.scrollLeft > 1,
      canScrollForward: element.scrollLeft < maxScrollLeft - 1
    })
  }

  function scrollWorkspaceTabs(direction: -1 | 1) {
    workspaceTabsRef.current?.scrollBy({ left: direction * 240, behavior: 'smooth' })
  }

  function scrollWorkspaceTabsWithWheel(event: ReactWheelEvent<HTMLDivElement>) {
    const element = event.currentTarget
    if (event.ctrlKey || element.scrollWidth <= element.clientWidth) return
    const unit =
      event.deltaMode === WheelEvent.DOM_DELTA_LINE
        ? 16
        : event.deltaMode === WheelEvent.DOM_DELTA_PAGE
          ? element.clientWidth
          : 1
    const delta =
      (Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY) * unit
    if (!delta) return
    event.preventDefault()
    element.scrollBy({ left: delta, behavior: 'auto' })
  }

  function closableTabs() {
    return workspaceTabs.filter((tab) => tab.closable).map((tab) => tab.id)
  }

  function tabsToRight(tab: WorkspaceTabID) {
    const index = workspaceTabs.findIndex((item) => item.id === tab)
    return index < 0 ? [] : workspaceTabs.slice(index + 1).map((item) => item.id)
  }

  function startTabDrag(event: DragEvent<HTMLElement>, tab: WorkspaceTabID) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', tab)
    setDraggingWorkspaceTabID(tab)
  }

  function dropTab(event: DragEvent<HTMLElement>, target: WorkspaceTabID) {
    event.preventDefault()
    const source =
      draggingWorkspaceTabID ?? (event.dataTransfer.getData('text/plain') as WorkspaceTabID)
    if (!source || source === target) return
    const bounds = event.currentTarget.getBoundingClientRect()
    setTabs(
      reorderWorkspaceTabs(
        workspaceTabsStateRef.current,
        source,
        target,
        event.clientX > bounds.left + bounds.width / 2
      )
    )
    finishTabDrag()
  }

  function previewTabDrop(event: DragEvent<HTMLElement>, target: WorkspaceTabID) {
    event.preventDefault()
    if (!draggingWorkspaceTabID || draggingWorkspaceTabID === target) return
    const bounds = event.currentTarget.getBoundingClientRect()
    setWorkspaceTabDropTarget({
      id: target,
      position: event.clientX > bounds.left + bounds.width / 2 ? 'after' : 'before'
    })
  }

  function finishTabDrag() {
    setDraggingWorkspaceTabID(null)
    setWorkspaceTabDropTarget(null)
  }

  function adjacentTab(direction: -1 | 1) {
    return adjacentWorkspaceTab(workspaceTabs, activeWorkspaceTab, direction)
  }

  function moveActiveTab(direction: -1 | 1) {
    const current = workspaceTabsStateRef.current
    const index = current.findIndex((tab) => tab.id === activeWorkspaceTab)
    const target = index + direction
    if (index < 0 || target < 0 || target >= current.length) return
    setTabs(reorderWorkspaceTabs(current, activeWorkspaceTab, current[target].id, direction > 0))
  }

  useEffect(() => {
    const element = workspaceTabsRef.current
    if (!element) return
    const update = () => updateTabScrollState()
    update()
    element.addEventListener('scroll', update, { passive: true })
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => {
      element.removeEventListener('scroll', update)
      observer.disconnect()
    }
  }, [])

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      workspaceTabsRef.current
        ?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]')
        ?.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'smooth' })
      updateTabScrollState()
    })
    return () => cancelAnimationFrame(frame)
  }, [activeWorkspaceTab, workspaceTabs])

  return {
    activeWorkspaceTab,
    setActiveWorkspaceTab,
    workspaceTabs,
    setWorkspaceTabs,
    draggingWorkspaceTabID,
    setDraggingWorkspaceTabID,
    workspaceTabDropTarget,
    setWorkspaceTabDropTarget,
    tabScrollState,
    setTabScrollState,
    workspaceTabsRef,
    openWorkspaceTab,
    openHostTab,
    findRecentTab,
    openFilesWorkspaceTab,
    removeTabs,
    updateTabScrollState,
    scrollWorkspaceTabs,
    scrollWorkspaceTabsWithWheel,
    closableTabs,
    tabsToRight,
    startTabDrag,
    dropTab,
    previewTabDrop,
    finishTabDrag,
    adjacentTab,
    moveActiveTab
  }
}
