import { useRef, useState } from 'react'
import type { domain } from '../../../../shared/contracts/domain'
import type { TunnelInput } from '../../../../shared/contracts/tunnels'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, SearchSelect } from '@/components/ui/desktop'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter
} from '@/components/ui/dialog'
import { validateTunnel } from './tunnelModel'

type Props = {
  input: TunnelInput
  hosts: domain.Host[]
  editing: boolean
  onClose: () => void
  onSave: (input: TunnelInput, start: boolean) => Promise<void>
}
const presets = {
  MySQL: [3306, 13306],
  Redis: [6379, 16379],
  'Web service': [8080, 18080],
  'Other TCP': [9000, 19000]
} as const
export function TunnelEditor({ input, hosts, editing, onClose, onSave }: Props) {
  const [draft, setDraft] = useState(input)
  const [privateHost, setPrivateHost] = useState(input.targetAddress !== '127.0.0.1')
  const [busy, setBusy] = useState(false)
  const saving = useRef(false)
  const [error, setError] = useState('')
  function update<K extends keyof TunnelInput>(key: K, value: TunnelInput[K]) {
    setDraft((current) => ({ ...current, [key]: value }))
    setError('')
  }
  async function save(start: boolean) {
    if (saving.current) return
    const value = { ...draft, targetAddress: privateHost ? draft.targetAddress : '127.0.0.1' }
    const invalid = validateTunnel(value)
    if (invalid) {
      setError(invalid)
      return
    }
    saving.current = true
    setBusy(true)
    try {
      await onSave(value, start)
    } catch (cause) {
      setError(String(cause))
    } finally {
      saving.current = false
      setBusy(false)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent
        aria-describedby={undefined}
        className="sm:max-w-lg"
        showCloseButton={!busy}
        onPointerDownOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>{editing ? 'Edit tunnel' : 'New tunnel'}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            void save(false)
          }}
          className="space-y-4"
        >
          <fieldset disabled={busy} className="host-editor grid gap-4 p-1">
            <Field label="Name" required>
              <Input
                autoFocus
                aria-label="Name"
                required
                value={draft.name}
                onChange={(event) => update('name', event.target.value)}
                spellCheck={false}
                autoComplete="off"
              />
            </Field>
            {!editing && (
              <Field label="Service">
                <TunnelSelect
                  label="Service"
                  value={
                    Object.keys(presets).find(
                      (key) => presets[key as keyof typeof presets][0] === draft.targetPort
                    ) ?? 'Other TCP'
                  }
                  disabled={busy}
                  options={Object.keys(presets).map((value) => ({ value, label: value }))}
                  onChange={(value) => {
                    const [targetPort, localPort] = presets[value as keyof typeof presets]
                    setDraft((current) => ({ ...current, targetPort, localPort }))
                  }}
                />
              </Field>
            )}
            <Field label="SSH host" required>
              <SearchSelect
                label="SSH host"
                placeholder="Select an SSH host"
                value={draft.hostID || undefined}
                onChange={(value) => update('hostID', value)}
                options={hosts.map((host) => ({
                  value: host.id,
                  label: `${host.alias} — ${host.user ? `${host.user}@` : ''}${host.hostName}`
                }))}
              />
            </Field>
            <Field label="Service location">
              <TunnelSelect
                label="Service location"
                disabled={busy}
                value={privateHost ? 'private' : 'same'}
                options={[
                  { value: 'same', label: 'This SSH host' },
                  { value: 'private', label: 'Another private host' }
                ]}
                onChange={(value) => {
                  setPrivateHost(value === 'private')
                  setError('')
                }}
              />
            </Field>
            {privateHost && (
              <Field label="Service address" required>
                <Input
                  aria-label="Service address"
                  required
                  value={draft.targetAddress}
                  onChange={(event) => update('targetAddress', event.target.value)}
                  spellCheck={false}
                  autoComplete="off"
                />
              </Field>
            )}
            <div className="grid grid-cols-2 gap-4">
              <Field label="Service port" required>
                <Input
                  aria-label="Service port"
                  type="number"
                  min={1}
                  max={65535}
                  required
                  value={draft.targetPort || ''}
                  onChange={(event) => update('targetPort', Number(event.target.value))}
                />
              </Field>
              <Field label="Local port" required>
                <Input
                  aria-label="Local port"
                  type="number"
                  min={1}
                  max={65535}
                  required
                  value={draft.localPort || ''}
                  onChange={(event) => update('localPort', Number(event.target.value))}
                />
              </Field>
            </div>
            <details>
              <summary className="text-xs text-muted-foreground">Advanced</summary>
              <Field label="Listen address" className="mt-3">
                <Input
                  aria-label="Listen address"
                  value={draft.localAddress}
                  onChange={(event) => update('localAddress', event.target.value)}
                  spellCheck={false}
                />
              </Field>
            </details>
          </fieldset>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={busy} onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" variant="outline" disabled={busy}>
              Save
            </Button>
            <Button type="button" disabled={busy} onClick={() => void save(true)}>
              {editing ? 'Save and restart' : 'Save and start'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function TunnelSelect({
  label,
  value,
  options,
  disabled,
  onChange
}: {
  label: string
  value: string
  options: { value: string; label: string }[]
  disabled: boolean
  onChange: (value: string) => void
}) {
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger aria-label={label} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
