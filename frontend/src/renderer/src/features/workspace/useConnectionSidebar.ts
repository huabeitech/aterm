import { sidebarLayout, constrainSidebarWidth } from './sidebarLayout'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'

const SIDEBAR_WIDTH_KEY = 'aterm.connectionSidebar.width'
const SIDEBAR_COLLAPSED_KEY = 'aterm.connectionSidebar.collapsed'

function readSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === 'true'
  } catch {
    return false
  }
}

function readSidebarWidth(): number {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(SIDEBAR_WIDTH_KEY) ?? 'null')
    if (typeof stored === 'number' && Number.isFinite(stored) && stored > 0) {
      return constrainSidebarWidth(stored)
    }
  } catch {
    // Fall back to the default width if storage is unavailable or invalid.
  }
  return sidebarLayout.defaultWidth
}

function viewport() {
  return document.querySelector<HTMLElement>('.source-sidebar [data-slot="scroll-area-viewport"]')
}

export function useConnectionSidebar() {
  const [preferredWidth, setPreferredWidth] = useState(readSidebarWidth)
  const [collapsed, setCollapsed] = useState(readSidebarCollapsed)
  const collapsedRef = useRef(collapsed)
  const scrollTop = useRef(0)
  const returnFocusToToggle = useRef(false)

  useEffect(() => {
    try {
      localStorage.setItem(SIDEBAR_COLLAPSED_KEY, JSON.stringify(collapsed))
    } catch {
      // Toggling should still work when persistence is unavailable.
    }
  }, [collapsed])

  function toggle() {
    if (!collapsedRef.current) {
      scrollTop.current = viewport()?.scrollTop ?? 0
      returnFocusToToggle.current = !!document.activeElement?.closest('.source-sidebar')
    }
    collapsedRef.current = !collapsedRef.current
    setCollapsed(collapsedRef.current)
  }

  useLayoutEffect(() => {
    if (collapsed) {
      if (returnFocusToToggle.current)
        document.querySelector<HTMLButtonElement>('[data-action="toggle-sidebar"]')?.focus()
      return
    }
    const element = viewport()
    if (element) element.scrollTop = scrollTop.current
  }, [collapsed])

  function revealSearch() {
    collapsedRef.current = false
    setCollapsed(false)
    const deadline = performance.now() + 1000
    function focusSearch() {
      if (collapsedRef.current || performance.now() > deadline) return
      if (document.querySelector('.workspace-body[data-sidebar-animating]')) {
        requestAnimationFrame(focusSearch)
        return
      }
      const search = document.querySelector<HTMLInputElement>('[data-action="search-hosts"]')
      // A view switch may not have committed yet; wait until the drawer can take focus.
      if (search && (!search.getClientRects().length || search.closest('[inert]'))) {
        requestAnimationFrame(focusSearch)
        return
      }
      search?.focus({ preventScroll: true })
    }
    requestAnimationFrame(focusSearch)
  }

  function commitWidth(width: number) {
    if (!Number.isFinite(width) || width <= 0) return
    const next = constrainSidebarWidth(width)
    setPreferredWidth(next)
    try {
      localStorage.setItem(SIDEBAR_WIDTH_KEY, JSON.stringify(next))
    } catch {
      // Keep the user's adjustment usable when storage is unavailable.
    }
  }

  return { preferredWidth, collapsed, toggle, revealSearch, commitWidth }
}
