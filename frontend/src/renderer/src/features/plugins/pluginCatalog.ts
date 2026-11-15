import { useSyncExternalStore } from 'react'
import type { PluginInfo } from '../../../../shared/contracts/plugins'
import { desktop } from '../../api/app'
let snapshot: PluginInfo[] = []
const listeners = new Set<() => void>()
let stop: (() => void) | undefined
function subscribe(listener: () => void) {
  listeners.add(listener)
  if (!stop) {
    let active = true
    const load = () =>
      void desktop.plugins
        .list()
        .then((value) => {
          if (active) {
            snapshot = value
            listeners.forEach((notify) => notify())
          }
        })
        .catch(() => {})
    const unsubscribe = desktop.plugins.onChanged(load)
    stop = () => {
      active = false
      unsubscribe()
    }
    load()
  }
  return () => {
    listeners.delete(listener)
    if (!listeners.size) {
      stop?.()
      stop = undefined
    }
  }
}
export function usePluginCatalog() {
  return useSyncExternalStore(subscribe, () => snapshot)
}
