import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { IconButton } from '@/components/ui/desktop'
import { toast } from '@/components/ui/toast'
import { ChevronRight, Copy, Eye, EyeOff } from 'lucide-react'
import { domain } from '../../../../api/types'
import { desktop } from '../../../../api/app'
import { ClipboardSetText } from '../../../../api/runtime'

const sectionHeading =
  'm-0 min-h-6 text-sm font-semibold text-foreground'

type SectionID = 'connection' | 'authentication' | 'advancedSettings' | 'notes'

export function HostDetails({
  host,
  directoryPath,
  jumpHostName,
  actions,
  onGetHostPassword
}: {
  host: domain.Host
  directoryPath?: string
  jumpHostName?: string
  actions: (identity: ReactNode) => ReactNode
  onGetHostPassword: (id: string) => Promise<string | null>
}) {
  const { i18n } = useTranslation()
  const seconds = new Intl.NumberFormat(i18n.language, { style: 'unit', unit: 'second' })
  const number = new Intl.NumberFormat(i18n.language)
  const defaultValue = t('hosts:default')
  const address = host.hostName.includes(':') && !host.hostName.startsWith('[')
    ? `[${host.hostName}]`
    : host.hostName
  const endpoint = `${host.user ? `${host.user}@` : ''}${address}:${host.port || 22}`
  const jumpHost = jumpHostName || host.proxyJump ||
    (host.jumpHostID ? '—' : t('hosts:directConnection'))
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col text-sm">
      <div className="shrink-0 border-b border-border/60 px-3 pt-3 pb-3">
        {actions(
          <div className="min-w-0">
            <h2 className="m-0 truncate text-base leading-tight font-semibold text-[var(--mac-text)]" title={host.name}>
              {host.name}
            </h2>
            <div className="mt-1 flex min-w-0 font-mono text-sm text-[var(--mac-secondary-text)]">
              <CopyableDetailValue
                alwaysShowCopy
                value={endpoint}
                copyLabel={t('hosts:copyConnectionAddress')}
              />
            </div>
          </div>
        )}
      </div>
      <div className="grid min-h-0 min-w-0 content-start gap-3 overflow-y-auto px-3 py-2">
        <DetailGroup sectionID="connection" title={t('hosts:connection')} initiallyOpen>
          <DetailRow label={t('hosts:address')}>{host.hostName}</DetailRow>
          <DetailRow label={t('hosts:port')}>{host.port || 22}</DetailRow>
          <DetailRow label={t('hosts:username')}>
            <CopyableDetailValue
              value={host.user}
              copyLabel={t('hosts:copyField', { label: t('hosts:username') })}
            />
          </DetailRow>
          <DetailRow label={t('hosts:hostAlias')}>
            <CopyableDetailValue
              value={host.alias}
              copyLabel={t('hosts:copyField', { label: t('hosts:hostAlias') })}
            />
          </DetailRow>
          <DetailRow label={t('hosts:directory')}>
            {host.directoryID ? directoryPath || '—' : t('hosts:rootDirectory')}
          </DetailRow>
        </DetailGroup>
        <DetailGroup sectionID="authentication" title={t('hosts:authentication')} initiallyOpen>
          <DetailRow label={t('hosts:method')}>
            {host.passwordAuth ? t('hosts:password') : t('hosts:publicKey')}
          </DetailRow>
          {host.passwordAuth ? (
            <DetailRow label={t('hosts:password')}>
              <PasswordDetailValue host={host} onGetHostPassword={onGetHostPassword} />
            </DetailRow>
          ) : (
            <DetailRow label={t('hosts:privateKeyFile')}>
              <IdentityDetailValue identityFile={host.identityFile} />
            </DetailRow>
          )}
        </DetailGroup>
        <DetailGroup
          sectionID="advancedSettings"
          title={t('hosts:advancedSettings')}
          collapsedSummary={jumpHostName || host.proxyJump ? (
            <span className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-sm font-normal text-muted-foreground">
              <span>{t('hosts:jumpHost')}</span>
              <span className="min-w-0 truncate" title={jumpHost}>{jumpHost}</span>
            </span>
          ) : undefined}
        >
          <DetailRow label={t('hosts:jumpHost')}>{jumpHost}</DetailRow>
          <DetailRow label={t('hosts:agentForwarding')}>
            {host.forwardAgent ? t('hosts:enabled') : t('hosts:disabled')}
          </DetailRow>
          <DetailRow label={t('hosts:compression')}>
            {host.compression ? t('hosts:enabled') : t('hosts:disabled')}
          </DetailRow>
          <DetailRow label={t('hosts:connectionTimeout')}>
            {host.connectTimeout > 0 ? seconds.format(host.connectTimeout) : defaultValue}
          </DetailRow>
          <DetailRow label={t('hosts:keepaliveInterval')}>
            {host.serverAliveInterval > 0 ? seconds.format(host.serverAliveInterval) : defaultValue}
          </DetailRow>
          <DetailRow label={t('hosts:maxUnansweredKeepalives')}>
            {host.serverAliveCountMax > 0 ? number.format(host.serverAliveCountMax) : defaultValue}
          </DetailRow>
        </DetailGroup>
        {host.remark && (
          <CollapsibleSection sectionID="notes" title={t('hosts:notes')} initiallyOpen>
            <p className="m-0 select-text wrap-anywhere whitespace-pre-wrap py-1 text-sm leading-relaxed text-foreground">
              {host.remark}
            </p>
          </CollapsibleSection>
        )}
      </div>
    </div>
  )
}

