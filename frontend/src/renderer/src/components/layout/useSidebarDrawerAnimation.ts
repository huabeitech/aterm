import { useLayoutEffect, useRef, type RefObject } from 'react'

// The grid owns geometry; animations only translate its contents. Nothing here
// measures or saves a preference, including when an animation is interrupted.
export function useSidebarDrawerAnimation(
  body: RefObject<HTMLDivElement>,
  drawer: RefObject<HTMLDivElement>,
  content: RefObject<HTMLDivElement>,
  hidden: boolean,
  width: number
) {
  const previousHidden = useRef(hidden)
  const interrupted = useRef<{ drawer: number; content: number } | null>(null)
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
    const workspace = content.current!
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (motion.matches) {
      interrupted.current = null
      return
    }
    const offset = width + 1
    const from = interrupted.current ?? {
      drawer: hidden ? 0 : -width,
      content: hidden ? offset : -offset
    }
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
    const animations = [
      sidebar.animate(
        [
          { transform: `translateX(${from.drawer}px)` },
          { transform: `translateX(${hidden ? -width : 0}px)` }
        ],
        options
      ),
      workspace.animate(
        [{ transform: `translateX(${from.content}px)` }, { transform: 'translateX(0)' }],
        options
      )
    ]
    let finished = false
    function finish() {
      if (finished) return
      finished = true
      animations.forEach((animation) => animation.cancel())
      sidebar.style.display = hidden ? 'none' : ''
      container.removeAttribute('data-sidebar-animating')
    }
    void Promise.all(animations.map((animation) => animation.finished)).then(finish, () => {})
    const motionChanged = () => {
      if (motion.matches) finish()
    }
    motion.addEventListener('change', motionChanged)
    return () => {
      if (!finished) {
        const translation = (element: HTMLElement) => {
          const transform = getComputedStyle(element).transform
          return transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).m41
        }
        // Content's grid origin moves when visibility changes again.
        interrupted.current = {
          drawer: translation(sidebar),
          content: translation(workspace) + (hidden ? -offset : offset)
        }
      }
      finish()
      motion.removeEventListener('change', motionChanged)
    }
  }, [body, drawer, content, hidden, width])
}
