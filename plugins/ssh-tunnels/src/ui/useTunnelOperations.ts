import { useRef, useState } from 'react'

// Serialize actions per tunnel while allowing independent tunnels to operate concurrently.
export function useTunnelOperations(refresh: () => Promise<void>) {
  const [failure, setFailure] = useState('')
  const [pending, setPending] = useState<string[]>([])
  const locks = useRef(new Set<string>())
  async function perform(id: string, action: () => Promise<unknown>) {
    if (locks.current.has(id)) return
    locks.current.add(id)
    setPending([...locks.current])
    setFailure('')
    try {
      await action()
      await refresh()
    } catch (cause) {
      setFailure(String(cause))
      throw cause
    } finally {
      locks.current.delete(id)
      setPending([...locks.current])
    }
  }
  return { perform, pending, failure }
}
