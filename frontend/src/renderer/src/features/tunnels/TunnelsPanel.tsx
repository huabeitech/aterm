import { useTunnelOperations } from './useTunnelOperations'
import { useState } from 'react'
import { Copy, Pencil, Play, Square, Trash2 } from 'lucide-react'
import type { domain } from '../../../../shared/contracts/domain'
import type { TunnelInput, TunnelStatus } from '../../../../shared/contracts/tunnels'
import { desktop } from '../../api/app'
import { Button } from '@/components/ui/button'
import { useAppUI } from '@/components/ui/confirm-provider'
import { TunnelEditor } from './TunnelEditor'
import { tunnelAddress, tunnelNeedsRestart } from './tunnelModel'
import type { useTunnels } from './useTunnels'
export type TunnelsPanelProps = ReturnType<typeof useTunnels> & {
  hosts: domain.Host[]
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
  refresh
}: TunnelsPanelProps) {
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
        <h2 className="text-sm font-medium">{hostID ? 'Tunnels' : 'All tunnels'}</h2>
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
                targetPort: 3306
              }
            })
          }
        >
          New tunnel
        </Button>
      </header>
      {(failure || error) && (
        <p role="alert" className="px-4 py-3 text-sm text-destructive">
          {failure || error}
        </p>
      )}
      <div className="min-h-0 overflow-auto">
        <table className="w-full text-left text-xs">
          <thead className="bg-muted/40 text-muted-foreground">
            <tr>
              {['Name', 'Local address', 'Service', 'Status', ''].map((label, index) => (
                <th key={index} className="px-3 py-2 font-normal">
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visible.map((row) => {
              const active = row.activeConfig ?? row
              return (
                <tr key={row.id} className="border-b border-border">
                  <td className="px-3 py-3">
                    {row.name}
                    {!hostID && (
                      <div className="text-muted-foreground">
                        {hosts.find((host) => host.id === row.hostID)?.alias}
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-3">
                    {tunnelAddress(active.localAddress, active.localPort)}
                  </td>
                  <td className="px-3 py-3">
                    {active.targetAddress === '127.0.0.1'
                      ? `This host:${active.targetPort}`
                      : tunnelAddress(active.targetAddress, active.targetPort)}
                  </td>
                  <td className="px-3 py-3">
                    <span className="capitalize">{row.state}</span>
                    {tunnelNeedsRestart(row) && <div>Restart required</div>}
                    {(row.error || row.lastError) && (
                      <p className="max-w-64 break-words text-destructive">
                        {row.error || row.lastError}
                      </p>
                    )}
                  </td>
                  <td className="px-3 py-3">
                    <div className="flex gap-1">
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        disabled={pending.includes(row.id) || row.state === 'stopping'}
                        aria-label={`${row.state === 'running' || row.state === 'starting' ? 'Stop' : 'Start'} ${row.name}`}
                        onClick={() => toggle(row)}
                      >
                        {row.state === 'running' || row.state === 'starting' ? (
                          <Square size={14} />
                        ) : (
                          <Play size={14} />
                        )}
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={`Edit ${row.name}`}
                        onClick={() => setEditor({ id: row.id, input: row })}
                      >
                        <Pencil size={14} />
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        disabled={row.state !== 'running'}
                        aria-label={`Copy address for ${row.name}`}
                        onClick={() =>
                          void desktop.library
                            .copyText(tunnelAddress(active.localAddress, active.localPort))
                            .catch(() => message.error('Unable to copy address.'))
                        }
                      >
                        <Copy size={14} />
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={`Delete ${row.name}`}
                        onClick={() =>
                          modal.confirm({
                            title: `Delete “${row.name}”?`,
                            content: 'This stops the tunnel and removes its configuration.',
                            okText: 'Delete',
                            cancelText: 'Cancel',
                            okButtonProps: { danger: true },
                            onOk: () => perform(row.id, () => desktop.tunnels.delete(row.id))
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
            {loading ? 'Loading…' : 'No tunnels'}
          </p>
        )}
      </div>
      {editor && (
        <TunnelEditor
          input={editor.input}
          hosts={hosts}
          editing={!!editor.id}
          onClose={() => setEditor(null)}
          onSave={async (input, start) => {
            if (start && !(await unlock())) return
            await perform(editor.id || 'new', async () => {
              const saved = await desktop.tunnels.save(editor.id, input)
              if (start) {
                await desktop.tunnels.stop(saved.id)
                await desktop.tunnels.start(saved.id)
              }
            })
            setEditor(null)
          }}
        />
      )}
    </section>
  )
}
