import { useEffect, useId, useState } from 'react'
import type { PluginInfo, PluginStatus } from '../../../../shared/contracts/plugins'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import { desktop } from '../../api/app'

/** Passive discovery never requests new grants. Unknown availability keeps an authorization entry. */
export function useWorkspaceAvailability(
  catalog: PluginInfo[],
  session: TerminalSession,
  active: boolean
) {
  const consumer = useId()
  const [states, setStates] = useState<Record<string, PluginStatus['views']>>({})
  const ids = catalog
    .filter(
      (plugin) =>
        plugin.enabled &&
        plugin.manifest.contributes.views?.some((view) => view.availability === 'dynamic')
    )
    .map((plugin) => plugin.manifest.id)
    .sort()
    .join(',')
  useEffect(() => {
    setStates({})
    return desktop.plugins.onStatus((status) => {
      if (status.sessionId !== session.id) return
      setStates((previous) => {
        const views = status.views
        if (!views && status.state === 'loading') return previous
        if (JSON.stringify(previous[status.pluginId]) === JSON.stringify(views)) return previous
        return { ...previous, [status.pluginId]: views }
      })
    })
  }, [session.id, session.state, ids])
  useEffect(() => {
    if (!active || session.state !== 'running' || !ids) return
    const plugins = ids.split(',')
    for (const id of plugins)
      void desktop.plugins.visibility(id, session.id, consumer, true, true).catch(() => {})
    return () => {
      for (const id of plugins)
        void desktop.plugins.visibility(id, session.id, consumer, false, true).catch(() => {})
    }
  }, [ids, session.id, session.state, active, consumer])
  return states
}
