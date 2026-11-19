import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import { sidebarLayout, constrainSidebarWidth } from '../../features/workspace/sidebarLayout'
import { useSidebarDrawerAnimation } from './useSidebarDrawerAnimation'

type Props = {
  side?: 'left' | 'right'
  sidebarID?: string
  preferredWidth: number
  hidden: boolean
  onWidthCommit: (width: number) => void
  sidebar: ReactNode
  children: ReactNode
}

// Preferences flow into layout. Only explicit pointer/keyboard actions flow back out.
export function SidebarLayout({ side = 'left', sidebarID, preferredWidth, hidden, onWidthCommit, sidebar, children }: Props) {
  useTranslation()
  const right = side === 'right'
  const direction = right ? -1 : 1
  const root = useRef<HTMLDivElement>(null)
  const drawer = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  const drag = useRef<{ pointerId: number; startX: number; startWidth: number } | null>(null)
  const [availableWidth, setAvailableWidth] = useState<number | null>(null)
  const [dragWidth, setDragWidth] = useState<number | null>(null)
  const maximum =
    availableWidth === null
      ? sidebarLayout.maxWidth
      : Math.max(
          sidebarLayout.minWidth,
          Math.min(sidebarLayout.maxWidth, availableWidth - (right ? 440 : sidebarLayout.minimumContentWidth) - 1)
        )
  const width = Math.min(dragWidth ?? preferredWidth, maximum)
  useLayoutEffect(() => {
    const element = root.current!
    const observer = new ResizeObserver(() => setAvailableWidth(element.clientWidth))
    setAvailableWidth(element.clientWidth)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  useLayoutEffect(() => {
    if (hidden) {
      drag.current = null
      setDragWidth(null)
    }
    drawer.current!.inert = hidden
  }, [hidden])
  useSidebarDrawerAnimation(root, drawer, content, hidden, width, right)

  function commit(value: number) {
    onWidthCommit(Math.min(constrainSidebarWidth(value), maximum))
  }

  return (
    <div ref={root} className="workspace-body h-full min-w-0 min-h-0 overflow-hidden">
      <div
        className="workspace-splitter grid h-full min-w-0 min-h-0"
        style={{
          gridTemplateColumns: right
            ? `minmax(0, 1fr) ${hidden ? 0 : 1}px ${hidden ? 0 : width}px`
            : `${hidden ? 0 : width}px ${hidden ? 0 : 1}px minmax(0, 1fr)`
        }}
      >
        <div
          ref={drawer}
          id={sidebarID}
          className={`row-start-1 ${right ? 'col-start-3' : 'source-sidebar col-start-1'} relative z-[1] grid grid-rows-[minmax(0,_1fr)_auto] min-w-0 min-h-0 overflow-hidden bg-[var(--hosts-background)] [-webkit-app-region:no-drag]`}
          style={{ width, display: hidden ? 'none' : undefined }}
        >
          {sidebar}
        </div>
        {!hidden ? (
          <div
            role="separator"
            aria-controls={sidebarID}
            aria-orientation="vertical"
            aria-label={t('common:resizePanels')}
            aria-valuemin={sidebarLayout.minWidth}
            aria-valuemax={maximum}
            aria-valuenow={Math.round(width)}
            tabIndex={0}
            className="split-handle row-start-1 col-start-2 relative flex items-center justify-center bg-border outline-none focus-visible:ring-1 focus-visible:ring-ring after:absolute after:inset-y-0 after:left-1/2 after:-translate-x-1/2 touch-none [-webkit-app-region:no-drag]"
            onPointerDown={(event) => {
              if (event.button !== 0 || root.current?.hasAttribute('data-sidebar-animating')) return
              event.preventDefault()
              event.currentTarget.focus()
              event.currentTarget.setPointerCapture(event.pointerId)
              drag.current = {
                pointerId: event.pointerId,
                startX: event.clientX,
                startWidth: width
              }
            }}
            onPointerMove={(event) => {
              const current = drag.current
              if (!current || current.pointerId !== event.pointerId) return
              setDragWidth(
                Math.min(
                  constrainSidebarWidth(current.startWidth + direction * (event.clientX - current.startX)),
                  maximum
                )
              )
            }}
            onPointerUp={(event) => {
              const current = drag.current
              if (!current || current.pointerId !== event.pointerId) return
              if (event.clientX !== current.startX)
                commit(current.startWidth + direction * (event.clientX - current.startX))
              drag.current = null
              setDragWidth(null)
              event.currentTarget.releasePointerCapture(event.pointerId)
            }}
            onLostPointerCapture={() => {
              drag.current = null
              setDragWidth(null)
            }}
            onKeyDown={(event) => {
              const next =
                event.key === 'Home'
                  ? sidebarLayout.minWidth
                  : event.key === 'End'
                    ? maximum
                    : event.key === 'ArrowLeft'
                      ? width - direction * 10
                      : event.key === 'ArrowRight'
                        ? width + direction * 10
                        : null
              if (next === null) return
              event.preventDefault()
              commit(next)
            }}
          >
            <div />
          </div>
        ) : (
          <div className="row-start-1 col-start-2" />
        )}
        <div
          ref={content}
          className={`workspace-content row-start-1 ${right ? 'col-start-1' : 'col-start-3'} min-w-0 min-h-0 relative overflow-hidden`}
        >
          {children}
        </div>
      </div>
    </div>
  )
}
