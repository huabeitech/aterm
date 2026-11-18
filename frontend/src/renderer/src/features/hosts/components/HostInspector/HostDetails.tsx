import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { IconButton } from '@/components/ui/desktop'
import { toast } from '@/components/ui/toast'
import { Copy, Eye, EyeOff, Server } from 'lucide-react'
import { domain } from '../../../../api/types'
import { desktop } from '../../../../api/app'
import { ClipboardSetText } from '../../../../api/runtime'

export function HostDetails({
  host,
  jumpHostName,
  notes,
  actions,
  onGetHostPassword
}: {
  host: domain.Host
  jumpHostName?: string
  notes: ReactNode
  actions?: ReactNode
  onGetHostPassword: (id: string) => Promise<string | null>
}) {
  useTranslation()
  const connectionHeadingID = useId()
  const address = host.hostName.includes(':') && !host.hostName.startsWith('[')
    ? `[${host.hostName}]`
    : host.hostName
  const endpoint = `${host.user ? `${host.user}@` : ''}${address}:${host.port || 22}`
  const jumpHost = jumpHostName || host.proxyJump
  return (
    <div className="grid w-full min-w-0 gap-4 px-4 pt-4 pb-6 text-[length:var(--app-text-13)] sm:px-6">
      <header className="flex min-w-0 items-center gap-3">
        <div className="grid size-12 shrink-0 place-items-center rounded-lg border border-[var(--mac-separator)] bg-[var(--mac-content)] text-primary">
          <Server size={28} aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <h2 className="m-0 wrap-anywhere text-2xl leading-tight font-semibold text-[var(--mac-text)]">
            {host.name}
          </h2>
          <div className="flex font-mono text-[var(--mac-secondary-text)]">
            <CopyableDetailValue
              value={endpoint}
              copyValue={host.hostName}
              label={t('hosts:address')}
            />
          </div>
        </div>
      </header>
      {actions}
      <section aria-labelledby={connectionHeadingID} className="min-w-0">
        <h3 id={connectionHeadingID} className="mb-2 text-sm font-semibold text-[var(--mac-text)]">
          {t('hosts:connection')}
        </h3>
        <dl className="grid min-w-0 grid-cols-[repeat(auto-fit,minmax(min(100%,10rem),1fr))] gap-2">
          <FullHostDetails host={host} jumpHost={jumpHost} onGetHostPassword={onGetHostPassword} />
        </dl>
      </section>
      {notes}
    </div>
  )
}

function FullHostDetails({
  host,
  jumpHost,
  onGetHostPassword
}: {
  host: domain.Host
  jumpHost: string
  onGetHostPassword: (id: string) => Promise<string | null>
}) {
  return (
    <>
      <DetailRow label={t('hosts:address')}>
        <CopyableDetailValue value={host.hostName} label={t('hosts:address')} />
      </DetailRow>
      <DetailRow label={t('hosts:username')}>
        <CopyableDetailValue value={host.user} label={t('hosts:username')} fallback="-" />
      </DetailRow>
      <DetailRow label={t('hosts:port')}>{String(host.port || 22)}</DetailRow>
      {host.alias && (
        <DetailRow label={t('hosts:hostAlias')}>
          <CopyableDetailValue value={host.alias} label={t('hosts:hostAlias')} />
        </DetailRow>
      )}
      <DetailRow label={t('hosts:jumpHost')}>
        {jumpHost || t('hosts:directConnection')}
      </DetailRow>
      <DetailRow label={t('hosts:authentication')}>
        {host.passwordAuth ? t('hosts:password') : t('hosts:publicKey')}
      </DetailRow>
      {host.passwordAuth ? (
        <DetailRow label={t('hosts:password')}>
          <PasswordDetailValue host={host} onGetHostPassword={onGetHostPassword} />
        </DetailRow>
      ) : (
        <>
          <DetailRow label={t('hosts:privateKeyFile')}>
            <IdentityDetailValue identityFile={host.identityFile} />
          </DetailRow>
          <DetailRow label={t('hosts:agentForwarding')}>
            {host.forwardAgent ? t('hosts:enabled') : t('hosts:disabled')}
          </DetailRow>
        </>
      )}
      <DetailRow label={t('hosts:compression')}>
        {host.compression ? t('hosts:enabled') : t('hosts:disabled')}
      </DetailRow>
    </>
  )
}

