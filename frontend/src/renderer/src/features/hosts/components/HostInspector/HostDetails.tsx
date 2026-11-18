import { t, i18n } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { IconButton } from '@/components/ui/desktop'
import { toast } from '@/components/ui/toast'
import { Copy, Eye, EyeOff } from 'lucide-react'
import { domain } from '../../../../api/types'
import { ClipboardSetText } from '../../../../api/runtime'
import { DetailSection } from './DetailSection'

export function HostDetails({
  host,
  directoryName,
  pluginActions,
  onGetHostPassword
}: {
  host: domain.Host
  directoryName: string
  pluginActions: import("react").ReactNode
  onGetHostPassword: (id: string) => Promise<string | null>
}) {
  useTranslation()
  return (
    <div className="details-stack grid gap-4.5 [.host-inspector_&]:gap-3.5">
      <h2 className="host-details-title m-0 text-[var(--mac-text)] text-lg font-[650] leading-[calc(1.5rem*var(--app-font-scale))] overflow-hidden text-ellipsis whitespace-nowrap">
        {host.name}
      </h2>
      <DetailSection title={t('hosts:connection')}>
        <dl className="host-details-table overflow-hidden rounded-md border border-border">
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
        </dl>
      </DetailSection>
      <DetailSection title={t('hosts:metadata')}>
        <dl className="host-details-table overflow-hidden rounded-md border border-border">
          <DetailRow label={t('hosts:notes')}>
            <div className="remark-content max-w-full wrap-anywhere whitespace-pre-wrap [word-break:break-word]">
              {host.remark || '-'}
            </div>
          </DetailRow>
          <DetailRow label={t('hosts:updated')}>{formatDate(host.updatedAt)}</DetailRow>
        </dl>
      </DetailSection>
      {pluginActions}
      <DetailSection title={t('hosts:authentication')}>
        <dl className="host-details-table overflow-hidden rounded-md border border-border">
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
                <span
                  className={cn(
                    'host-detail-value min-w-0 [font:inherit] leading-[calc(1.25rem*var(--app-font-scale))] [.copyable-detail-value_&]:min-w-0 [.copyable-detail-value_&]:overflow-hidden',
                    '[.copyable-detail-value_&]:text-ellipsis [.copyable-detail-value_&]:whitespace-nowrap',
                    '[.host-inspector_.copyable-detail-value_&]:whitespace-normal [.host-inspector_.copyable-detail-value_&]:wrap-anywhere'
                  )}
                >
                  {host.identityFile || t('hosts:defaultSshKeys')}
                </span>
              </DetailRow>
              <DetailRow label={t('hosts:agentForwarding')}>
                {host.forwardAgent ? t('hosts:enabled') : t('hosts:disabled')}
              </DetailRow>
            </>
          )}
          <DetailRow label={t('hosts:compression')}>
            {host.compression ? t('hosts:enabled') : t('hosts:disabled')}
          </DetailRow>
        </dl>
      </DetailSection>
      <DetailSection title={t('hosts:reliability')}>
        <dl className="host-details-table overflow-hidden rounded-md border border-border">
          <DetailRow label={t('hosts:keepaliveInterval')}>
            {secondsLabel(host.serverAliveInterval)}
          </DetailRow>
          <DetailRow label={t('hosts:maxUnansweredKeepalives')}>
            {host.serverAliveCountMax > 0 ? String(host.serverAliveCountMax) : t('hosts:default')}
          </DetailRow>
          <DetailRow label={t('hosts:connectionTimeout')}>
            {secondsLabel(host.connectTimeout)}
          </DetailRow>
        </dl>
      </DetailSection>
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
    <span className="copyable-detail-value inline-flex max-w-full items-center gap-1 align-middle">
      <span
        className={cn(
          'host-detail-value min-w-0 [font:inherit] leading-[calc(1.25rem*var(--app-font-scale))] [.copyable-detail-value_&]:min-w-0 [.copyable-detail-value_&]:overflow-hidden',
          '[.copyable-detail-value_&]:text-ellipsis [.copyable-detail-value_&]:whitespace-nowrap',
          '[.host-inspector_.copyable-detail-value_&]:whitespace-normal [.host-inspector_.copyable-detail-value_&]:wrap-anywhere'
        )}
      >
        {visible && password ? password : '••••••••'}
      </span>
      <span className="inline-flex gap-0.5">
        <IconButton
          className={cn(
            'copyable-detail-button w-6 min-w-6 h-6 text-[rgba(60,_60,_67,_0.58)] hover:text-[#1d1d1f]',
            '[:root[data-theme=dark]_&]:text-[var(--mac-secondary-text)] [:root[data-theme=dark]_&:hover]:text-[var(--mac-text)]',
            '[.host-inspector_&]:shrink-0'
          )}
          icon={visible ? <EyeOff size={13} /> : <Eye size={13} />}
          loading={loading === 'reveal'}
          title={visible ? t('hosts:hidePassword') : t('hosts:revealPassword')}
          aria-label={visible ? t('hosts:hidePassword') : t('hosts:revealPassword')}
          onClick={revealPassword}
        />
        <IconButton
          className={cn(
            'copyable-detail-button w-6 min-w-6 h-6 text-[rgba(60,_60,_67,_0.58)] hover:text-[#1d1d1f]',
            '[:root[data-theme=dark]_&]:text-[var(--mac-secondary-text)] [:root[data-theme=dark]_&:hover]:text-[var(--mac-text)]',
            '[.host-inspector_&]:shrink-0'
          )}
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
    <span className="copyable-detail-value inline-flex max-w-full items-center gap-1 align-middle">
      <span
        className={cn(
          'host-detail-value min-w-0 [font:inherit] leading-[calc(1.25rem*var(--app-font-scale))] [.copyable-detail-value_&]:min-w-0 [.copyable-detail-value_&]:overflow-hidden',
          '[.copyable-detail-value_&]:text-ellipsis [.copyable-detail-value_&]:whitespace-nowrap',
          '[.host-inspector_.copyable-detail-value_&]:whitespace-normal [.host-inspector_.copyable-detail-value_&]:wrap-anywhere'
        )}
      >
        {value}
      </span>
      <IconButton
        className={cn(
          'copyable-detail-button w-6 min-w-6 h-6 text-[rgba(60,_60,_67,_0.58)] hover:text-[#1d1d1f]',
          '[:root[data-theme=dark]_&]:text-[var(--mac-secondary-text)] [:root[data-theme=dark]_&:hover]:text-[var(--mac-text)]',
          '[.host-inspector_&]:shrink-0'
        )}
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

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[104px_minmax(0,1fr)] border-b border-border last:border-0 text-xs">
      <dt className="bg-muted/50 px-2.5 py-2 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words px-2.5 py-2">{children}</dd>
    </div>
  )
}
