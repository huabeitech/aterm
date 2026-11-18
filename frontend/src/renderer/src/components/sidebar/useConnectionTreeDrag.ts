import { t } from '../../../../shared/i18n'
import { useEffect, useRef, useState, type DragEvent } from 'react'
import type { ConnectionTreeProps } from './connectionTreeTypes'
import type { ConnectionTreeNode } from './connectionTreeModel'
import { buildConnectionTree } from './connectionTreeModel'
import { calculateDirectoryDropTarget } from '../../features/directories/model/directoryModel'
import { compareHostsInScope } from '../../features/hosts/model/hostModel'

type Target = { id: string | null; position: 'before' | 'after' | 'inside' | 'root' }

export function useConnectionTreeDrag(
  props: ConnectionTreeProps,
  reveal: (id: string) => void,
  searching = false
) {
  const preview = useRef<HTMLElement | null>(null)
  const scroll = useRef<{ viewport: HTMLElement; speed: number } | null>(null)
  const frame = useRef<number | null>(null)
  const cancelled = useRef(false)
  const suppressUntil = useRef(0)
  const [target, setTarget] = useState<Target | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pending = useRef<string | null>(null)
  const active = !!props.draggingDirectoryID || props.draggingHostIDs.length > 0
  function clearTimer() {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
    pending.current = null
  }
  function stopScroll() {
    if (frame.current !== null) cancelAnimationFrame(frame.current)
    frame.current = null
    scroll.current = null
  }
  function autoScroll(event: DragEvent<HTMLElement>) {
    const viewport = (event.currentTarget.closest('[data-slot="scroll-area-viewport"]') ??
      event.currentTarget.querySelector('[data-slot="scroll-area-viewport"]')) as HTMLElement | null
    if (!viewport) return
    const bounds = viewport.getBoundingClientRect()
    const edge = Math.min(48, bounds.height / 4)
    const speed =
      event.clientY < bounds.top + edge
        ? -Math.max(0, (bounds.top + edge - event.clientY) / edge) * 12
        : Math.max(0, (event.clientY - bounds.bottom + edge) / edge) * 12
    scroll.current = { viewport, speed }
    if (!speed) {
      stopScroll()
      return
    }
    if (frame.current !== null) return
    const tick = () => {
      const current = scroll.current
      if (!current) return
      const before = current.viewport.scrollTop
      current.viewport.scrollTop += current.speed
      if (before !== current.viewport.scrollTop) {
        setTarget(null)
        clearTimer()
      }
      frame.current = requestAnimationFrame(tick)
    }
    frame.current = requestAnimationFrame(tick)
  }
  function clear() {
    stopScroll()
    clearTimer()
    setTarget(null)
  }
  useEffect(() => {
    if (!active) clear()
    return () => {
      clearTimer()
      stopScroll()
    }
  }, [active])
  useEffect(
    () => () => {
      preview.current?.remove()
    },
    []
  )
  useEffect(() => {
    const cancel = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && active) {
        event.preventDefault()
        cancelled.current = true
        end()
      }
    }
    window.addEventListener('keydown', cancel, true)
    return () => {
      window.removeEventListener('keydown', cancel, true)
    }
  }, [active])

  function resolve(event: DragEvent<HTMLElement>, node?: ConnectionTreeNode): Target | null {
    if (cancelled.current || !active || props.busyAction) return null
    if (!node) return { id: null, position: 'root' }
    if (props.draggingDirectoryID) {
      // A host is not a directory drop zone. Never let its event become a root drop.
      if (node.kind === 'host') return null
      const next = calculateDirectoryDropTarget(
        event,
        event.currentTarget.getBoundingClientRect(),
        node.directory,
        node.depth,
        props.draggingDirectoryID,
        props.directoryTree.map((entry) => entry.directory)
      )
      return searching && next?.position !== 'inside' ? null : next
    }
    if (node.kind === 'directory') return { id: node.id, position: 'inside' }
    if (searching || props.draggingHostIDs.includes(node.id)) return null
    const bounds = event.currentTarget.getBoundingClientRect()
    return {
      id: node.id,
      position: event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after'
    }
  }

  function over(event: DragEvent<HTMLElement>, node?: ConnectionTreeNode) {
    if (!active) return
    event.preventDefault()
    event.stopPropagation()
    if (node) autoScroll(event)
    else stopScroll()
    const next = resolve(event, node)
    event.dataTransfer.dropEffect = next ? 'move' : 'none'
    setTarget(next)
    const expandID =
      next?.position === 'inside' && node?.kind === 'directory' && !node.expanded ? node.id : null
    if (expandID !== pending.current) {
      clearTimer()
      if (expandID) {
        pending.current = expandID
        timer.current = setTimeout(() => {
          reveal(expandID)
          clearTimer()
        }, 650)
      }
    }
  }

  function leave(event: DragEvent<HTMLElement>) {
    if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget))
      return
    clear()
  }

  function start(event: DragEvent<HTMLElement>, node: ConnectionTreeNode) {
    event.stopPropagation()
    if (props.busyAction) {
      event.preventDefault()
      return
    }
    cancelled.current = false
    clear()
    preview.current?.remove()
    const ghost = document.createElement('div')
    ghost.className = 'connection-drag-preview'
    const count =
      node.kind === 'host' && props.selectedHostIDs.includes(node.id)
        ? props.selectedHostIDs.length
        : 1
    ghost.textContent =
      (node.kind === 'host' ? node.host.name : node.directory.name) +
      (count > 1 ? ` · ${t('hosts:selectedConnections', { count })}` : '')
    document.body.append(ghost)
    preview.current = ghost
    event.dataTransfer.setDragImage(ghost, 16, 16)
    if (node.kind === 'directory') props.onDirectoryDragStart(event, node.directory)
    else {
      const selected = props.selectedHostIDs.includes(node.id) ? props.selectedHostIDs : [node.id]
      // Use tree order, including selected rows currently hidden by collapse/search.
      const ids = buildConnectionTree(props.directoryTree, props.hosts, new Set(), '')
        .filter((item) => item.kind === 'host' && selected.includes(item.id))
        .map((item) => item.id)
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData('text/plain', ids.join(','))
      props.onHostDragStart(ids)
    }
  }

  function end() {
    suppressUntil.current = Date.now() + 350
    preview.current?.remove()
    preview.current = null
    clear()
    props.onHostDragEnd()
    props.onDirectoryDragEnd()
  }

  function drop(event: DragEvent<HTMLElement>, node?: ConnectionTreeNode) {
    if (!active) return
    event.preventDefault()
    event.stopPropagation()
    const next = resolve(event, node)
    clear()
    if (!next) {
      end()
      return
    }
    if (props.draggingDirectoryID) {
      suppressUntil.current = Date.now() + 350
      preview.current?.remove()
      props.onDirectoryDrop(event, node?.kind === 'directory' ? node.directory : null, node?.depth)
      return
    }
    const moving = props.draggingHostIDs
    if (node?.kind === 'host') {
      const directoryID = node.host.directoryID
      const scope = {
        type: directoryID ? ('directory' as const) : ('uncategorized' as const),
        id: directoryID
      }
      const siblings = props.hosts
        .filter((host) => host.directoryID === directoryID && !moving.includes(host.id))
        .sort((a, b) => compareHostsInScope(a, b, scope))
        .map((host) => host.id)
      siblings.splice(siblings.indexOf(node.id) + (next.position === 'after' ? 1 : 0), 0, ...moving)
      if (
        moving.every(
          (id) => props.hosts.find((host) => host.id === id)?.directoryID === directoryID
        )
      )
        props.onReorderHosts(scope, siblings)
      else props.onMoveHosts(moving, directoryID, siblings)
    } else {
      if (node?.kind === 'directory') reveal(node.id)
      props.onMoveHosts(moving, node?.id ?? '')
    }
    end()
  }

  function backgroundOver(event: DragEvent<HTMLElement>) {
    if (!active) return
    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'none'
    setTarget(null)
    clearTimer()
    autoScroll(event)
  }

  return {
    target,
    active,
    start,
    over,
    leave,
    drop,
    end,
    backgroundOver,
    suppressClick: () => active || Date.now() < suppressUntil.current
  }
}
export type ConnectionTreeDrag = ReturnType<typeof useConnectionTreeDrag>