function IdentityDetailValue({ identityFile }: { identityFile: string }) {
  const [keyName, setKeyName] = useState<{ identity: string; name: string } | null>(null)
  const managed = identityFile.startsWith('aterm-key://')

  useEffect(() => {
    if (!managed) return
    let active = true
    void (async () => {
      try {
        const status = await desktop.library.vaultStatus()
        if (!active || !status.unlocked) return
        const keys = await desktop.library.listManagedKeys()
        const key = keys.find((item) => identityFile === `aterm-key://${item.id}`)
        if (active && key) setKeyName({ identity: identityFile, name: key.name })
      } catch {
        // The localized key source remains available when the vault is locked.
      }
    })()
    return () => { active = false }
  }, [identityFile, managed])

  const value = managed
    ? (keyName?.identity === identityFile ? keyName.name : t('hosts:managedSshKey'))
    : identityFile || t('hosts:defaultSshKeys')
  return <span className="block truncate" title={identityFile || value}>{value}</span>
}

function PasswordDetailValue({
  host,
  onGetHostPassword
}: {
  host: domain.Host
  onGetHostPassword: (id: string) => Promise<string | null>
}) {
  useTranslation()
  const [password, setPassword] = useState<string | null>(null)
  const [visible, setVisible] = useState(false)
  const [loading, setLoading] = useState<'reveal' | 'copy' | null>(null)
  const hideTimer = useRef<number | null>(null)

  useEffect(() => {
    setPassword(null)
    setVisible(false)
    return () => {
      if (hideTimer.current) window.clearTimeout(hideTimer.current)
    }
  }, [host.id])

  function scheduleHide() {
    if (hideTimer.current) window.clearTimeout(hideTimer.current)
    hideTimer.current = window.setTimeout(() => setVisible(false), 30000)
  }

  async function loadPassword(action: 'reveal' | 'copy') {
    if (password) return password
    setLoading(action)
    try {
      return await onGetHostPassword(host.id)
    } finally {
      setLoading(null)
    }
  }

  async function revealPassword() {
    if (visible) {
      setVisible(false)
      return
    }
    const next = await loadPassword('reveal')
    if (!next) return
    setPassword(next)
    setVisible(true)
    scheduleHide()
  }

  async function copyPassword() {
    const next = await loadPassword('copy')
    if (!next) return
    try {
      await ClipboardSetText(next)
    } catch {
      await navigator.clipboard?.writeText(next)
    }
    toast.add({ title: t('hosts:passwordCopied'), type: 'success' })
  }

  return (
    <span className="inline-flex max-w-full items-center gap-1">
      <span className="min-w-0 wrap-anywhere">
        {visible && password ? password : '••••••••'}
      </span>
      <span className="inline-flex gap-0.5">
        <IconButton
          className="size-6 shrink-0 text-muted-foreground hover:text-foreground"
          icon={visible ? <EyeOff size={13} /> : <Eye size={13} />}
          loading={loading === 'reveal'}
          title={visible ? t('hosts:hidePassword') : t('hosts:revealPassword')}
          aria-label={visible ? t('hosts:hidePassword') : t('hosts:revealPassword')}
          onClick={revealPassword}
        />
        <IconButton
          className="size-6 shrink-0 text-muted-foreground hover:text-foreground"
          icon={<Copy size={13} />}
          loading={loading === 'copy'}
          title={t('hosts:copyPassword')}
          aria-label={t('hosts:copyPassword')}
          onClick={copyPassword}
        />
      </span>
    </span>
  )
}

function CopyableDetailValue({
  value,
  copyValue = value,
  label,
  fallback
}: {
  value?: string
  copyValue?: string
  label: string
  fallback?: string
}) {
  useTranslation()
  if (!value) return <>{fallback ?? '-'}</>
  async function copy(event: React.MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    if (!copyValue) return
    try {
      await ClipboardSetText(copyValue)
    } catch {
      await navigator.clipboard?.writeText(copyValue)
    }
    toast.add({ title: t('hosts:copied'), type: 'success' })
  }
  return (
    <span className="group/copy inline-flex max-w-full items-center gap-1">
      <span className="min-w-0 truncate" title={value}>
        {value}
      </span>
      <IconButton
        className="size-6 shrink-0 text-muted-foreground opacity-40 hover:text-foreground hover:opacity-100 focus-visible:opacity-100 group-hover/copy:opacity-100 group-focus-within/copy:opacity-100 [@media(hover:none)]:opacity-100"
        icon={<Copy size={13} />}
        title={t('hosts:copyField', { label })}
        aria-label={t('hosts:copyField', { label })}
        onClick={copy}
      />
    </span>
  )
}

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid min-w-0 content-start gap-1 rounded-lg bg-muted/50 px-3 py-2">
      <dt className="min-w-0 wrap-anywhere text-xs text-[var(--mac-secondary-text)]">{label}</dt>
      <dd className="flex min-h-6 min-w-0 items-center wrap-anywhere text-[var(--mac-text)]">
        {children}
      </dd>
    </div>
  )
}
