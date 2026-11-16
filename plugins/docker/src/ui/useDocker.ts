import { executionError } from './i18n'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { JsonValue, PluginViewState } from '../../../../frontend/packages/plugin-sdk'
// The UI only talks to this plugin's declared commands. No host components or IPC imports.
export interface Container {
  ID: string
  Names: string
  Image: string
  State: string
  Status: string
  Ports: string
}
export interface Engine {
  name: string
  context: string
  endpoint: string
  version: string
  engine: string
}
export interface ContainerDetail {
  Id: string
  Image: string
  Created: string
  State?: { Status: string; Health?: { Status: string } }
  RestartPolicy?: { Name: string }
  Ports?: Record<string, { HostIp: string; HostPort: string }[] | null>
  Mounts?: { Source: string; Destination: string; RW: boolean }[]
  Networks?: string[]
}
export type Operation = 'start' | 'stop' | 'restart' | 'remove' | 'exec'
export function useDocker(view: PluginViewState, resource: 'containers' | 'images' = 'containers') {
  const [logRun, setLogRun] = useState(0)
  const [engine, setEngine] = useState<Engine | null>(null),
    [containers, setContainers] = useState<Container[]>([])
  const [selected, setSelected] = useState<string | null>(null),
    [detail, setDetail] = useState<ContainerDetail | null>(null)
  const [tab, setTab] = useState('overview'),
    [logs, setLogs] = useState(''),
    [following, setFollowing] = useState(true),
    [logTruncated, setLogTruncated] = useState(false),
    [logEnded, setLogEnded] = useState(false)
  const [busy, setBusy] = useState(false),
    [ready, setReady] = useState(false),
    [error, setError] = useState(''),
    [detailError, setDetailError] = useState('')
  const active = view.context.active !== false
  const current = useRef({ engine, selected, tab, following, active })
  current.current = { engine, selected, tab, following, active }
  const epoch = useRef(0),
    manual = useRef(false),
    polling = useRef(false),
    logSubscription = useRef<string | null>(null),
    logSequence = useRef(0),
    queue = useRef<Promise<unknown>>(Promise.resolve())
  const request = useCallback(
    async <T>(operation: string, id?: string, shell?: 'sh' | 'bash'): Promise<T> => {
      const args: Record<string, JsonValue> = { operation }
      if (operation !== 'connect' && current.current.engine)
        Object.assign(args, {
          context: current.current.engine.context,
          engine: current.current.engine.engine
        })
      if (id) args.id = id
      if (shell) args.shell = shell
      const version = epoch.current
      // Serialize transport requests, but never drop a user action because a poll is in flight.
      const next = queue.current.then(async () => {
        if (
          version !== epoch.current ||
          !current.current.active ||
          ((operation === 'logs' || operation === 'inspect') && id !== current.current.selected)
        )
          throw Error('Request cancelled')
        const result = (await atermView.executeCommand('docker.request', args)) as {
          error?: string
          data?: T
          terminalSessionId?: string
        }
        if (result?.error) throw Error(executionError(result.error, operation))
        return (result?.terminalSessionId ? result : result?.data) as T
      })
      queue.current = next.then(
        () => undefined,
        () => undefined
      )
      return next
    },
    []
  )
  const refreshSelection = useCallback(
    async (version: number, id: string) => {
      const value = await request<ContainerDetail>('inspect', id)
      if (version !== epoch.current || current.current.selected !== id) return
      setDetail(value)
      setDetailError('')
    },
    [request]
  )
  const refresh = useCallback(
    async (reconnect = false) => {
      if (!current.current.active || manual.current) return
      manual.current = true
      setBusy(true)
      setError('')
      const version = epoch.current
      try {
        if (reconnect || !current.current.engine) {
          if (reconnect) {
            setSelected(null)
            current.current.selected = null
            setDetail(null)
            setLogs('')
          }
          const next = await request<Engine>('connect')
          if (version !== epoch.current) return
          current.current.engine = next
          setEngine(next)
        }
        const rows = await request<Container[]>('list')
        if (version !== epoch.current) return
        setContainers(rows)
        setReady(true)
        const id = current.current.selected
        if (id && rows.some((c) => c.ID === id)) await refreshSelection(version, id)
        else if (id) {
          setSelected(null)
          current.current.selected = null
          setDetail(null)
        }
      } catch (e) {
        if (version === epoch.current) setError(String((e as Error).message))
      } finally {
        if (version === epoch.current) {
          manual.current = false
          setBusy(false)
        }
      }
    },
    [request, refreshSelection]
  )
  useEffect(() => {
    epoch.current++
    manual.current = false
    polling.current = false
    setBusy(false)
    if (!active) return
    void refresh()
    return () => {
      epoch.current++
      manual.current = false
      polling.current = false
    }
  }, [active, refresh])
  useEffect(() => {
    if (!active || resource !== 'containers') return
    void refresh()
    const timer = setInterval(async () => {
      if (manual.current || polling.current || !current.current.engine) return
      polling.current = true
      const version = epoch.current
      try {
        const rows = await request<Container[]>('list')
        if (version !== epoch.current) return
        setContainers(rows)
        const id = current.current.selected
        if (id && !rows.some((c) => c.ID === id)) {
          setSelected(null)
          current.current.selected = null
          setDetail(null)
        }
      } catch (e) {
        if (version === epoch.current) setError((e as Error).message)
      } finally {
        if (version === epoch.current) polling.current = false
      }
    }, 3000)
    return () => clearInterval(timer)
  }, [active, resource, refresh, request])
  const choose = useCallback(
    (id: string | null) => {
      current.current.selected = id
      setSelected(id)
      setDetail(null)
      setLogs('')
      setLogTruncated(false)
      setDetailError('')
      if (!id) return
      const version = epoch.current
      void refreshSelection(version, id).catch((e) => {
        if (epoch.current === version && current.current.selected === id)
          setDetailError((e as Error).message)
      })
    },
    [refreshSelection]
  )
  useEffect(() => {
    if (
      !active ||
      resource !== 'containers' ||
      !selected ||
      tab !== 'logs' ||
      !following ||
      !engine
    )
      return
    const subscription = `log-${Date.now()}-${++logSequence.current}`
    const version = epoch.current
    logSubscription.current = subscription
    setLogEnded(false)
    setError('')
    let disposed = false
    const command = (action: 'start' | 'stop') =>
      atermView.executeCommand('docker.logs', {
        action,
        subscription,
        id: selected,
        context: engine.context,
        engine: engine.engine
      })
    void command('start')
      .then((result) => {
        const error = (result as { error?: string } | null)?.error
        if (error) throw Error(executionError(error, 'logs'))
      })
      .catch((error) => {
        if (!disposed && version === epoch.current) {
          setLogEnded(true)
          setError((error as Error).message)
        }
      })
    return () => {
      disposed = true
      if (logSubscription.current === subscription) logSubscription.current = null
      void command('stop').catch(() => {
        /* Source lifetime also cancels streams on the host. */
      })
    }
  }, [selected, tab, active, resource, following, logRun, engine?.context, engine?.engine])
  useEffect(() => {
    const data = view.status?.data as
      | {
          dockerLogs?: {
            subscription: string
            id: string
            text: string
            truncated: boolean
            ended: boolean
            error?: string
          }
        }
      | undefined
    const output = data?.dockerLogs
    if (!output || output.subscription !== logSubscription.current || output.id !== selected) return
    setLogs(output.text)
    setLogTruncated(output.truncated)
    setLogEnded(output.ended)
    if (output.error) setError(executionError(output.error, 'logs'))
  }, [view.status?.data, selected])
  const perform = useCallback(
    async (operation: Operation, shell?: 'sh' | 'bash') => {
      const id = current.current.selected
      if (!id || !current.current.active || manual.current) return
      manual.current = true
      setBusy(true)
      setError('')
      const version = epoch.current
      try {
        await request(operation, id, shell)
        if (version !== epoch.current || operation === 'exec') return
        if (operation === 'remove') {
          choose(null)
        }
        const rows = await request<Container[]>('list')
        if (version !== epoch.current) return
        setContainers(rows)
        if (operation !== 'remove') await refreshSelection(version, id)
        if (operation === 'start' || operation === 'restart') setLogRun((n) => n + 1)
      } catch (e) {
        if (version === epoch.current) setError((e as Error).message)
      } finally {
        if (version === epoch.current) {
          manual.current = false
          setBusy(false)
        }
      }
    },
    [choose, refreshSelection, request]
  )
  const resumeLogs = useCallback(() => {
    setFollowing(true)
    setLogRun((n) => n + 1)
  }, [])
  return {
    engine,
    containers,
    selected: containers.find((c) => c.ID === selected) ?? null,
    detail,
    tab,
    setTab,
    logs,
    logTruncated,
    logEnded,
    following,
    setFollowing,
    resumeLogs,
    busy,
    ready,
    error,
    detailError,
    active,
    refresh,
    choose,
    perform,
    request
  }
}
