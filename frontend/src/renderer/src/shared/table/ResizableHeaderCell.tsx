import { cn } from '@/lib/utils'
import { useEffect, useRef, type CSSProperties, type ReactNode, type ThHTMLAttributes } from 'react'

type Props = ThHTMLAttributes<HTMLTableCellElement> & {
  width?: number
  minWidth?: number
  maxWidth?: number
  onResize?: (width: number) => void
  onResizeStart?: () => void
  children?: ReactNode
}

export function ResizableHeaderCell({
  width,
  minWidth = 80,
  maxWidth = 640,
  onResize,
  onResizeStart,
  children,
  style,
  ...props
}: Props) {
  const cleanupResize = useRef<(() => void) | undefined>()
  useEffect(() => () => cleanupResize.current?.(), [])

  function startResize(event: React.MouseEvent<HTMLSpanElement>) {
    if (!width || !onResize) return
    cleanupResize.current?.()
    event.preventDefault()
    event.stopPropagation()
    onResizeStart?.()
    const startX = event.clientX
    const startWidth = width
    const scale = Math.max(
      1,
      Number(getComputedStyle(document.documentElement).getPropertyValue('--app-font-scale')) || 1
    )
    const handleMouseMove = (moveEvent: MouseEvent) => {
      if (!(moveEvent.buttons & 1)) return handleMouseUp()
      onResize(clamp(startWidth + (moveEvent.clientX - startX) / scale, minWidth, maxWidth))
    }
    const handleMouseUp = () => {
      document.body.classList.remove('is-resizing-table-column', 'select-none')
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
      window.removeEventListener('blur', handleMouseUp)
      cleanupResize.current = undefined
    }
    cleanupResize.current = handleMouseUp
    document.body.classList.add('is-resizing-table-column', 'select-none')
    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    window.addEventListener('blur', handleMouseUp)
  }

  const mergedStyle: CSSProperties = width
    ? { ...style, width: `calc(${width}px * var(--app-layout-scale))` }
    : (style ?? {})
  return (
    <th {...props} style={mergedStyle}>
      {children}
      {onResize && (
        <span
          className={cn(
            'table-column-resizer absolute top-0 right-0 bottom-0 z-1 w-2 cursor-col-resize select-none [-webkit-app-region:no-drag]',
            "after:absolute after:top-2 after:right-[3px] after:bottom-2 after:w-px after:content-[''] after:bg-[var(--mac-separator)]",
            '[&:hover::after]:w-0.5 [&:hover::after]:bg-[#1677ff] [body.is-resizing-table-column_&::after]:absolute',
            '[body.is-resizing-table-column_&::after]:top-2 [body.is-resizing-table-column_&::after]:right-[3px]',
            '[body.is-resizing-table-column_&::after]:bottom-2 [body.is-resizing-table-column_&::after]:w-0.5',
            "[body.is-resizing-table-column_&::after]:content-[''] [body.is-resizing-table-column_&::after]:bg-[#1677ff]"
          )}
          onMouseDown={startResize}
        />
      )}
    </th>
  )
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max)
}
