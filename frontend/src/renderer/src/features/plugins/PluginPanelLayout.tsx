import { useState, type ReactNode } from 'react'
import { SplitLayout } from '../../components/ui/split-layout'

const widthKey = 'aterm.pluginPanel.width'
const minimumWidth = 280
function readWidth() {
  try {
    const saved = Number(localStorage.getItem(widthKey))
    if (Number.isFinite(saved) && saved > 0) return Math.max(minimumWidth, saved)
  } catch {
    // Resizing remains usable when local storage is unavailable.
  }
  return 360
}

/** Trusted layout policy shared by terminal and plugin workspace surfaces. */
export function PluginPanelLayout({
  children,
  open,
  onPanel
}: {
  children: ReactNode
  open: boolean
  onPanel: (panel: HTMLDivElement | null) => void
}) {
  const [width, setWidth] = useState(readWidth)
  return (
    <SplitLayout
      className="min-h-0 min-w-0 flex-1 workspace-splitter plugin-terminal-split"
      first={<div className="relative h-full min-h-0 min-w-0">{children}</div>}
      second={<div className="plugin-side-panel h-full" ref={onPanel} />}
      side="second"
      size={width}
      min={minimumWidth}
      max="70%"
      minimumOther={160}
      collapsed={!open}
      preservePixelSize
      withResizeHandle
      onResize={(sizes) => {
        setWidth(sizes[1])
        try {
          localStorage.setItem(widthKey, String(sizes[1]))
        } catch {
          // Layout state still lives in memory when persistence is unavailable.
        }
      }}
    />
  )
}
