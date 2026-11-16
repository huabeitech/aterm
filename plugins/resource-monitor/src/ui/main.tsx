import { useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { ResourceDetails, type ResourceGroup, type MonitorPreferences } from './ResourceDetails'
import type { ResourceSnapshot } from '../resources'
import type { ResourcePoint } from './resourceModel'
import type { PluginViewApi, PluginViewState } from '../../../../frontend/packages/plugin-sdk'
import './details.css'

declare const atermView: PluginViewApi
interface MonitorData {
  resources?: ResourceSnapshot
  history: ResourcePoint[]
  warnings: string[]
  preference: MonitorPreferences
  interval: number
}
function App() {
  const [state, setState] = useState<PluginViewState>(atermView.state)
  const [section, setSection] = useState<ResourceGroup>('system')
  const [error, setError] = useState('')
  const data = state.status?.data as unknown as MonitorData | undefined
  useEffect(() => atermView.onDidChange(setState), [])
  useEffect(() => {
    const requested = (state.context.view as { section?: ResourceGroup })?.section ?? 'system'
    setSection(requested)
  }, [state.context.invocationId])
  useEffect(() => {
    document
      .querySelector(`[data-resource-section="${section}"]`)
      ?.scrollIntoView({ block: 'start' })
  }, [section])
  const loading = !state.status || state.status.state === 'loading'
  const stale = state.status?.state !== 'ready'
  return (
    <ResourceDetails
      t={atermView.t}
      resources={data?.resources}
      history={data?.history ?? []}
      interval={data?.interval ?? 5}
      sessionLabel={state.context.sessionLabel}
      state={atermView.t(loading ? 'monitorLoading' : stale ? 'pluginsDelayed' : 'monitorLive')}
      section={section}
      onSection={setSection}
      warnings={data?.warnings ?? []}
      error={error}
      preference={data?.preference ?? {}}
      select={(patch) => {
        setError('')
        void atermView
          .executeCommand(
            'monitor.select',
            Object.fromEntries(Object.entries(patch).map(([key, value]) => [key, value ?? null]))
          )
          .catch((err) => setError(String(err)))
      }}
    />
  )
}
void atermView.ready.then(() => createRoot(document.getElementById('root')!).render(<App />))
