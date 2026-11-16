import { useCallback, useEffect, useRef, useState } from 'react'
import { desktop } from '../../api/app'
import type { TunnelStatus } from '../../../../shared/contracts/tunnels'

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
    const unsubscribe = desktop.tunnels.onChanged(() => void refresh())
    void refresh()
    return () => {
      unsubscribe()
      revision.current++
    }
  }, [refresh])
  return { tunnels, loading, error, refresh }
}
