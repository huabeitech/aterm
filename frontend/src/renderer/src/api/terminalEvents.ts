import type { TerminalEvent } from '../../../shared/contracts/terminal'

type Listener = { callback: (event: TerminalEvent) => void }

// Route before crossing contextBridge: a data packet only reaches its pane and
// explicit global subscribers, regardless of how many other panes are open.
export function terminalEvents(connect: (dispatch: (event: TerminalEvent) => void) => () => void) {
  const global = new Set<Listener>()
  const sessions = new Map<string, Set<Listener>>()
  let stop: (() => void) | undefined
  let count = 0
  const deliver = (listeners: Set<Listener> | undefined, event: TerminalEvent) => {
    for (const listener of [...(listeners ?? [])]) {
      if (!listeners?.has(listener)) continue
      try {
        listener.callback(event)
      } catch (error) {
        console.error('Terminal event subscriber failed', error)
      }
    }
  }
  const dispatch = (event: TerminalEvent) => {
    deliver(global, event)
    if (event.type === 'accessibility') {
      for (const listeners of [...sessions.values()]) deliver(listeners, event)
    } else {
      const id =
        event.type === 'state'
          ? event.session.id
          : event.type === 'prompt'
            ? event.prompt.sessionID
            : event.type === 'prompt-closed'
              ? undefined
              : event.id
      if (id) deliver(sessions.get(id), event)
    }
  }
  return (callback: (event: TerminalEvent) => void, sessionID?: string) => {
    const listeners =
      sessionID === undefined ? global : (sessions.get(sessionID) ?? new Set<Listener>())
    if (sessionID !== undefined) sessions.set(sessionID, listeners)
    const listener = { callback }
    listeners.add(listener)
    count++
    stop ??= connect(dispatch)
    let active = true
    return () => {
      if (!active) return
      active = false
      listeners.delete(listener)
      if (sessionID !== undefined && !listeners.size) sessions.delete(sessionID)
      if (--count === 0) {
        stop?.()
        stop = undefined
      }
    }
  }
}
