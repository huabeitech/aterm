import {
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
  type PointerEvent
} from 'react'
export function useMarqueeSelection(
  selected: string[],
  setSelected: Dispatch<SetStateAction<string[]>>,
  onCommit: (paths: string[]) => void
) {
  const container = useRef<HTMLDivElement>(null)
  const [box, setBox] = useState<{
    left: number
    top: number
    width: number
    height: number
  } | null>(null)
  const gesture = useRef<{
    x: number
    y: number
    clientX: number
    clientY: number
    base: string[]
    additive: boolean
    row: boolean
    active: boolean
    pointer: number
    selection?: string[]
  }>()
  const frame = useRef(0),
    suppressClick = useRef(false)
  function update() {
    const element = container.current,
      g = gesture.current
    if (!element || !g?.active) return
    const rect = element.getBoundingClientRect()
    const x = Math.max(0, Math.min(rect.width, g.clientX - rect.left)) + element.scrollLeft
    const y = Math.max(0, Math.min(rect.height, g.clientY - rect.top)) + element.scrollTop
    const next = {
      left: Math.min(x, g.x),
      top: Math.min(y, g.y),
      width: Math.abs(x - g.x),
      height: Math.abs(y - g.y)
    }
    setBox(next)
    const hits = [...element.querySelectorAll<HTMLElement>('[data-remote-path]')]
      .filter((row) => {
        const r = row.getBoundingClientRect(),
          left = r.left - rect.left + element.scrollLeft,
          top = r.top - rect.top + element.scrollTop
        return (
          left < next.left + next.width &&
          left + r.width > next.left &&
          top < next.top + next.height &&
          top + r.height > next.top
        )
      })
      .map((row) => row.dataset.remotePath!)
    const selection = g.additive ? [...new Set([...g.base, ...hits])] : hits
    g.selection = selection
    setSelected((previous) =>
      previous.length === selection.length && previous.every((p, i) => p === selection[i])
        ? previous
        : selection
    )
  }
  function autoScroll() {
    const element = container.current,
      g = gesture.current
    if (!element || !g?.active) return
    const rect = element.getBoundingClientRect(),
      previous = element.scrollTop
    if (g.clientY < rect.top + 32) element.scrollTop -= 12
    else if (g.clientY > rect.bottom - 28) element.scrollTop += 12
    if (previous !== element.scrollTop) update()
    frame.current = requestAnimationFrame(autoScroll)
  }
  function stop(cancel = false) {
    const g = gesture.current
    if (g && cancel) setSelected(g.base)
    if (g && container.current?.hasPointerCapture(g.pointer))
      container.current.releasePointerCapture(g.pointer)
    gesture.current = undefined
    cancelAnimationFrame(frame.current)
    setBox(null)
  }
  useEffect(() => () => cancelAnimationFrame(frame.current), [])
  return {
    container,
    box,
    cancel: () => stop(true),
    handlers: {
      onPointerDown: (event: PointerEvent<HTMLDivElement>) => {
        if (
          event.button !== 0 ||
          !event.currentTarget.contains(event.target as Node) ||
          (event.target as HTMLElement).closest('button,input,label,thead,.files-entry-name')
        )
          return
        const element = event.currentTarget,
          rect = element.getBoundingClientRect()
        element.focus({ preventScroll: true })
        gesture.current = {
          x: event.clientX - rect.left + element.scrollLeft,
          y: event.clientY - rect.top + element.scrollTop,
          clientX: event.clientX,
          clientY: event.clientY,
          base: selected,
          additive: event.metaKey || event.ctrlKey || event.shiftKey,
          row: !!(event.target as HTMLElement).closest('[data-remote-path]'),
          active: false,
          pointer: event.pointerId
        }
      },
      onPointerMove: (event: PointerEvent<HTMLDivElement>) => {
        const g = gesture.current
        if (!g || g.pointer !== event.pointerId) return
        const distance = Math.hypot(event.clientX - g.clientX, event.clientY - g.clientY)
        if (!g.active && distance < 4) return
        if (!g.active) {
          g.active = true
          event.currentTarget.setPointerCapture(event.pointerId)
          frame.current = requestAnimationFrame(autoScroll)
        }
        g.clientX = event.clientX
        g.clientY = event.clientY
        update()
      },
      onPointerUp: () => {
        const g = gesture.current
        if (!g) return
        suppressClick.current = g.active
        if (g.active) onCommit(g.selection ?? [])
        else if (!g.row && !g.additive) {
          setSelected([])
          onCommit([])
        }
        stop()
      },
      onPointerCancel: () => stop(true),
      onClickCapture: (event: React.MouseEvent<HTMLDivElement>) => {
        if (suppressClick.current) {
          suppressClick.current = false
          event.preventDefault()
          event.stopPropagation()
        }
      }
    }
  }
}
