import { useEffect, useRef, useState } from 'react'
import { t } from '../../../../shared/i18n'
import type { PluginInfo } from '../../../../shared/contracts/plugins'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import { desktop } from '../../api/app'
import { PluginView } from './PluginView'
import { PluginPanelLayout } from './PluginPanelLayout'
import { PluginViewProvider, usePluginViewController } from './usePluginViewController'
import './plugins.css'
import { usePluginSession } from './usePluginSession'
import { pluginPresentation, pluginPresentationKeys } from './presentation'

/** Workspace contribution lifecycle is independent of terminal status bars. */
interface PluginWorkspaceProps {
  plugin: PluginInfo
  session: TerminalSession
  viewId: string
  active: boolean
  onClose: () => void
}
function PluginWorkspaceContent({
  plugin,
  session,
  viewId,
  active,
  onClose
}: PluginWorkspaceProps) {
  const [panel, setPanel] = useState<HTMLDivElement | null>(null)
  const [error, setError] = useState('')
  const initialized = useRef(false)
  const [sidePanel, setSidePanel] = useState<HTMLDivElement | null>(null)
  const id = plugin.manifest.id
  const { view, close, execute } = usePluginViewController(id, session.id)
  const {
    status,
    error: sessionError,
    available
  } = usePluginSession(
    id,
    session.id,
    active && session.state === 'running' && plugin.state !== 'failed'
  )
  useEffect(() => {
    initialized.current = false
  }, [id, session.id])
  if (status?.state === 'ready') initialized.current = true
  const presentation = pluginPresentation(session.state, plugin.state, status?.state)
  useEffect(close, [
    active,
    plugin.manifest.version,
    plugin.state,
    session.state,
    status?.state === 'permission-required',
    close
  ])
  const ready =
    session.state === 'running' &&
    (status?.state === 'ready' || (initialized.current && status?.state === 'loading')) &&
    plugin.state !== 'failed'
  return (
    <PluginPanelLayout open={!!view} onPanel={setSidePanel}>
      <div className="plugin-workspace" ref={setPanel}>
        {ready && panel ? (
          <PluginView
            workspace
            active={active && available}
            plugin={plugin}
            session={session}
            status={status}
            view={{ id: viewId, invocationId: session.id }}
            panel={panel}
            onClose={onClose}
            onCommand={execute}
          />
        ) : (
          <div className="connection-empty" role="status">
            <span>
              {error || plugin.error || sessionError || t(pluginPresentationKeys[presentation])}
            </span>
            {status?.state === 'permission-required' && (
              <button
                onClick={() =>
                  void desktop.plugins.authorize(id, session.id).catch((e) => setError(String(e)))
                }
              >
                {t('settings:pluginsAllowHost')}
              </button>
            )}
          </div>
        )}
        {view && sidePanel && active && (
          <PluginView
            plugin={plugin}
            session={session}
            status={status}
            view={view}
            panel={sidePanel}
            onClose={close}
            onCommand={execute}
          />
        )}
      </div>
    </PluginPanelLayout>
  )
}

export function PluginWorkspace(props: PluginWorkspaceProps) {
  return (
    <PluginViewProvider>
      <PluginWorkspaceContent {...props} />
    </PluginViewProvider>
  )
}
