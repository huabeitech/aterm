import { useState } from 'react'
import { sidebarLayout, constrainSidebarWidth } from './sidebarLayout'

export function useSidebarWidth(storageKey: string, defaultWidth: number = sidebarLayout.defaultWidth) {
  const [preferredWidth, setPreferredWidth] = useState(() => {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(storageKey) ?? 'null')
      if (typeof stored === 'number' && Number.isFinite(stored) && stored > 0) {
        return constrainSidebarWidth(stored)
      }
    } catch {
      // Fall back when storage is unavailable or invalid.
    }
    return defaultWidth
  })

  function commitWidth(width: number) {
    if (!Number.isFinite(width) || width <= 0) return
    const next = constrainSidebarWidth(width)
    setPreferredWidth(next)
    try {
      localStorage.setItem(storageKey, JSON.stringify(next))
    } catch {
      // Keep resizing usable when storage is unavailable.
    }
  }

  return { preferredWidth, commitWidth }
}
