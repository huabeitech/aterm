import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SidebarUtilityIcon } from '../../components/sidebar/SidebarUtilityIcon'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Box } from 'lucide-react'
import type {
  JsonValue,
  PluginInfo,
  PluginView as View,
} from '../../../../shared/contracts/plugins'
import { desktop } from '../../api/app'
import { PluginView } from './PluginView'
import { usePluginCatalog } from './pluginCatalog'
import { Button } from '@/components/ui/button'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
export function pluginTitle(plugin: PluginInfo, key: string, locale: string): string {
  const bundle = plugin.localizations
  return bundle?.messages[locale]?.[key] ?? bundle?.messages[bundle.defaultLocale]?.[key] ?? key
}
export function PluginIcon({ view }: { view?: View }) {
  return view?.icon ? <SidebarUtilityIcon path={view.icon} /> : <Box size={16} aria-hidden="true" />
}
export type OpenNativePlugin = (pluginID: string, viewID: string, hostID?: string) => void
export function PluginActions({
  location,
  hostID,
  onOpen,
}: {
  location: string
  hostID?: string
  onOpen: OpenNativePlugin
}) {
  const catalog = usePluginCatalog()
  const { i18n } = useTranslation()
  return (
    <>
      {catalog
        .filter((p) => p.enabled && p.manifest.runtime === 'native')
        .flatMap((p) =>
          (p.manifest.contributes.views ?? [])
            .filter((v) => v.locations?.includes(location))
            .map((v) => {
              const label = pluginTitle(p, v.title, i18n.resolvedLanguage ?? i18n.language)
              const open = () => onOpen(p.manifest.id, v.id, hostID)
              return location === 'sidebar' ? (
                <Tooltip key={`${p.manifest.id}:${v.id}`}>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      className="sidebar-utility-button sidebar-utility-icon"
                      aria-label={label}
                      onClick={open}
                    >
                      <PluginIcon view={v} />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="top" sideOffset={8}>
                    {label}
                  </TooltipContent>
                </Tooltip>
              ) : location === 'host.details' ? (
                <DropdownMenuItem key={`${p.manifest.id}:${v.id}`} onSelect={open}>
                  <PluginIcon view={v} />
                  {label}
                </DropdownMenuItem>
              ) : (
                <Button key={`${p.manifest.id}:${v.id}`} variant="ghost" size="sm" onClick={open}>
                  <PluginIcon view={v} />
                  {label}
                </Button>
              )
            }),
        )}
    </>
  )
}
export function NativeWorkspace({
  plugin,
  view,
  hostID,
  active = true,
  unlock,
  onClose,
}: {
  plugin: PluginInfo
  view: View
  hostID?: string
  active?: boolean
  unlock: () => Promise<boolean>
  onClose: () => void
}) {
  const [panel, setPanel] = useState<HTMLDivElement | null>(null)
  const consumer = useId()
  const sessionID = hostID ? `host:${hostID}` : 'global'
  const [error, setError] = useState('')
  useEffect(() => {
    if (!active) return
    void desktop.plugins
      .visibility(plugin.manifest.id, sessionID, consumer, true)
      .catch((e) => setError(String(e)))
    return () => {
      void desktop.plugins
        .visibility(plugin.manifest.id, sessionID, consumer, false)
        .catch(() => {})
    }
  }, [
    plugin.manifest.id,
    plugin.manifest.version,
    plugin.state === 'failed',
    sessionID,
    consumer,
    active,
  ])
  async function execute(command: string, args?: JsonValue) {
    const declaration = plugin.manifest.contributes.commands?.find((c) => c.id === command)
    if (declaration?.requiresUnlock && !(await unlock())) return null
    try {
      const result = await desktop.plugins.executeCommand(
        plugin.manifest.id,
        sessionID,
        command,
        args,
      )
      return result.data
    } catch (error) {
      // The plugin owns its error summary; retain raw transport diagnostics.
      if (
        error &&
        typeof error === 'object' &&
        'detail' in error &&
        typeof error.detail === 'string'
      )
        throw new Error(error.detail)
      throw error
    }
  }
  return (
    <div ref={setPanel} className="plugin-workspace h-full min-h-0">
      {error ? (
        <p role="alert">{error}</p>
      ) : (
        panel && (
          <PluginView
            workspace
            active={active}
            plugin={plugin}
            session={{ id: sessionID, alias: '', title: '' }}
            view={{ id: view.id, invocationId: sessionID, context: { hostID: hostID ?? '' } }}
            panel={panel}
            onClose={onClose}
            onCommand={execute}
          />
        )
      )}
    </div>
  )
}
