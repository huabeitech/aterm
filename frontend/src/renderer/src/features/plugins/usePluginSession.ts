import { useEffect, useId, useState } from 'react'
import type { PluginStatus } from '../../../../shared/contracts/plugins'
import { desktop } from '../../api/app'

/** Each surface holds its own demand; hiding one cannot stop another visible surface. */
export function usePluginSession(pluginId: string, sessionId: string, active: boolean) {
  const consumer = useId()
  const [status, setStatus] = useState<PluginStatus>()
  const [error, setError] = useState('')
  const [available, setAvailable] = useState(false)
  useEffect(() => {
    setStatus(undefined)
    setError('')
    return desktop.plugins.onStatus((value) => {
      if (value.pluginId === pluginId && value.sessionId === sessionId) setStatus(value)
    })
  }, [pluginId, sessionId])
  useEffect(() => {
    let cancelled = false
    setAvailable(false)
    setError('')
    if (active) {
      void desktop.plugins
        .visibility(pluginId, sessionId, consumer, true)
        .then(() => {
          if (!cancelled) setAvailable(true)
        })
        .catch((err) => {
          if (!cancelled) setError(String(err))
        })
    }
    return () => {
      cancelled = true
      if (active)
        void desktop.plugins.visibility(pluginId, sessionId, consumer, false).catch(() => {})
    }
  }, [pluginId, sessionId, consumer, active])
  return { status, error, available }
}
