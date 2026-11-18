import { tunnelErrorMessage } from './tunnelErrors'
import { t } from './i18n'
import { useTranslation } from 'react-i18next'
import { useTunnelOperations } from './useTunnelOperations'
import { useState } from 'react'
import { Copy, Pencil, CirclePlay, CircleStop, Trash2 } from 'lucide-react'
import type { Host } from './types'
import type { TunnelInput, TunnelStatus } from './types'
import { desktop } from './api'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useAppUI } from '@/components/ui/confirm-provider'
import { TunnelEditor } from './TunnelEditor'
import { TunnelStatusBadge } from './TunnelStatusBadge'
import { tunnelAddress, tunnelNeedsRestart } from './tunnelModel'
import type { useTunnels } from './useTunnels'
export type TunnelsPanelProps = ReturnType<typeof useTunnels> & {
  hosts: Host[]
  hostID?: string
  unlock: () => Promise<boolean>
}
export function TunnelsPanel({
  hosts,
  hostID,
  unlock,
  tunnels,
  loading,
  error,
  refresh,
}: TunnelsPanelProps) {
  useTranslation()
  const { modal, message } = useAppUI()
  const [editor, setEditor] = useState<{ id: string; input: TunnelInput } | null>(null)
  const { perform, pending, failure } = useTunnelOperations(refresh)
  function toggle(row: TunnelStatus) {
    void perform(row.id, async () => {
      if (row.state === 'running' || row.state === 'starting') await desktop.tunnels.stop(row.id)
      else if (await unlock()) await desktop.tunnels.start(row.id)
    }).catch(() => {})
  }
  const visible = hostID ? tunnels.filter((row) => row.hostID === hostID) : tunnels
  return (
    <section className="flex h-full min-h-0 flex-col">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-medium">
          {hostID ? t('hosts:tunnels.title') : t('hosts:tunnels.all')}
        </h2>
        <Button
          size="sm"
          variant="outline"
          disabled={!hosts.length}
          onClick={() =>
            setEditor({
              id: '',
              input: {
                name: '',
                hostID: hostID || hosts[0]?.id || '',
                localAddress: '127.0.0.1',
                localPort: 13306,
                targetAddress: '127.0.0.1',
                targetPort: 3306,
              },
            })
          }
        >
          {t('hosts:tunnels.new')}
        </Button>
      </header>
      {(failure || error) && (
        <p role="alert" className="px-4 py-3 text-sm text-destructive">
          {tunnelErrorMessage(failure || error)}
        </p>
      )}
      <div className="min-h-0 overflow-auto [scrollbar-gutter:stable]">
        <table className="w-full min-w-[640px] table-fixed text-left text-xs">
          <colgroup>
            <col />
            <col />
            <col />
            <col />
            <col className="w-40" />
          </colgroup>
          <thead className="bg-muted/40 text-muted-foreground">
            <tr>
              {[
                t('hosts:tunnels.name'),
                t('hosts:tunnels.localAddress'),
                t('hosts:tunnels.service'),
                t('hosts:tunnels.status'),
                '',
              ].map((label, index) => (
                <th key={index} className="px-3 py-2 font-normal">
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visible.map((row) => {
              const active = row.activeConfig ?? row
              const canStop = row.state === 'running' || row.state === 'starting'
              const toggleLabel = t(
                canStop ? 'hosts:tunnels.stopAction' : 'hosts:tunnels.startAction',
                { name: row.name },
              )
              return (
                <tr key={row.id} className="border-b border-border">
                  <td className="break-words px-3 py-3">
                    {row.name}
                    {!hostID && (
                      <div className="text-muted-foreground">
                        {hosts.find((host) => host.id === row.hostID)?.alias}
                      </div>
                    )}
                  </td>
                  <td className="break-words px-3 py-3">
                    {tunnelAddress(active.localAddress, active.localPort)}
                  </td>
                  <td className="break-words px-3 py-3">
                    {active.targetAddress === '127.0.0.1'
                      ? `${t('hosts:tunnels.same')}:${active.targetPort}`
                      : tunnelAddress(active.targetAddress, active.targetPort)}
                  </td>
                  <td className="break-words px-3 py-3">
                    <TunnelStatusBadge state={row.state} />
                    {tunnelNeedsRestart(row) && <div>{t('hosts:tunnels.restartRequired')}</div>}
                    {(row.error || row.lastError) && (
                      <p className="max-w-64 break-words text-destructive">
                        {tunnelErrorMessage(row.error || row.lastError)}
                      </p>
                    )}
                  </td>
                  <td className="break-words px-3 py-3">
                    <div className="flex gap-1">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            className={
                              canStop
                                ? 'text-red-600 hover:bg-red-500/10 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300'
                                : 'text-green-700 hover:bg-green-500/10 hover:text-green-800 dark:text-green-400 dark:hover:text-green-300'
                            }
                            disabled={pending.includes(row.id) || row.state === 'stopping'}
                            aria-label={toggleLabel}
                            onClick={() => toggle(row)}
                          >
                            {canStop ? (
                              <CircleStop aria-hidden="true" />
                            ) : (
                              <CirclePlay aria-hidden="true" />
                            )}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{toggleLabel}</TooltipContent>
                      </Tooltip>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={t('hosts:tunnels.editAction', { name: row.name })}
                        onClick={() => setEditor({ id: row.id, input: row })}
                      >
                        <Pencil size={14} />
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        disabled={row.state !== 'running'}
                        aria-label={t('hosts:tunnels.copy', { name: row.name })}
                        onClick={() =>
                          void desktop.library
                            .copyText(tunnelAddress(active.localAddress, active.localPort))
                            .catch(() => message.error(t('hosts:tunnels.copyFailed')))
                        }
                      >
                        <Copy size={14} />
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={t('hosts:tunnels.deleteAction', { name: row.name })}
                        onClick={() =>
                          modal.confirm({
                            title: t('hosts:tunnels.deleteTitle', { name: row.name }),
                            content: t('hosts:tunnels.deleteHelp'),
                            okText: t('hosts:tunnels.delete'),
                            cancelText: t('hosts:tunnels.cancel'),
                            okButtonProps: { danger: true },
                            onOk: () => perform(row.id, () => desktop.tunnels.delete(row.id)),
                          })
                        }
                      >
                        <Trash2 size={14} />
                      </Button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {!visible.length && (
          <p className="p-8 text-center text-sm text-muted-foreground">
            {loading ? t('hosts:tunnels.loading') : t('hosts:tunnels.empty')}
          </p>
        )}
      </div>
      {editor && (
        <TunnelEditor
          input={editor.input}
          onTest={async (input) => {
            if (!(await unlock())) return null
            return desktop.tunnels.test(editor.id, input)
          }}
          hosts={hosts}
          editing={!!editor.id}
          onClose={() => setEditor(null)}
          onSave={async (input) => {
            await perform(editor.id || 'new', () => desktop.tunnels.save(editor.id, input))
            setEditor(null)
          }}
        />
      )}
    </section>
  )
}
