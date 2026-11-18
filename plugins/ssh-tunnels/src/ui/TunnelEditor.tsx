import { tunnelErrorMessage } from './tunnelErrors'
import { t } from './i18n'
import { useTranslation } from 'react-i18next'
import { useRef, useState } from 'react'
import { CircleCheck, CircleAlert } from 'lucide-react'
import type { Host } from './types'
import type { TunnelInput } from './types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, SearchSelect } from '@/components/ui/desktop'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { validateTunnel } from './tunnelModel'

type Props = {
  input: TunnelInput
  hosts: Host[]
  editing: boolean
  onClose: () => void
  onTest: (input: TunnelInput) => Promise<{ localListener: 'checked' | 'existing' } | null>
  onSave: (input: TunnelInput) => Promise<void>
}
const presets = {
  MySQL: [3306, 13306],
  Redis: [6379, 16379],
  'Web service': [8080, 18080],
  'Other TCP': [9000, 19000],
} as const
export function TunnelEditor({ input, hosts, editing, onClose, onSave, onTest }: Props) {
  useTranslation()
  const [draft, setDraft] = useState(input)
  const [privateHost, setPrivateHost] = useState(input.targetAddress !== '127.0.0.1')
  const [busy, setBusy] = useState(false)
  const saving = useRef(false)
  const [error, setError] = useState('')
  const [testMessage, setTestMessage] = useState('')
  const [testing, setTesting] = useState(false)
  function update<K extends keyof TunnelInput>(key: K, value: TunnelInput[K]) {
    setDraft((current) => ({ ...current, [key]: value }))
    setError('')
    setTestMessage('')
  }
  async function save() {
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
      await onSave(value)
    } catch (cause) {
      setError(String(cause))
    } finally {
      saving.current = false
      setBusy(false)
    }
  }
  async function testConnection() {
    if (saving.current) return
    const value = { ...draft, targetAddress: privateHost ? draft.targetAddress : '127.0.0.1' }
    const invalid = validateTunnel(value)
    if (invalid) {
      setError(invalid)
      return
    }
    saving.current = true
    setBusy(true)
    setTesting(true)
    setError('')
    setTestMessage('')
    try {
      const result = await onTest(value)
      if (result)
        setTestMessage(result.localListener === 'existing' ? 'existingSuccess' : 'success')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      saving.current = false
      setBusy(false)
      setTesting(false)
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
          <DialogTitle>{editing ? t('hosts:tunnels.edit') : t('hosts:tunnels.new')}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
          className="space-y-4"
        >
          <fieldset disabled={busy} className="host-editor grid gap-4 p-1">
            <Field label={t('hosts:tunnels.name')} required>
              <Input
                autoFocus
                aria-label={t('hosts:tunnels.name')}
                required
                value={draft.name}
                onChange={(event) => update('name', event.target.value)}
                spellCheck={false}
                autoComplete="off"
              />
            </Field>
            {!editing && (
              <Field label={t('hosts:tunnels.service')}>
                <TunnelSelect
                  label={t('hosts:tunnels.service')}
                  value={
                    Object.keys(presets).find(
                      (key) => presets[key as keyof typeof presets][0] === draft.targetPort,
                    ) ?? 'Other TCP'
                  }
                  disabled={busy}
                  options={Object.keys(presets).map((value) => ({
                    value,
                    label:
                      value === 'Web service'
                        ? t('hosts:tunnels.web')
                        : value === 'Other TCP'
                          ? t('hosts:tunnels.custom')
                          : value,
                  }))}
                  onChange={(value) => {
                    const [targetPort, localPort] = presets[value as keyof typeof presets]
                    setTestMessage('')
                    setDraft((current) => ({ ...current, targetPort, localPort }))
                  }}
                />
              </Field>
            )}
            <Field label={t('hosts:tunnels.sshHost')} required>
              <SearchSelect
                label={t('hosts:tunnels.sshHost')}
                placeholder={t('hosts:tunnels.selectHost')}
                value={draft.hostID || undefined}
                onChange={(value) => update('hostID', value)}
                options={hosts.map((host) => ({
                  value: host.id,
                  label: `${host.alias} — ${host.user ? `${host.user}@` : ''}${host.hostName}`,
                }))}
              />
            </Field>
            <Field label={t('hosts:tunnels.location')}>
              <TunnelSelect
                label={t('hosts:tunnels.location')}
                disabled={busy}
                value={privateHost ? 'private' : 'same'}
                options={[
                  { value: 'same', label: t('hosts:tunnels.same') },
                  { value: 'private', label: t('hosts:tunnels.private') },
                ]}
                onChange={(value) => {
                  setTestMessage('')
                  setPrivateHost(value === 'private')
                  setError('')
                }}
              />
            </Field>
            {privateHost && (
              <Field label={t('hosts:tunnels.address')} required>
                <Input
                  aria-label={t('hosts:tunnels.address')}
                  required
                  value={draft.targetAddress}
                  onChange={(event) => update('targetAddress', event.target.value)}
                  spellCheck={false}
                  autoComplete="off"
                />
              </Field>
            )}
            <div className="grid grid-cols-2 gap-4">
              <Field label={t('hosts:tunnels.servicePort')} required>
                <Input
                  aria-label={t('hosts:tunnels.servicePort')}
                  type="number"
                  min={1}
                  max={65535}
                  required
                  value={draft.targetPort || ''}
                  onChange={(event) => update('targetPort', Number(event.target.value))}
                />
              </Field>
              <Field label={t('hosts:tunnels.localPort')} required>
                <Input
                  aria-label={t('hosts:tunnels.localPort')}
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
              <summary className="text-xs text-muted-foreground">
                {t('hosts:tunnels.advanced')}
              </summary>
              <Field label={t('hosts:tunnels.listen')} className="mt-3">
                <Input
                  aria-label={t('hosts:tunnels.listen')}
                  value={draft.localAddress}
                  onChange={(event) => update('localAddress', event.target.value)}
                  spellCheck={false}
                />
              </Field>
            </details>
          </fieldset>
          <div className="h-16 overflow-y-auto [scrollbar-gutter:stable]">
            {error && (
              <p role="alert" className="flex items-start gap-2 text-sm text-destructive">
                <CircleAlert size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
                <span>{tunnelErrorMessage(error)}</span>
              </p>
            )}
            {testMessage && (
              <p
                role="status"
                className="flex items-start gap-2 text-sm text-green-700 dark:text-green-400"
              >
                <CircleCheck size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
                <span>
                  {t(
                    testMessage === 'existingSuccess'
                      ? 'hosts:tunnels.existingSuccess'
                      : 'hosts:tunnels.success',
                  )}
                </span>
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => void testConnection()}
            >
              <span className="grid">
                <span
                  className={
                    testing ? 'invisible col-start-1 row-start-1' : 'col-start-1 row-start-1'
                  }
                >
                  {t('hosts:tunnels.test')}
                </span>
                <span
                  className={
                    testing ? 'col-start-1 row-start-1' : 'invisible col-start-1 row-start-1'
                  }
                >
                  {t('hosts:tunnels.testing')}
                </span>
              </span>
            </Button>
            {!editing && (
              <Button type="button" variant="outline" disabled={busy} onClick={onClose}>
                {t('hosts:tunnels.cancel')}
              </Button>
            )}
            <Button type="submit" disabled={busy}>
              {t('hosts:tunnels.save')}
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
  onChange,
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
