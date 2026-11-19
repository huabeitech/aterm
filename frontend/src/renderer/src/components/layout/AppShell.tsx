import { t } from '../../../../shared/i18n'
import { PanelLeft, PanelRight } from 'lucide-react'
import { IconButton, Tip } from '@/components/ui/desktop'
import { formatCommandPaletteShortcut } from '../common/commandPaletteShortcut'
import { cn } from '@/lib/utils'
import { type MouseEvent, type ReactNode } from 'react'
import { SidebarLayout } from './SidebarLayout'
import { WindowToggleMaximise } from '../../api/runtime'

export type WorkspaceMode = 'home' | 'terminal' | 'files' | 'settings'

type AppShellProps = {
  style: React.CSSProperties
  sidebarWidth: number
  sidebarCollapsed: boolean
  onToggleSidebar: () => void
  toggleSidebarShortcut: string
  mode: WorkspaceMode
  platform: string
  header: ReactNode
  sidebar: ReactNode
  content: ReactNode
  inspector: ReactNode
  inspectorWidth: number
  inspectorOpen: boolean
  onToggleInspector: () => void
  onInspectorWidthCommit: (width: number) => void
  onSidebarWidthCommit: (width: number) => void
}

export function AppShell({
  style,
  sidebarWidth,
  sidebarCollapsed,
  onToggleSidebar,
  toggleSidebarShortcut,
  mode,
  platform,
  header,
  sidebar,
  content,
  inspector,
  inspectorWidth,
  inspectorOpen,
  onToggleInspector,
  onInspectorWidthCommit,
  onSidebarWidthCommit
}: AppShellProps) {
  const sidebarAvailable = mode !== 'settings'
  const sidebarHidden = !sidebarAvailable || sidebarCollapsed
  const sidebarLabel = t(sidebarCollapsed ? 'hosts:showSidebar' : 'hosts:hideSidebar')
  const inspectorLabel = t(inspectorOpen ? 'hosts:hideHostDetails' : 'hosts:showHostDetails')
  const toggleWindowMaximise = (event: MouseEvent<HTMLElement>) => {
    const target = event.target as HTMLElement
    const isEmptyTabStrip = target === target.closest('.workspace-tabs')
    if (
      target === event.currentTarget ||
      isEmptyTabStrip ||
      target.closest('.workspace-window-drag-region')
    ) {
      WindowToggleMaximise()
    }
  }

  return (
    <main
      className={cn(
        'app-workspace [:root[data-theme=dark]_&]:bg-[var(--workspace-background)] w-screen h-screen min-w-275 min-h-170 overflow-hidden',
        'text-[var(--mac-text)] grid grid-rows-[40px_minmax(0,_1fr)] bg-[var(--workspace-background)]',
        `workspace-${mode}`,
        `platform-${platform}`
      )}
      style={style}
    >
      <header
        className={cn(
          'workspace-tab-bar flex items-center min-w-0 pt-0 pr-3 pb-0 pl-22 text-[var(--mac-text)] [-webkit-app-region:drag]',
          '[.app-workspace:not(.platform-darwin)_&]:pl-3'
        )}
        onDoubleClick={toggleWindowMaximise}
      >
        {sidebarAvailable && (
          <Tip title={`${sidebarLabel} (${formatCommandPaletteShortcut(toggleSidebarShortcut)})`}>
            <IconButton
              className="mr-2 h-7 shrink-0 rounded-[7px] border border-transparent text-[var(--mac-secondary-text)] hover:bg-[var(--workspace-tab-hover-background)] hover:border-[var(--workspace-tab-hover-border)] hover:text-[var(--mac-text)] aria-pressed:bg-[var(--workspace-tab-active-background)] aria-pressed:border-[var(--workspace-tab-active-border)] aria-pressed:text-[var(--workspace-tab-active-text)] aria-pressed:shadow-[var(--workspace-tab-active-shadow)] [-webkit-app-region:no-drag]"
              data-action="toggle-sidebar"
              aria-label={sidebarLabel}
              aria-expanded={!sidebarCollapsed}
              aria-pressed={!sidebarCollapsed}
              aria-controls="connection-sidebar"
              icon={<PanelLeft size={16} />}
              onClick={onToggleSidebar}
            />
          </Tip>
        )}
        {header}
        <Tip title={inspectorLabel}>
          <IconButton
            className="ml-2 h-7 shrink-0 rounded-[7px] border border-transparent text-[var(--mac-secondary-text)] hover:bg-[var(--workspace-tab-hover-background)] aria-pressed:bg-[var(--workspace-tab-active-background)] aria-pressed:border-[var(--workspace-tab-active-border)] [-webkit-app-region:no-drag]"
            aria-label={inspectorLabel}
            aria-expanded={inspectorOpen}
            aria-pressed={inspectorOpen}
            aria-controls="host-inspector-sidebar"
            icon={<PanelRight size={16} />}
            onClick={onToggleInspector}
          />
        </Tip>
      </header>
      <SidebarLayout
        preferredWidth={sidebarWidth}
        hidden={sidebarHidden}
        onWidthCommit={onSidebarWidthCommit}
        sidebar={sidebar}
      >
        <SidebarLayout
          side="right"
          preferredWidth={inspectorWidth}
          hidden={!inspectorOpen}
          onWidthCommit={onInspectorWidthCommit}
          sidebar={
            <aside id="host-inspector-sidebar" aria-label={t('hosts:hostDetails')} className="min-w-0 min-h-0">
              {inspectorOpen && inspector}
            </aside>
          }
        >
          {content}
        </SidebarLayout>
      </SidebarLayout>
    </main>
  )
}
