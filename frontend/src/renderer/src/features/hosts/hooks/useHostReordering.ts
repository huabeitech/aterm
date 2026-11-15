import { type DragEvent, useEffect, useRef, useState } from 'react'
import { domain } from '../../../api/types'
import type { HostSortScope } from '../../../components/common/types'
import { getHostDragIDs } from '../../../components/hosts/selectionState'

type Placement = 'before' | 'after'

export function useHostReordering({
  hosts,
  selectedIDs,
  reorderScope,
  onHostDragStart,
  onHostDragEnd,
  onReorderHosts
}: {
  hosts: domain.Host[]
  selectedIDs: string[]
  reorderScope: HostSortScope | null
  onHostDragStart: (ids: string[]) => void
  onHostDragEnd: () => void
  onReorderHosts: (scope: HostSortScope, ids: string[]) => void
}) {
  const [orderedHosts, setOrderedHosts] = useState(hosts)
  const [draggingHostIDs, setDraggingHostIDs] = useState<string[]>([])
  const [hostDropTarget, setHostDropTarget] = useState<{ id: string; placement: Placement } | null>(
    null
  )
  const draggingHostIDsRef = useRef<string[]>([])
  const canReorder = reorderScope !== null

  useEffect(() => setOrderedHosts(hosts), [hosts])

  function startHostDrag(event: DragEvent<HTMLTableRowElement>, host: domain.Host) {
    if (!canReorder) {
      event.preventDefault()
      return
    }
    const dragIDs = getHostDragIDs(selectedIDs, host.id)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('application/x-ssh-connect-hosts', JSON.stringify(dragIDs))
    event.dataTransfer.setData('text/plain', dragIDs.join(','))
    draggingHostIDsRef.current = dragIDs
    setDraggingHostIDs(dragIDs)
    onHostDragStart(dragIDs)
  }

  function finishHostDrag() {
    draggingHostIDsRef.current = []
    setDraggingHostIDs([])
    setHostDropTarget(null)
    onHostDragEnd()
  }

  const placement = (event: DragEvent<HTMLTableRowElement>): Placement => {
    const bounds = event.currentTarget.getBoundingClientRect()
    return event.clientY > bounds.top + bounds.height / 2 ? 'after' : 'before'
  }

  function handleHostDragOver(event: DragEvent<HTMLTableRowElement>, targetHost: domain.Host) {
    const draggedIDs = draggingHostIDsRef.current
    if (!canReorder || !draggedIDs.length || draggedIDs.includes(targetHost.id)) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    const nextPlacement = placement(event)
    setHostDropTarget((current) =>
      current?.id === targetHost.id && current.placement === nextPlacement
        ? current
        : { id: targetHost.id, placement: nextPlacement }
    )
  }

  function dropHostsToPosition(event: DragEvent<HTMLTableRowElement>, targetHost: domain.Host) {
    if (!canReorder || !reorderScope) return
    event.preventDefault()
    const draggedIDs = new Set(draggingHostIDsRef.current)
    if (!draggedIDs.size || draggedIDs.has(targetHost.id)) return
    const draggedHosts = orderedHosts.filter((host) => draggedIDs.has(host.id))
    const remainingHosts = orderedHosts.filter((host) => !draggedIDs.has(host.id))
    const targetIndex = remainingHosts.findIndex((host) => host.id === targetHost.id)
    if (targetIndex < 0) return
    const insertIndex = targetIndex + (placement(event) === 'after' ? 1 : 0)
    const nextHosts = [
      ...remainingHosts.slice(0, insertIndex),
      ...draggedHosts,
      ...remainingHosts.slice(insertIndex)
    ]
    if (nextHosts.every((host, index) => host.id === orderedHosts[index].id)) return
    setOrderedHosts(nextHosts)
    onReorderHosts(
      reorderScope,
      nextHosts.map((host) => host.id)
    )
  }

  return {
    orderedHosts,
    draggingHostIDs,
    hostDropTarget,
    canReorder,
    startHostDrag,
    finishHostDrag,
    handleHostDragOver,
    dropHostsToPosition
  }
}
