import { useRef, useState, type PointerEvent, type KeyboardEvent } from 'react'
/** Shared by container and image inspectors; width survives switching resource tabs. */
export function useDetailResize() {
  const root = useRef<HTMLDivElement>(null)
  const [ratio, setRatio] = useState(0.44),
    [dragging, setDragging] = useState(false)
  const clamp = (width: number, total: number) =>
    Math.max(320, Math.min(total - 280, width)) / total
  const move = (e: PointerEvent<HTMLDivElement>) => {
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return
    const bounds = root.current!.getBoundingClientRect()
    setRatio(clamp(bounds.right - e.clientX, bounds.width))
  }
  const end = (e: PointerEvent<HTMLDivElement>) => {
    if (e.currentTarget.hasPointerCapture(e.pointerId))
      e.currentTarget.releasePointerCapture(e.pointerId)
    setDragging(false)
  }
  const key = (e: KeyboardEvent<HTMLDivElement>) => {
    const total = root.current?.clientWidth
    if (!total || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return
    e.preventDefault()
    const width =
      e.key === 'Home'
        ? 320
        : e.key === 'End'
          ? total - 280
          : clamp(ratio * total, total) * total + (e.key === 'ArrowLeft' ? 16 : -16)
    setRatio(clamp(width, total))
  }
  return {
    root,
    ratio,
    dragging,
    separator: {
      onPointerDown: (e: PointerEvent<HTMLDivElement>) => {
        e.preventDefault()
        e.currentTarget.setPointerCapture(e.pointerId)
        setDragging(true)
      },
      onPointerMove: move,
      onPointerUp: end,
      onPointerCancel: end,
      onLostPointerCapture: () => setDragging(false),
      onKeyDown: key,
      onDoubleClick: () => setRatio(0.44)
    }
  }
}