function CollapsibleSection({
  sectionID,
  title,
  collapsedSummary,
  children,
  initiallyOpen = false
}: {
  sectionID: SectionID
  title: string
  collapsedSummary?: ReactNode
  children: ReactNode
  initiallyOpen?: boolean
}) {
  const storageKey = `aterm.hostInspector.section.${sectionID}`
  const [open, setOpen] = useState(() => {
    try {
      const stored = localStorage.getItem(storageKey)
      if (stored === 'true' || stored === 'false') return stored === 'true'
    } catch {
      // Use the default when storage is unavailable.
    }
    return initiallyOpen
  })

  return (
    <details className="group/section min-w-0" open={open}>
      <summary
        className={`${sectionHeading} flex cursor-pointer list-none items-center gap-1.5 [&::-webkit-details-marker]:hidden`}
        onClick={(event) => {
          event.preventDefault()
          const next = !open
          setOpen(next)
          try {
            localStorage.setItem(storageKey, String(next))
          } catch {
            // Keep the section usable when storage is unavailable.
          }
        }}
      >
        <ChevronRight size={13} aria-hidden="true" className="shrink-0 group-open/section:rotate-90" />
        <span className="shrink-0">{title}</span>
        {!open && collapsedSummary}
      </summary>
      {children}
    </details>
  )
}

function DetailGroup({
  sectionID,
  title,
  collapsedSummary,
  children,
  initiallyOpen
}: {
  sectionID: SectionID
  title: string
  collapsedSummary?: ReactNode
  children: ReactNode
  initiallyOpen?: boolean
}) {
  return (
    <CollapsibleSection
      sectionID={sectionID}
      title={title}
      initiallyOpen={initiallyOpen}
      collapsedSummary={collapsedSummary}
    >
      <dl className="m-0">{children}</dl>
    </CollapsibleSection>
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
  return <span className="min-w-0 select-text wrap-anywhere" title={value}>{value}</span>
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
          className="size-5 shrink-0 text-muted-foreground hover:text-foreground"
          icon={visible ? <EyeOff className="size-[1em]" /> : <Eye className="size-[1em]" />}
          loading={loading === 'reveal'}
          title={visible ? t('hosts:hidePassword') : t('hosts:revealPassword')}
          aria-label={visible ? t('hosts:hidePassword') : t('hosts:revealPassword')}
          onClick={revealPassword}
        />
        <IconButton
          className="size-5 shrink-0 text-muted-foreground hover:text-foreground"
          icon={<Copy className="size-[1em]" />}
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
  copyLabel,
  alwaysShowCopy = false
}: {
  value?: string
  copyLabel: string
  alwaysShowCopy?: boolean
}) {
  useTranslation()
  if (!value) return <>—</>
  async function copy(event: React.MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    if (!value) return
    try {
      await ClipboardSetText(value)
    } catch {
      await navigator.clipboard?.writeText(value)
    }
    toast.add({ title: t('hosts:copied'), type: 'success' })
  }
  return (
    <span className="inline-flex max-w-full items-center gap-1">
      <span className="min-w-0 select-text wrap-anywhere" title={value}>
        {value}
      </span>
      <IconButton
        className={`size-5 shrink-0 text-muted-foreground hover:text-foreground ${alwaysShowCopy ? '' : 'opacity-0 group-hover/detail-row:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100'}`}
        icon={<Copy className="size-[1em]" />}
        title={copyLabel}
        aria-label={copyLabel}
        onClick={copy}
      />
    </span>
  )
}

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="group/detail-row grid min-w-0 grid-cols-[min(8rem,42%)_minmax(0,1fr)] items-baseline gap-x-4 py-1 text-[length:var(--app-text-13)]">
      <dt className="min-w-0 wrap-anywhere text-muted-foreground">{label}</dt>
      <dd className="m-0 min-w-0 select-text wrap-anywhere text-foreground">
        {children}
      </dd>
    </div>
  )
}
