import { useState, type ReactNode } from 'react'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import { usePluginCatalog } from './pluginCatalog'
import { PluginStatusBar } from './PluginStatusBar'
import { PluginPanelLayout } from './PluginPanelLayout'
import { PluginViewProvider, usePluginPanelOpen } from './usePluginViewController'

interface TerminalPluginWorkspaceProps {
  session?: TerminalSession
  visible: boolean
  children: ReactNode
}
function TerminalPluginContent({ session, visible, children }: TerminalPluginWorkspaceProps) {
  const [panel, setPanel] = useState<HTMLDivElement | null>(null)
  const open = usePluginPanelOpen()
  return (
    <div
      className="flex h-full min-h-0 min-w-0 flex-col"
      style={{ display: visible ? 'flex' : 'none' }}
    >
      {session && <TerminalPluginBars session={session} visible={visible} panel={panel} />}
      <PluginPanelLayout open={open} onPanel={setPanel}>
        {children}
      </PluginPanelLayout>
    </div>
  )
}
function TerminalPluginBars({
  session,
  visible,
  panel
}: {
  session: TerminalSession
  visible: boolean
  panel: HTMLElement | null
}) {
  const plugins = usePluginCatalog()
  return (
    <>
      {plugins
        .filter(
          (p) =>
            p.enabled &&
            p.manifest.contributes.terminalStatusBar &&
            (!p.manifest.execution || p.manifest.execution.targets.includes(session.kind))
        )
        .map((plugin) => (
          <PluginStatusBar
            key={plugin.manifest.id}
            plugin={plugin}
            session={session}
            visible={visible}
            panel={panel}
          />
        ))}
    </>
  )
}

export function TerminalPluginWorkspace(props: TerminalPluginWorkspaceProps) {
  return (
    <PluginViewProvider>
      <TerminalPluginContent {...props} />
    </PluginViewProvider>
  )
}
