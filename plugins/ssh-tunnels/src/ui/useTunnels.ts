import { useCallback, useEffect, useRef, useState } from 'react'
import { desktop } from './api'
import type { TunnelStatus } from './types'

export function useTunnels() {
  const [tunnels, setTunnels] = useState<TunnelStatus[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const revision = useRef(0)
  const refresh = useCallback(async () => {
    const request = ++revision.current
    try {
      const next = await desktop.tunnels.list()
      if (request === revision.current) {
        setTunnels(next)
        setError('')
      }
    } catch (cause) {
      if (request === revision.current)
        setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      if (request === revision.current) setLoading(false)
    }
  }, [])
  useEffect(() => {
    let timer: number | undefined
    function synchronize() {
      window.clearInterval(timer)
      if (atermView.state.context.active) {
        void refresh()
        timer = window.setInterval(() => void refresh(), 1000)
      }
    }
    const unsubscribe = desktop.tunnels.onChanged(synchronize)
    synchronize()
    return () => {
      window.clearInterval(timer)
      unsubscribe()
      revision.current++
    }
  }, [refresh])
  return { tunnels, loading, error, refresh }
}
