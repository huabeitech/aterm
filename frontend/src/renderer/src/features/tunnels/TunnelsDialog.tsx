import { useId, useRef, useState, type FormEvent } from 'react'
import { Cable, Copy, Pencil, Play, Plus, RefreshCw, Square, Trash2 } from 'lucide-react'
import type { domain } from '../../../../shared/contracts/domain'
import type { TunnelInput, TunnelStatus } from '../../../../shared/contracts/tunnels'
import { desktop } from '../../api/app'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter
} from '@/components/ui/dialog'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem
} from '@/components/ui/select'
import { useAppUI } from '@/components/ui/confirm-provider'
import {
  localAddressIsLoopback,
  tunnelAddress,
  tunnelNeedsRestart,
  validateTunnel
} from './tunnelModel'
import type { useTunnels } from './useTunnels'

type Props = ReturnType<typeof useTunnels> & {
  hosts: domain.Host[]
  initialHostID: string
  onClose: () => void
  unlock: () => Promise<boolean>
}
const stateLabels: Record<TunnelStatus['state'], string> = {
  stopped: 'Stopped',
  starting: 'Starting…',
  running: 'Running',
  stopping: 'Stopping…',
  failed: 'Failed'
}

export function TunnelsDialog({
  hosts,
  initialHostID,
  onClose,
  unlock,
  tunnels,
  loading,
  error,
  refresh
}: Props) {
  const { modal, message } = useAppUI()
  const [hostID, setHostID] = useState(initialHostID || 'all')
  const [editor, setEditor] = useState<{
    id: string
    input: TunnelInput
  } | null>(null)
  const [busy, setBusy] = useState<string[]>([])
  const [actionError, setActionError] = useState('')
  const pending = useRef(new Set<string>())
  const visible = hostID === 'all' ? tunnels : tunnels.filter((tunnel) => tunnel.hostID === hostID)
  async function perform(id: string, action: () => Promise<unknown>) {
    if (pending.current.has(id)) return
    pending.current.add(id)
    setBusy([...pending.current])
    setActionError('')
    try {
      await action()
      await refresh()
    } catch (cause) {
      setActionError(cause instanceof Error ? cause.message : String(cause))
      throw cause
    } finally {
      pending.current.delete(id)
      setBusy([...pending.current])
    }
  }
  function create() {
    setEditor({
      id: '',
      input: {
        name: '',
        hostID: hostID === 'all' ? (hosts[0]?.id ?? '') : hostID,
        localAddress: '127.0.0.1',
        localPort: 13306,
        targetAddress: '',
        targetPort: 3306
      }
    })
  }
  function remove(tunnel: TunnelStatus) {
    modal.confirm({
      title: `Delete “${tunnel.name}”?`,
      content:
        'This removes the saved configuration and stops any active connections through this tunnel.',
      okText: 'Delete tunnel',
      cancelText: 'Cancel',
      okButtonProps: { danger: true },
      onOk: () => perform(tunnel.id, () => desktop.tunnels.delete(tunnel.id))
    })
  }
  async function start(tunnel: TunnelStatus, restart = false) {
    await perform(tunnel.id, async () => {
      if (!(await unlock())) return
      if (restart) await desktop.tunnels.stop(tunnel.id)
      await desktop.tunnels.start(tunnel.id)
    })
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="flex max-h-[85vh] flex-col gap-4 sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>SSH tunnels</DialogTitle>
          <DialogDescription>
            Access private services through your saved SSH hosts. Tunnels run independently of
            terminal tabs and stop when aTerm quits.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          <Select value={hostID} onValueChange={setHostID}>
            <SelectTrigger aria-label="Filter tunnels by SSH host" className="min-w-48 max-w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All SSH hosts</SelectItem>
              {hosts.map((host) => (
                <SelectItem key={host.id} value={host.id}>
                  {host.alias}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button variant="outline" size="sm" onClick={() => void refresh()}>
            <RefreshCw size={14} />
            Refresh
          </Button>
          <Button size="sm" disabled={!hosts.length} onClick={create}>
            <Plus size={14} />
            New tunnel
          </Button>
        </div>
        {(error || actionError) && (
          <p role="alert" className="text-sm text-destructive break-words">
            {actionError || error}
          </p>
        )}
        <div className="min-h-0 overflow-y-auto space-y-3" aria-busy={loading}>
          {loading ? (
            <p role="status" className="p-8 text-sm text-muted-foreground">
              Loading tunnels…
            </p>
          ) : visible.length === 0 ? (
            <div className="flex flex-col items-center gap-3 rounded-lg border border-border p-8 text-center">
              <Cable size={24} className="text-muted-foreground" />
              <p className="text-sm font-medium">No tunnels saved</p>
              <p className="text-sm text-muted-foreground">
                {hosts.length
                  ? 'Create a tunnel to connect a local port to a private database, dashboard, or other TCP service.'
                  : 'Add an SSH host before creating a tunnel.'}
              </p>
              {hosts.length > 0 && (
                <Button variant="outline" size="sm" onClick={create}>
                  Create tunnel
                </Button>
              )}
            </div>
          ) : (
            visible.map((tunnel) => {
              const active = tunnel.activeConfig ?? tunnel
              const inProgress = tunnel.state === 'starting' || tunnel.state === 'stopping'
              const isActive = inProgress || tunnel.state === 'running'
              const disabled = busy.includes(tunnel.id)
              const hostName =
                hosts.find((host) => host.id === active.hostID)?.alias ?? 'Missing SSH host'
              const local = tunnelAddress(active.localAddress, active.localPort)
              return (
                <section
                  key={tunnel.id}
                  aria-label={tunnel.name}
                  className="rounded-lg border border-border p-4 space-y-3"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex min-w-0 items-center gap-2">
                      <h3 className="text-sm font-medium break-words">{tunnel.name}</h3>
                      <Badge variant={tunnel.state === 'failed' ? 'destructive' : 'secondary'}>
                        {stateLabels[tunnel.state]}
                      </Badge>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      {isActive ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={disabled || tunnel.state === 'stopping'}
                          onClick={() =>
                            void perform(tunnel.id, () => desktop.tunnels.stop(tunnel.id)).catch(
                              () => {}
                            )
                          }
                        >
                          <Square size={14} />
                          Stop
                        </Button>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={disabled}
                          onClick={() => void start(tunnel).catch(() => {})}
                        >
                          <Play size={14} />
                          {tunnel.state === 'failed' ? 'Retry' : 'Start'}
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="icon-sm"
                        aria-label={`Edit ${tunnel.name}`}
                        disabled={disabled || inProgress}
                        onClick={() =>
                          setEditor({
                            id: tunnel.id,
                            input: {
                              name: tunnel.name,
                              hostID: tunnel.hostID,
                              localAddress: tunnel.localAddress,
                              localPort: tunnel.localPort,
                              targetAddress: tunnel.targetAddress,
                              targetPort: tunnel.targetPort
                            }
                          })
                        }
                      >
                        <Pencil size={14} />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon-sm"
                        aria-label={`Delete ${tunnel.name}`}
                        disabled={disabled}
                        onClick={() => remove(tunnel)}
                      >
                        <Trash2 size={14} />
                      </Button>
                    </div>
                  </div>
                  <dl className="grid gap-2 text-xs sm:grid-cols-3">
                    <div>
                      <dt className="text-muted-foreground">Local address</dt>
                      <dd className="mt-1 flex items-center gap-2">
                        <code className="break-all">{local}</code>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Copy local address for ${tunnel.name}`}
                          onClick={() =>
                            void desktop.library
                              .copyText(local)
                              .then(() => message.success('Local address copied.'))
                              .catch(() => message.error('Unable to copy the local address.'))
                          }
                        >
                          <Copy size={14} />
                        </Button>
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">SSH host</dt>
                      <dd className="mt-1 break-words">{hostName}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Target from SSH host</dt>
                      <dd className="mt-1">
                        <code className="break-all">
                          {tunnelAddress(active.targetAddress, active.targetPort)}
                        </code>
                      </dd>
                    </div>
                  </dl>
                  {tunnel.state === 'running' && !tunnel.lastError && (
                    <p className="text-xs text-muted-foreground">
                      Listening locally. The target service is contacted when an application
                      connects.
                    </p>
                  )}
                  {tunnel.error && (
                    <p role="alert" className="text-sm text-destructive break-words">
                      {tunnel.error}
                    </p>
                  )}
                  {tunnel.lastError && (
                    <p role="status" className="text-sm text-destructive break-words">
                      Last target connection failed: {tunnel.lastError}. The tunnel is still
                      listening.
                    </p>
                  )}
                  {tunnelNeedsRestart(tunnel) && (
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>
                        Saved settings have changed. Restart to apply them; existing connections
                        will close.
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={disabled || inProgress}
                        onClick={() => void start(tunnel, true).catch(() => {})}
                      >
                        Restart
                      </Button>
                    </div>
                  )}
                </section>
              )
            })
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Done
          </Button>
        </DialogFooter>
        {editor && (
          <TunnelEditor
            key={editor.id || 'new'}
            value={editor}
            hosts={hosts}
            onClose={() => setEditor(null)}
            onSave={async (input) => {
              await perform(editor.id || 'new', () => desktop.tunnels.save(editor.id, input))
              setEditor(null)
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function TunnelEditor({
  value,
  hosts,
  onClose,
  onSave
}: {
  value: { id: string; input: TunnelInput }
  hosts: domain.Host[]
  onClose: () => void
  onSave: (input: TunnelInput) => Promise<void>
}) {
  const [input, setInput] = useState(value.input)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const savingRef = useRef(false)
  const prefix = useId()
  function update<K extends keyof TunnelInput>(key: K, next: TunnelInput[K]) {
    setInput((previous) => ({ ...previous, [key]: next }))
    setError('')
  }
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (savingRef.current) return
    const next = {
      ...input,
      name: input.name.trim(),
      localAddress: input.localAddress.trim(),
      targetAddress: input.targetAddress.trim()
    }
    const invalid = validateTunnel(next)
    if (invalid) {
      setError(invalid)
      return
    }
    savingRef.current = true
    setSaving(true)
    try {
      await onSave(next)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving) onClose()
      }}
    >
      <DialogContent
        className="sm:max-w-lg"
        aria-describedby={`${prefix}-help`}
        showCloseButton={!saving}
        onPointerDownOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (saving) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>{value.id ? 'Edit tunnel' : 'New tunnel'}</DialogTitle>
          <DialogDescription id={`${prefix}-help`}>
            Forward a local TCP port through an SSH host. Target addresses are reached from that
            host.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={(event) => void submit(event)} className="space-y-4">
          <fieldset disabled={saving} className="space-y-4">
            <label className="grid gap-2 text-sm" htmlFor={`${prefix}-name`}>
              Name
              <Input
                id={`${prefix}-name`}
                autoFocus
                required
                value={input.name}
                onChange={(event) => update('name', event.target.value)}
                placeholder="Production database"
              />
            </label>
            <div className="grid gap-2 text-sm">
              <label id={`${prefix}-host-label`} htmlFor={`${prefix}-host`}>
                SSH host
              </label>
              <Select
                value={input.hostID}
                onValueChange={(value) => update('hostID', value)}
                disabled={saving}
              >
                <SelectTrigger
                  id={`${prefix}-host`}
                  aria-labelledby={`${prefix}-host-label`}
                  className="w-full"
                >
                  <SelectValue placeholder="Select an SSH host" />
                </SelectTrigger>
                <SelectContent>
                  {hosts.map((host) => (
                    <SelectItem key={host.id} value={host.id}>
                      {host.alias}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-[minmax(0,1fr)_8rem] gap-3">
              <label className="grid gap-2 text-sm" htmlFor={`${prefix}-local`}>
                Local listen address
                <Input
                  id={`${prefix}-local`}
                  required
                  value={input.localAddress}
                  onChange={(event) => update('localAddress', event.target.value)}
                  placeholder="127.0.0.1"
                  spellCheck={false}
                />
              </label>
              <label className="grid gap-2 text-sm" htmlFor={`${prefix}-local-port`}>
                Local port
                <Input
                  id={`${prefix}-local-port`}
                  required
                  type="number"
                  min={1}
                  max={65535}
                  step={1}
                  value={input.localPort || ''}
                  onChange={(event) => update('localPort', Number(event.target.value))}
                />
              </label>
            </div>
            {!localAddressIsLoopback(input.localAddress) && (
              <p role="status" className="text-xs text-muted-foreground">
                This address may allow other devices to access the forwarded service. Use 127.0.0.1
                or ::1 to allow access only from this computer.
              </p>
            )}
            <div className="grid grid-cols-[minmax(0,1fr)_8rem] gap-3">
              <label className="grid gap-2 text-sm" htmlFor={`${prefix}-target`}>
                Target address
                <Input
                  id={`${prefix}-target`}
                  required
                  value={input.targetAddress}
                  onChange={(event) => update('targetAddress', event.target.value)}
                  placeholder="db.internal"
                  spellCheck={false}
                />
              </label>
              <label className="grid gap-2 text-sm" htmlFor={`${prefix}-target-port`}>
                Target port
                <Input
                  id={`${prefix}-target-port`}
                  required
                  type="number"
                  min={1}
                  max={65535}
                  step={1}
                  value={input.targetPort || ''}
                  onChange={(event) => update('targetPort', Number(event.target.value))}
                />
              </label>
            </div>
            <p className="text-xs text-muted-foreground">
              A target of 127.0.0.1 refers to the SSH host itself. SSH authentication and jump hosts
              come from the saved host configuration.
            </p>
            {value.id && (
              <p className="text-xs text-muted-foreground">
                Changes to a running tunnel take effect after you restart it.
              </p>
            )}
          </fieldset>
          {error && (
            <p role="alert" className="text-sm text-destructive break-words">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={saving} onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? 'Saving…' : 'Save tunnel'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
