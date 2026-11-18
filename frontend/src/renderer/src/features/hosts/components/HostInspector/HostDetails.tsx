import { t, i18n } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { IconButton } from '@/components/ui/desktop'
import { toast } from '@/components/ui/toast'
import { Copy, Eye, EyeOff } from 'lucide-react'
import { domain } from '../../../../api/types'
import { ClipboardSetText } from '../../../../api/runtime'

export function HostDetails({
  host,
  directoryName,
  onGetHostPassword
}: {
  host: domain.Host
  directoryName: string
  onGetHostPassword: (id: string) => Promise<string | null>
}) {
  useTranslation()
  return (
    <div className="grid w-full max-w-3xl gap-5 px-6 pt-5 pb-8 text-[length:var(--app-text-13)]">
      <h2 className="m-0 wrap-anywhere text-lg font-semibold text-[var(--mac-text)]">
        {host.name}
      </h2>
      <DetailGroup title={t('hosts:connection')}>
        <DetailRow label={t('hosts:directory')}>{directoryName}</DetailRow>
        {host.alias && (
          <DetailRow label={t('hosts:hostAlias')}>
            <CopyableDetailValue value={host.alias} label={t('hosts:hostAlias')} />
          </DetailRow>
        )}
        <DetailRow label={t('hosts:address')}>
          <CopyableDetailValue value={host.hostName} label={t('hosts:address')} />
        </DetailRow>
        <DetailRow label={t('hosts:username')}>
          <CopyableDetailValue value={host.user} label={t('hosts:username')} fallback="-" />
        </DetailRow>
        <DetailRow label={t('hosts:port')}>{String(host.port || 22)}</DetailRow>
        <DetailRow label={t('hosts:jumpHost')}>
          {host.proxyJump || t('hosts:directConnection')}
        </DetailRow>
      </DetailGroup>
      <DetailGroup title={t('hosts:metadata')}>
        <DetailRow label={t('hosts:notes')} multiline>
          {host.remark || '-'}
        </DetailRow>
        <DetailRow label={t('hosts:updated')}>{formatDate(host.updatedAt)}</DetailRow>
      </DetailGroup>
      <DetailGroup title={t('hosts:authentication')}>
        <DetailRow label={t('hosts:method')}>
          {host.passwordAuth ? t('hosts:password') : t('hosts:publicKey')}
        </DetailRow>
        {host.passwordAuth ? (
          <DetailRow label={t('hosts:password')}>
            <PasswordDetailValue host={host} onGetHostPassword={onGetHostPassword} />
          </DetailRow>
        ) : (
          <>
            <DetailRow label={t('hosts:privateKeyFile')}>
              {host.identityFile || t('hosts:defaultSshKeys')}
            </DetailRow>
            <DetailRow label={t('hosts:agentForwarding')}>
              {host.forwardAgent ? t('hosts:enabled') : t('hosts:disabled')}
            </DetailRow>
          </>
        )}
        <DetailRow label={t('hosts:compression')}>
          {host.compression ? t('hosts:enabled') : t('hosts:disabled')}
        </DetailRow>
      </DetailGroup>
      <DetailGroup title={t('hosts:reliability')}>
        <DetailRow label={t('hosts:keepaliveInterval')}>
          {secondsLabel(host.serverAliveInterval)}
        </DetailRow>
        <DetailRow label={t('hosts:maxUnansweredKeepalives')}>
          {host.serverAliveCountMax > 0 ? String(host.serverAliveCountMax) : t('hosts:default')}
        </DetailRow>
        <DetailRow label={t('hosts:connectionTimeout')}>
          {secondsLabel(host.connectTimeout)}
        </DetailRow>
      </DetailGroup>
    </div>
  )
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
    if (hideTimer.current) window.clearTimeout(hideTimer.current)
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
  label,
  fallback
}: {
  value?: string
  label: string
  fallback?: string
}) {
  useTranslation()
  if (!value) return <>{fallback ?? '-'}</>
  async function copyValue(event: React.MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    try {
      await ClipboardSetText(value!)
    } catch {
      await navigator.clipboard?.writeText(value!)
    }
    toast.add({ title: t('hosts:copied'), type: 'success' })
  }
  return (
    <span className="inline-flex max-w-full items-center gap-1">
      <span className="min-w-0 wrap-anywhere">
        {value}
      </span>
      <IconButton
        className="size-6 shrink-0 text-muted-foreground hover:text-foreground"
        icon={<Copy size={13} />}
        title={t('hosts:copyField', { label })}
        aria-label={t('hosts:copyField', { label })}
        onClick={copyValue}
      />
    </span>
  )
}

const secondsLabel = (value: number) =>
  value > 0 ? t('hosts:seconds', { count: value }) : t('hosts:default')
function formatDate(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? value
    : new Intl.DateTimeFormat(i18n.language, {
        dateStyle: 'medium',
        timeStyle: 'short'
      }).format(date)
}

function DetailGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="min-w-0">
      <h3 className="mb-2 px-3 text-xs font-semibold text-[var(--mac-secondary-text)]">{title}</h3>
      <dl className="overflow-hidden rounded-lg border border-[var(--mac-separator)] bg-[var(--mac-content)]">
        {children}
      </dl>
    </section>
  )
}

function DetailRow({
  label,
  children,
  multiline = false
}: {
  label: string
  children: ReactNode
  multiline?: boolean
}) {
  return (
    <div className={cn(
      'grid min-h-11 items-center gap-y-1 border-b border-[var(--mac-separator)] px-3 py-2 last:border-b-0',
      !multiline && 'grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-x-4'
    )}>
      <dt className="min-w-0 wrap-anywhere">{label}</dt>
      <dd className={cn(
        'min-w-0 wrap-anywhere text-[var(--mac-secondary-text)]',
        multiline ? 'whitespace-pre-wrap' : 'flex items-center justify-end text-right'
      )}>
        {children}
      </dd>
    </div>
  )
}
