import { createRoot } from 'react-dom/client'
import { useEffect, useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { i18n } from './i18n'
import { ConfirmationProvider } from '@/components/ui/confirm-provider'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Toaster } from '@/components/ui/toast'
import type { Host } from './types'
import { TunnelsPanel } from './TunnelsPanel'
import { useTunnels } from './useTunnels'
import './styles.css'

function applyEnvironment() {
  const { environment } = atermView.state
  for (const [key, value] of Object.entries(environment.tokens))
    document.documentElement.style.setProperty(key, value)
  document.documentElement.classList.toggle('dark', environment.appearance === 'dark')
  void i18n.changeLanguage(environment.locale)
}
function App() {
  const tunnels = useTunnels()
  const [hosts, setHosts] = useState<Host[]>([])
  const [hostID, setHostID] = useState<string>()
  useEffect(() => {
    let active = true
    const update = () => {
      applyEnvironment()
      const ctx = atermView.state.context.view
      setHostID(
        ctx && typeof ctx === 'object' && !Array.isArray(ctx) && typeof ctx.hostID === 'string'
          ? ctx.hostID
          : undefined,
      )
      if (atermView.state.context.active)
        void atermView.executeCommand('tunnels.request', { op: 'hosts' }).then((value) => {
          if (active) setHosts(value as unknown as Host[])
        })
    }
    update()
    const off = atermView.onDidChange(update)
    return () => {
      active = false
      off()
    }
  }, [])
  return <TunnelsPanel {...tunnels} hosts={hosts} hostID={hostID} unlock={async () => true} />
}
void atermView.ready.then(() => {
  applyEnvironment()
  createRoot(document.getElementById('root')!).render(
    <I18nextProvider i18n={i18n}>
      <TooltipProvider>
        <ConfirmationProvider>
          <App />
          <Toaster />
        </ConfirmationProvider>
      </TooltipProvider>
    </I18nextProvider>,
  )
})
