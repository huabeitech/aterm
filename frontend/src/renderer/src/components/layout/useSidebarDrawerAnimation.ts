import { useLayoutEffect, useRef, type RefObject } from 'react'

// The grid owns geometry; the animation only translates the drawer. Nothing here
// measures or saves a preference, including when an animation is interrupted.
export function useSidebarDrawerAnimation(
  body: RefObject<HTMLDivElement>,
  drawer: RefObject<HTMLDivElement>,
  hidden: boolean,
  width: number,
  right: boolean
) {
  const previousHidden = useRef(hidden)
  const interrupted = useRef<number | null>(null)
  useLayoutEffect(() => {
    const sidebar = drawer.current!
    // Restore the requested terminal state after an interrupted animation cleans up.
    sidebar.style.display = hidden ? 'none' : ''
    if (previousHidden.current === hidden) {
      interrupted.current = null
      return
    }
    previousHidden.current = hidden
    const container = body.current!
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (motion.matches) {
      interrupted.current = null
      return
    }
    const drawerOffset = right ? width : -width
    const from = interrupted.current ?? (hidden ? 0 : drawerOffset)
    interrupted.current = null
    sidebar.style.display = 'grid'
    container.setAttribute('data-sidebar-animating', '')
    // Keep the final grid width throughout the slide so responsive content does
    // not reflow a second time when the animation finishes.
    const options: KeyframeAnimationOptions = {
      duration: 280,
      easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
      fill: 'both'
    }
    const animation = sidebar.animate(
      [
        { transform: `translateX(${from}px)` },
        { transform: `translateX(${hidden ? drawerOffset : 0}px)` }
      ],
      options
    )
    let finished = false
    function finish() {
      if (finished) return
      finished = true
      animation.cancel()
      sidebar.style.display = hidden ? 'none' : ''
      container.removeAttribute('data-sidebar-animating')
    }
    void animation.finished.then(finish, () => {})
    const motionChanged = () => {
      if (motion.matches) finish()
    }
    motion.addEventListener('change', motionChanged)
    return () => {
      if (!finished) {
        const transform = getComputedStyle(sidebar).transform
        interrupted.current = transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).m41
      }
      finish()
      motion.removeEventListener('change', motionChanged)
    }
  }, [body, drawer, hidden, width, right])
}
