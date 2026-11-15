import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useId, useLayoutEffect, useRef, type ReactNode } from 'react'
import { usePanelRef } from 'react-resizable-panels'
import { ResizablePanelGroup, ResizablePanel, ResizableHandle } from './resizable'

type Props = {
  first: ReactNode
  second: ReactNode
  size: number | string
  side?: 'first' | 'second'
  min: number
  max: number | string
  minimumOther?: number
  collapsed?: boolean
  preservePixelSize?: boolean
  withResizeHandle?: boolean
  onResize?: (sizes: number[]) => void
  className?: string
  firstClassName?: string
  secondClassName?: string
}

// Connect persisted desktop panel widths to the official Resizable component.
export function SplitLayout({
  first,
  second,
  size,
  side = 'first',
  min,
  max,
  minimumOther = 0,
  collapsed,
  preservePixelSize = false,
  withResizeHandle = false,
  onResize,
  className,
  firstClassName,
  secondClassName
}: Props) {
  useTranslation()
  const id = useId()
  const initialSize = useRef(collapsed ? 0 : size)
  const root = useRef<HTMLDivElement>(null)
  const panel = usePanelRef()
  const previousCollapsed = useRef(collapsed)
  useLayoutEffect(() => {
    if (previousCollapsed.current !== collapsed) {
      if (collapsed) panel.current?.collapse()
      else panel.current?.resize(size)
      previousCollapsed.current = collapsed
    }
  }, [collapsed, panel, size])
  const controlled = {
    panelRef: panel,
    groupResizeBehavior: preservePixelSize
      ? ('preserve-pixel-size' as const)
      : ('preserve-relative-size' as const),
    defaultSize: initialSize.current,
    minSize: collapsed ? 0 : min,
    maxSize: collapsed ? 0 : max,
    collapsible: !!collapsed,
    collapsedSize: 0,
    onResize: ({ inPixels }: { inPixels: number }) => {
      if (collapsed || inPixels <= 0) return
      const width = root.current?.clientWidth ?? 0
      if (!width) return
      onResize?.(side === 'first' ? [inPixels, width - inPixels] : [width - inPixels, inPixels])
    }
  }
  return (
    <ResizablePanelGroup
      elementRef={root}
      orientation="horizontal"
      // Every split must opt out: a nested split can otherwise override cursors
      // throughout the document, including the connection tree and plugin frames.
      disableCursor
      className={className}
      resizeTargetMinimumSize={withResizeHandle ? { fine: 12, coarse: 24 } : undefined}
    >
      <ResizablePanel
        id={`${id}-first`}
        {...(side === 'first' ? controlled : { minSize: minimumOther })}
      >
        <div
          className={firstClassName}
          style={{
            height: '100%',
            minWidth: 0,
            overflow: 'hidden',
            display: side === 'first' && collapsed ? 'none' : undefined
          }}
        >
          {first}
        </div>
      </ResizablePanel>
      {!collapsed && (
        <ResizableHandle
          withHandle={withResizeHandle}
          onPointerUp={(event) => {
            // Release synchronously so the next interaction can enter an iframe panel.
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              event.currentTarget.releasePointerCapture(event.pointerId)
          }}
          onPointerCancel={(event) => {
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              event.currentTarget.releasePointerCapture(event.pointerId)
          }}
          title={withResizeHandle ? t('common:resizePanels') : undefined}
          className="split-handle [.workspace-splitter_&]:[-webkit-app-region:no-drag] [.workspace-splitter.is-single-pane_&]:hidden"
          aria-label={t('common:resizePanels')}
        />
      )}
      <ResizablePanel
        id={`${id}-second`}
        {...(side === 'second' ? controlled : { minSize: minimumOther })}
      >
        <div
          className={secondClassName}
          style={{
            height: '100%',
            minWidth: 0,
            overflow: 'hidden',
            display: side === 'second' && collapsed ? 'none' : undefined
          }}
        >
          {second}
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}
