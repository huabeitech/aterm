import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode
} from 'react'
import type { JsonValue, PluginViewAction } from '../../../../shared/contracts/plugins'
import { desktop } from '../../api/app'
import { applyViewAction, type OpenPluginView } from './viewActions'

interface OwnedView {
  pluginId: string
  sessionId: string
  view: OpenPluginView
}
interface ViewController {
  current?: OwnedView
  apply: (pluginId: string, sessionId: string, action: PluginViewAction) => void
  close: (pluginId: string, sessionId: string) => void
}
const Context = createContext<ViewController | null>(null)
const owns = (current: OwnedView | undefined, pluginId: string, sessionId: string) =>
  current?.pluginId === pluginId && current.sessionId === sessionId

/** One side-panel slot per workspace. Opening another plugin replaces the current view. */
export function PluginViewProvider({ children }: { children: ReactNode }) {
  const [current, setCurrent] = useState<OwnedView>()
  const close = useCallback((pluginId: string, sessionId: string) => {
    setCurrent((before) => (owns(before, pluginId, sessionId) ? undefined : before))
  }, [])
  const apply = useCallback((pluginId: string, sessionId: string, action: PluginViewAction) => {
    const invocationId = crypto.randomUUID()
    setCurrent((before) => {
      if (action.type === 'view.close' && !owns(before, pluginId, sessionId)) return before
      const view = applyViewAction(
        owns(before, pluginId, sessionId) ? before?.view : undefined,
        action,
        invocationId
      )
      return view ? { pluginId, sessionId, view } : undefined
    })
  }, [])
  return <Context.Provider value={{ current, apply, close }}>{children}</Context.Provider>
}

/** Commands from status items, menus and iframe content follow the same action path. */
export function usePluginViewController(pluginId: string, sessionId: string) {
  const controller = useContext(Context)
  if (!controller) throw Error('Plugin view controller requires a workspace provider')
  const { apply, close: closeOwnedView } = controller
  const generation = useRef(0)
  const request = useRef(0)
  const close = useCallback(() => {
    generation.current++
    closeOwnedView(pluginId, sessionId)
  }, [closeOwnedView, pluginId, sessionId])
  useLayoutEffect(() => {
    close()
    return () => {
      generation.current++
      closeOwnedView(pluginId, sessionId)
    }
  }, [pluginId, sessionId, close, closeOwnedView])
  const execute = useCallback(
    async (command: string, args?: JsonValue) => {
      const started = generation.current
      const sequence = ++request.current
      const result = await desktop.plugins.executeCommand(pluginId, sessionId, command, args)
      if (result.action && started === generation.current && sequence === request.current)
        apply(pluginId, sessionId, result.action)
      return result.data
    },
    [pluginId, sessionId, apply]
  )
  const view = owns(controller.current, pluginId, sessionId) ? controller.current?.view : undefined
  return { view, close, execute }
}

export function usePluginPanelOpen() {
  return !!useContext(Context)?.current
}
