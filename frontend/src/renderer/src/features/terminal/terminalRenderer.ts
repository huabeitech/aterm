import type { Terminal } from '@xterm/xterm'
import type { WebglAddon } from '@xterm/addon-webgl'

// addon-webgl 0.19 does not dispose this timer (upstream xterm.js#5817).
// Keep the version-specific private access here, away from terminal behavior.
function disposeAddon(addon: WebglAddon) {
  try {
    ;(
      addon as unknown as {
        _renderer?: { _cursorBlinkStateManager?: { dispose(): void } }
      }
    )._renderer?._cursorBlinkStateManager?.dispose()
  } finally {
    addon.dispose()
  }
}

export function terminalRenderer(
  term: Pick<Terminal, 'loadAddon' | 'refresh' | 'rows'>,
  visible: () => boolean,
  load = async () => {
    const { WebglAddon } = await import('@xterm/addon-webgl')
    return new WebglAddon()
  }
) {
  let addon: WebglAddon | undefined
  let disposed = false
  let loading = false
  let failures = 0
  let enabled = true
  let stableTimer: ReturnType<typeof setTimeout> | undefined
  let contextListener: { dispose(): void } | undefined

  const release = () => {
    clearTimeout(stableTimer)
    stableTimer = undefined
    contextListener?.dispose()
    contextListener = undefined
    const previous = addon
    addon = undefined
    if (previous) disposeAddon(previous)
  }
  const refresh = () => {
    if (disposed || !visible()) return
    term.refresh(0, term.rows - 1)
    // Count consecutive failures, not lifetime allocations. Successful renderers
    // replenish the budget after 30 seconds, while rapid GPU resets stay bounded.
    if (!enabled || addon || loading || failures >= 3) return
    loading = true
    void load()
      .then((next) => {
        if (disposed || !enabled || !visible()) {
          disposeAddon(next)
          return
        }
        addon = next
        contextListener = next.onContextLoss(() => {
          // Do not replace a renderer in the middle of its event dispatch.
          queueMicrotask(() => {
            if (disposed || addon !== next) return
            failures++
            release()
            refresh()
          })
        })
        term.loadAddon(next)
        stableTimer = setTimeout(() => {
          if (addon === next && !disposed) failures = 0
        }, 30000)
      })
      .catch((error: unknown) => {
        if (disposed) return
        failures++
        release()
        console.warn('Terminal WebGL unavailable; using DOM rendering.', error)
      })
      .finally(() => {
        loading = false
      })
  }
  return {
    refresh,
    refreshAfterResize: () => {
      if (disposed || !visible()) return
      // xterm 6 / addon-webgl 0.19 clears the canvas synchronously on resize,
      // but refresh() queues drawing for the next animation frame. Render through
      // the service now so the browser cannot present the cleared canvas. Keep
      // this version-specific access here; the service also honors synchronized
      // output mode and updates selection and render listeners.
      const service = (
        term as unknown as {
          _core?: { _renderService?: { _renderRows?: (start: number, end: number) => void } }
        }
      )._core?._renderService
      if (service?._renderRows) service._renderRows(0, term.rows - 1)
      else term.refresh(0, term.rows - 1)
    },
    setEnabled: (value: boolean) => {
      if (disposed) return
      enabled = value
      release()
      failures = 0
      refresh()
    },
    clearTextureAtlas: () => addon?.clearTextureAtlas(),
    dispose: () => {
      disposed = true
      release()
    }
  }
}
